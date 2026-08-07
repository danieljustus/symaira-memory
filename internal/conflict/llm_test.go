package conflict

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/llm"
)

func TestParseVerdictResponseStrictJSON(t *testing.T) {
	raw := `{"verdicts":[{"pair":0,"verdict":"contradiction"},{"pair":1,"verdict":"repeat"},{"pair":2,"verdict":"ambiguous"}]}`
	got, err := parseVerdictResponse(raw, 3)
	if err != nil {
		t.Fatalf("parseVerdictResponse: %v", err)
	}
	want := []Verdict{VerdictContradiction, VerdictRepeat, VerdictAmbiguous}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("verdict[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestParseVerdictResponseDefaultsMissingPairsToAmbiguous(t *testing.T) {
	// Only pair 1 is decided; pairs 0 and 2 must default to ambiguous.
	raw := `{"verdicts":[{"pair":1,"verdict":"repeat"}]}`
	got, err := parseVerdictResponse(raw, 3)
	if err != nil {
		t.Fatalf("parseVerdictResponse: %v", err)
	}
	if got[0] != VerdictAmbiguous || got[2] != VerdictAmbiguous {
		t.Errorf("undecided pairs must be ambiguous, got %v", got)
	}
	if got[1] != VerdictRepeat {
		t.Errorf("pair 1 = %v, want repeat", got[1])
	}
}

func TestParseVerdictResponseIgnoresOutOfRangePairs(t *testing.T) {
	raw := `{"verdicts":[{"pair":7,"verdict":"contradiction"},{"pair":0,"verdict":"repeat"}]}`
	got, err := parseVerdictResponse(raw, 2)
	if err != nil {
		t.Fatalf("parseVerdictResponse: %v", err)
	}
	if got[0] != VerdictRepeat || got[1] != VerdictAmbiguous {
		t.Errorf("out-of-range pair must be ignored, got %v", got)
	}
}

func TestParseVerdictResponseSalvageFromProse(t *testing.T) {
	raw := "Here are my decisions:\n\"verdict\": \"contradiction\" for the first pair, and \"verdict\": \"repeat\" for the second."
	got, err := parseVerdictResponse(raw, 2)
	if err != nil {
		t.Fatalf("parseVerdictResponse: %v", err)
	}
	if got[0] != VerdictContradiction || got[1] != VerdictRepeat {
		t.Errorf("salvage parse failed, got %v", got)
	}
}

func TestParseVerdictResponseUnknownTokensStayAmbiguous(t *testing.T) {
	raw := `{"verdicts":[{"pair":0,"verdict":"maybe"}]}`
	got, err := parseVerdictResponse(raw, 1)
	if err != nil {
		t.Fatalf("parseVerdictResponse: %v", err)
	}
	if got[0] != VerdictAmbiguous {
		t.Errorf("unknown verdict token must stay ambiguous, got %v", got[0])
	}
}

// ollamaStreamServer simulates an Ollama streaming endpoint that echoes
// the requested schema back inside a JSON payload.
func ollamaStreamServer(t *testing.T, payload string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model  string         `json:"model"`
			Prompt string         `json:"prompt"`
			System string         `json:"system"`
			Format map[string]any `json:"format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if req.Model != "test-model" {
			t.Errorf("model = %q, want test-model", req.Model)
		}
		if req.Format == nil {
			t.Error("expected a JSON schema in the format field")
		}
		if !strings.Contains(req.Prompt, "pair 0") || !strings.Contains(req.Prompt, "pair 1") {
			t.Errorf("expected both pairs in one batched prompt, got: %.200s", req.Prompt)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		// Stream the payload in small chunks, then finish.
		chunks := []string{payload[:len(payload)/2], payload[len(payload)/2:], `{"done":true}`}
		for _, c := range chunks {
			if c == "" {
				continue
			}
			line, _ := json.Marshal(map[string]any{"response": c})
			_, _ = fmt.Fprintf(w, "%s\n", line)
		}
	}))
}

func TestLLMVerdictProviderBatchedCall(t *testing.T) {
	server := ollamaStreamServer(t, `{"verdicts":[{"pair":0,"verdict":"contradiction"},{"pair":1,"verdict":"repeat"}]}`)
	defer server.Close()

	p := &LLMVerdictProvider{client: llm.NewClient(server.URL, "test-model"), provider: "ollama"}
	p.client.HTTPClient = server.Client()

	pairs := []Pair{
		{Cand: &db.Memory{ID: "a"}, NewContent: "the daemon runs on port 9000", OldContent: "the daemon listens on port 8787", NewActor: "client-a", OldActor: "client-b", Similarity: 0.9},
		{Cand: &db.Memory{ID: "b"}, NewContent: "alice prefers dark mode", OldContent: "alice prefers dark mode in all applications", NewActor: "client-a", OldActor: "client-c", Similarity: 0.88},
	}
	got, err := p.Verdicts(context.Background(), pairs)
	if err != nil {
		t.Fatalf("Verdicts: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 verdicts, got %d", len(got))
	}
	if got[0] != VerdictContradiction || got[1] != VerdictRepeat {
		t.Errorf("verdicts = %v, want [contradiction repeat]", got)
	}
}

func TestLLMVerdictProviderErrorPropagates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	p := &LLMVerdictProvider{client: llm.NewClient(server.URL, "test-model"), provider: "ollama"}
	p.client.HTTPClient = server.Client()

	_, err := p.Verdicts(context.Background(), []Pair{{Cand: &db.Memory{ID: "a"}}})
	if err == nil {
		t.Fatal("expected an error from a failing endpoint")
	}
}
