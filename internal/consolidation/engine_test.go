package consolidation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/extractor"
)

func TestParseJSONResponse(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantErr     bool
		wantConsol  int
		wantDiscard int
	}{
		{
			name: "valid JSON",
			input: `{
				"consolidated": [{"content": "fact", "replaces_ids": ["a"], "metadata": {}}],
				"discarded_ids": ["b"]
			}`,
			wantErr:     false,
			wantConsol:  1,
			wantDiscard: 1,
		},
		{
			name:    "malformed JSON - missing closing brace",
			input:   `{"consolidated": [{"content": "fact"`,
			wantErr: true,
		},
		{
			name:    "malformed JSON - invalid syntax",
			input:   `not json at all`,
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "empty object",
			input:   `{}`,
			wantErr: false,
		},
		{
			name: "markdown wrapped JSON",
			input: "```json\n" + `{"consolidated": [], "discarded_ids": []}` + "\n```",
			wantErr: false,
		},
		{
			name:    "JSON with extra text before",
			input:   "Here is the result: {\"consolidated\": [], \"discarded_ids\": []}",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseJSONResponse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseJSONResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != nil {
				if len(result.Consolidated) != tt.wantConsol {
					t.Errorf("expected %d consolidated items, got %d", tt.wantConsol, len(result.Consolidated))
				}
				if len(result.DiscardedIDs) != tt.wantDiscard {
					t.Errorf("expected %d discarded IDs, got %d", tt.wantDiscard, len(result.DiscardedIDs))
				}
			}
		})
	}
}

func TestQueryOllamaConnectionFailure(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "symmemory-ollama-fail-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := config.Defaults()
	cfg.Database.Path = filepath.Join(tempDir, "test.db")

	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	embeddings := extractor.NewEmbeddingsGenerator(cfg)
	engine := NewEngine(database, embeddings, "http://localhost:1", "test-model", "ollama", false)

	_, err = engine.queryOllama(context.Background(), "test prompt")
	if err == nil {
		t.Error("expected error for connection failure, got nil")
	}
}

func TestNewEngineDefaults(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "symmemory-engine-defaults-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := config.Defaults()
	cfg.Database.Path = filepath.Join(tempDir, "test.db")

	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	embeddings := extractor.NewEmbeddingsGenerator(cfg)

	// Test Ollama defaults
	eng := NewEngine(database, embeddings, "", "", "", false)
	if eng.llmURL != "http://localhost:11434/api/generate" {
		t.Errorf("expected Ollama default URL, got %s", eng.llmURL)
	}
	if eng.llmModel != "llama3" {
		t.Errorf("expected Ollama default model 'llama3', got %s", eng.llmModel)
	}
	if eng.llmProvider != "ollama" {
		t.Errorf("expected provider 'ollama', got %s", eng.llmProvider)
	}

	// Test OpenAI defaults
	eng2 := NewEngine(database, embeddings, "", "", "openai", false)
	if eng2.llmURL != "https://api.openai.com/v1/chat/completions" {
		t.Errorf("expected OpenAI default URL, got %s", eng2.llmURL)
	}
	if eng2.llmModel != "gpt-4o-mini" {
		t.Errorf("expected OpenAI default model 'gpt-4o-mini', got %s", eng2.llmModel)
	}
}

func TestEngineConsolidation(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "symmemory-engine-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := config.Defaults()
	cfg.Database.Path = filepath.Join(tempDir, "test.db")

	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Seed raw memories
	m1 := &db.Memory{
		ID:                  "mem-1",
		Content:             "Daniel likes dark mode",
		Scope:               "global",
		ConsolidationStatus: "raw",
		Metadata:            map[string]string{},
	}
	m2 := &db.Memory{
		ID:                  "mem-2",
		Content:             "Daniel prefers dark backgrounds",
		Scope:               "global",
		ConsolidationStatus: "raw",
		Metadata:            map[string]string{},
	}
	m3 := &db.Memory{
		ID:                  "mem-3",
		Content:             "Daniel is going to get coffee now",
		Scope:               "global",
		ConsolidationStatus: "raw",
		Metadata:            map[string]string{},
	}
	// A memory in a different scope
	m4 := &db.Memory{
		ID:                  "mem-4",
		Content:             "Swift is used for terminal app",
		Scope:               "project-a",
		ConsolidationStatus: "raw",
		Metadata:            map[string]string{},
	}

	for _, m := range []*db.Memory{m1, m2, m3, m4} {
		if err := database.SaveMemory(m); err != nil {
			t.Fatalf("failed to seed memory %s: %v", m.ID, err)
		}
	}

	// Create mock LLM server
	serverCalled := 0
	mockLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalled++
		w.Header().Set("Content-Type", "application/json")

		// Mock response for Ollama
		respObj := map[string]string{
			"response": `{
				"consolidated": [
					{
						"content": "Daniel prefers dark mode.",
						"replaces_ids": ["mem-1", "mem-2"],
						"metadata": { "topic": "preferences" }
					}
				],
				"discarded_ids": ["mem-3"]
			}`,
		}
		json.NewEncoder(w).Encode(respObj)
	}))
	defer mockLLM.Close()

	embeddings := extractor.NewEmbeddingsGenerator(cfg)
	engine := NewEngine(database, embeddings, mockLLM.URL, "test-model", "ollama", false)

	// 1. Dry Run test
	dryRunSummaries, err := engine.RunConsolidation(context.Background(), "global", true)
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}

	if len(dryRunSummaries) != 1 {
		t.Errorf("expected 1 scope summary for dry run, got %d", len(dryRunSummaries))
	} else {
		s := dryRunSummaries[0]
		if s.Scope != "global" {
			t.Errorf("expected global scope, got %s", s.Scope)
		}
		if len(s.NewMemories) != 1 {
			t.Errorf("expected 1 proposed consolidated memory, got %d", len(s.NewMemories))
		}
		if len(s.ArchivedMemoryIDs) != 3 { // mem-1, mem-2 (replaced) + mem-3 (discarded)
			t.Errorf("expected 3 archived memory proposals, got %d", len(s.ArchivedMemoryIDs))
		}
	}

	// Verify database was NOT changed during dry run
	rawMems, err := database.GetRawMemories()
	if err != nil {
		t.Fatalf("failed to get raw memories: %v", err)
	}
	if len(rawMems) != 4 {
		t.Errorf("expected 4 raw memories after dry run, got %d", len(rawMems))
	}

	// 2. Real run for scope "global"
	realSummaries, err := engine.RunConsolidation(context.Background(), "global", false)
	if err != nil {
		t.Fatalf("real run failed: %v", err)
	}

	if len(realSummaries) != 1 {
		t.Fatalf("expected 1 scope summary for real run, got %d", len(realSummaries))
	}

	// Verify database changes
	// raw memories remaining should be: mem-4 (different scope)
	rawRemaining, err := database.GetRawMemories()
	if err != nil {
		t.Fatalf("failed to get raw memories: %v", err)
	}
	if len(rawRemaining) != 1 || rawRemaining[0].ID != "mem-4" {
		t.Errorf("expected only mem-4 to remain raw, got %v", rawRemaining)
	}

	// ListMemories should return:
	// - the new consolidated memory (status consolidated)
	// - mem-4 (status raw)
	// (excluding mem-1, mem-2, mem-3 because they are now archived)
	activeList, err := database.ListMemories("", 0, 10)
	if err != nil {
		t.Fatalf("failed to list memories: %v", err)
	}
	if len(activeList) != 2 {
		t.Errorf("expected 2 active memories, got %d", len(activeList))
	}

	// Retrieve archived memories directly to check fields
	m1Archived, err := database.GetMemory("mem-1")
	if err != nil {
		t.Fatalf("failed to get mem-1: %v", err)
	}
	if m1Archived.ConsolidationStatus != "archived" {
		t.Errorf("expected mem-1 status 'archived', got '%s'", m1Archived.ConsolidationStatus)
	}
	if m1Archived.ConsolidatedIntoID == "" {
		t.Errorf("expected mem-1 to link to new consolidated memory, got empty ConsolidatedIntoID")
	}

	m3Archived, err := database.GetMemory("mem-3")
	if err != nil {
		t.Fatalf("failed to get mem-3: %v", err)
	}
	if m3Archived.ConsolidationStatus != "archived" {
		t.Errorf("expected mem-3 status 'archived', got '%s'", m3Archived.ConsolidationStatus)
	}
	if m3Archived.ConsolidatedIntoID != "" {
		t.Errorf("expected mem-3 (discarded) ConsolidatedIntoID to be empty, got '%s'", m3Archived.ConsolidatedIntoID)
	}

	// 3. Run consolidation for "project-a" (single memory -> should be marked consolidated without LLM call)
	origServerCalls := serverCalled
	projectSummaries, err := engine.RunConsolidation(context.Background(), "project-a", false)
	if err != nil {
		t.Fatalf("project consolidation failed: %v", err)
	}

	if len(projectSummaries) != 1 {
		t.Fatalf("expected 1 project summary, got %d", len(projectSummaries))
	}
	if serverCalled != origServerCalls {
		t.Errorf("expected 0 LLM calls for single-memory consolidation, but server was called")
	}

	// Verify mem-4 has status consolidated now
	mem4, err := database.GetMemory("mem-4")
	if err != nil {
		t.Fatalf("failed to get mem-4: %v", err)
	}
	if mem4.ConsolidationStatus != "consolidated" {
		t.Errorf("expected mem-4 status 'consolidated', got '%s'", mem4.ConsolidationStatus)
	}
}
