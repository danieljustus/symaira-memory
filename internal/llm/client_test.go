package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClientDefaults(t *testing.T) {
	client := NewClient("", "")

	if client.OllamaURL != "http://localhost:11434/api/generate" {
		t.Errorf("expected default OllamaURL, got %s", client.OllamaURL)
	}
	if client.OllamaModel != "llama3" {
		t.Errorf("expected default OllamaModel 'llama3', got %s", client.OllamaModel)
	}
	if client.HTTPClient == nil {
		t.Error("expected HTTPClient to be set")
	}
	if client.HTTPClient.Timeout != 45*time.Second {
		t.Errorf("expected 45s timeout, got %v", client.HTTPClient.Timeout)
	}
}

func TestNewClientCustomParams(t *testing.T) {
	client := NewClient("http://custom:11434/api/generate", "mistral")

	if client.OllamaURL != "http://custom:11434/api/generate" {
		t.Errorf("expected custom OllamaURL, got %s", client.OllamaURL)
	}
	if client.OllamaModel != "mistral" {
		t.Errorf("expected custom OllamaModel 'mistral', got %s", client.OllamaModel)
	}
}

func TestQueryOllamaSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		var reqBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}

		if reqBody["model"] != "llama3" {
			t.Errorf("expected model 'llama3', got %v", reqBody["model"])
		}
		if reqBody["stream"] != false {
			t.Errorf("expected stream false, got %v", reqBody["stream"])
		}
		if reqBody["format"] != "json" {
			t.Errorf("expected format 'json', got %v", reqBody["format"])
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"response": "test response",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "llama3")
	result, err := client.QueryOllama(context.Background(), "system", "user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "test response" {
		t.Errorf("expected 'test response', got %s", result)
	}
}

func TestQueryOllamaHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, "llama3")
	_, err := client.QueryOllama(context.Background(), "system", "user")
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
}

func TestQueryOllamaMalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "llama3")
	_, err := client.QueryOllama(context.Background(), "system", "user")
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestQueryOllamaContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := NewClient(server.URL, "llama3")
	_, err := client.QueryOllama(ctx, "system", "user")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestQueryOpenAISuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Bearer test-key, got %s", r.Header.Get("Authorization"))
		}

		var reqBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}

		if reqBody["model"] != "gpt-4o-mini" {
			t.Errorf("expected model 'gpt-4o-mini', got %v", reqBody["model"])
		}

		messages := reqBody["messages"].([]interface{})
		if len(messages) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(messages))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": "openai response",
					},
				},
			},
		})
	}))
	defer server.Close()

	client := NewClient("", "")
	result, err := client.QueryOpenAI(context.Background(), "system", "user", "test-key", "", server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "openai response" {
		t.Errorf("expected 'openai response', got %s", result)
	}
}

func TestQueryOpenAICustomModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]interface{}
		json.NewDecoder(r.Body).Decode(&reqBody)

		if reqBody["model"] != "gpt-4" {
			t.Errorf("expected model 'gpt-4', got %v", reqBody["model"])
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": "response",
					},
				},
			},
		})
	}))
	defer server.Close()

	client := NewClient("", "")
	result, err := client.QueryOpenAI(context.Background(), "system", "user", "test-key", "gpt-4", server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "response" {
		t.Errorf("expected 'response', got %s", result)
	}
}

func TestQueryOpenAIHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewClient("", "")
	_, err := client.QueryOpenAI(context.Background(), "system", "user", "bad-key", "", server.URL)
	if err == nil {
		t.Fatal("expected error for HTTP 401")
	}
}

func TestQueryOpenAIEmptyChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []interface{}{},
		})
	}))
	defer server.Close()

	client := NewClient("", "")
	_, err := client.QueryOpenAI(context.Background(), "system", "user", "test-key", "", server.URL)
	if err == nil {
		t.Fatal("expected error for empty choices")
	}
}

func TestQueryOpenAIMalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	client := NewClient("", "")
	_, err := client.QueryOpenAI(context.Background(), "system", "user", "test-key", "", server.URL)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestQueryOllamaProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"response": "ollama result",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "llama3")
	result, err := client.Query(context.Background(), "system", "user", "ollama", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ollama result" {
		t.Errorf("expected 'ollama result', got %s", result)
	}
}

func TestQueryOpenAIProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": "openai result",
					},
				},
			},
		})
	}))
	defer server.Close()

	client := NewClient("", "")
	result, err := client.QueryOpenAI(context.Background(), "system", "user", "test-key", "", server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "openai result" {
		t.Errorf("expected 'openai result', got %s", result)
	}
}

func TestQueryOpenAIProviderNoKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")

	client := NewClient("", "")
	_, err := client.Query(context.Background(), "system", "user", "openai", "")
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestQueryOpenAIProviderUsesEnvKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer env-key" {
			t.Errorf("expected Bearer env-key, got %s", r.Header.Get("Authorization"))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": "result",
					},
				},
			},
		})
	}))
	defer server.Close()

	t.Setenv("OPENAI_API_KEY", "env-key")

	client := NewClient("", "")
	result, err := client.QueryOpenAI(context.Background(), "system", "user", "env-key", "", server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "result" {
		t.Errorf("expected 'result', got %s", result)
	}
}
