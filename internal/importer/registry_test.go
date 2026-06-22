package importer

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/extractor"
	"os"
)

func helperRegistryDB(t *testing.T) *db.DB {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "symmemory-importer-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tempDir) })

	cfg := config.Defaults()
	cfg.Database.Path = filepath.Join(tempDir, "test.db")

	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func helperRegistry(t *testing.T, database *db.DB) *Registry {
	t.Helper()
	return NewRegistry(database, extractor.NewEmbeddingsGenerator(config.Defaults()))
}

func TestStoreFactsPIIRedaction(t *testing.T) {
	database := helperRegistryDB(t)
	r := helperRegistry(t, database)

	facts := []ImportedFact{
		{
			Content:   "Contact alice@example.com for API key ghp_abcdefghijklmnopqrstuvwxyz0123456789",
			Source:    "test-importer",
			SessionID: "sess-001",
			Timestamp: time.Now().UTC(),
			Metadata:  map[string]string{"extra": "info"},
		},
	}

	if err := r.storeFacts(facts); err != nil {
		t.Fatalf("storeFacts failed: %v", err)
	}

	memories, err := database.ListMemoriesLite("", 0, 10)
	if err != nil {
		t.Fatalf("ListMemoriesLite failed: %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(memories))
	}

	m := memories[0]
	if strings.Contains(m.Content, "alice@example.com") {
		t.Errorf("expected content email redacted, got %q", m.Content)
	}
	if strings.Contains(m.Content, "ghp_") {
		t.Errorf("expected content API key redacted, got %q", m.Content)
	}
}

func TestStoreFactsMetadataPIIRedaction(t *testing.T) {
	database := helperRegistryDB(t)
	r := helperRegistry(t, database)

	facts := []ImportedFact{
		{
			Content:   "clean content",
			Source:    "test-importer",
			SessionID: "sess-002",
			Timestamp: time.Now().UTC(),
			Metadata:  map[string]string{"contact": "bob@example.com"},
		},
	}

	if err := r.storeFacts(facts); err != nil {
		t.Fatalf("storeFacts failed: %v", err)
	}

	memories, err := database.ListMemoriesLite("", 0, 10)
	if err != nil {
		t.Fatalf("ListMemoriesLite failed: %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(memories))
	}

	m := memories[0]
	if m.Metadata["contact"] != "[REDACTED_EMAIL]" {
		t.Errorf("expected metadata email redacted, got %q", m.Metadata["contact"])
	}
	if m.Metadata["source"] != "test-importer" {
		t.Errorf("expected source preserved, got %q", m.Metadata["source"])
	}
}

func TestStoreFactsNilMetadata(t *testing.T) {
	database := helperRegistryDB(t)
	r := helperRegistry(t, database)

	facts := []ImportedFact{
		{
			Content:   "test content",
			Source:    "test-importer",
			SessionID: "sess-003",
			Timestamp: time.Now().UTC(),
			Metadata:  nil,
		},
	}

	if err := r.storeFacts(facts); err != nil {
		t.Fatalf("storeFacts failed: %v", err)
	}

	memories, err := database.ListMemoriesLite("", 0, 10)
	if err != nil {
		t.Fatalf("ListMemoriesLite failed: %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(memories))
	}

	m := memories[0]
	if m.Metadata["source"] != "test-importer" {
		t.Errorf("expected source populated from nil metadata, got %q", m.Metadata["source"])
	}
}

func TestStoreFactsGeneratesEmbedding(t *testing.T) {
	database := helperRegistryDB(t)
	r := helperRegistry(t, database)

	facts := []ImportedFact{
		{
			Content:   "Alice prefers dark mode in all applications",
			Source:    "test-importer",
			SessionID: "sess-emb-001",
			Timestamp: time.Now().UTC(),
		},
	}

	if err := r.storeFacts(facts); err != nil {
		t.Fatalf("storeFacts failed: %v", err)
	}

	memories, err := database.ListMemories("", 0, 10)
	if err != nil {
		t.Fatalf("ListMemories failed: %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(memories))
	}

	m := memories[0]
	if len(m.Embedding) == 0 {
		t.Fatal("expected embedding to be generated for imported memory")
	}
	if len(m.Embedding) != 768 {
		t.Errorf("expected 768-dim embedding, got %d", len(m.Embedding))
	}
}

func TestRunImportUnknownToolError(t *testing.T) {
	database := helperRegistryDB(t)
	r := helperRegistry(t, database)

	_, err := r.RunImport(t.Context(), []string{"does-not-exist"}, true)
	if err == nil {
		t.Fatal("expected error for unknown importer")
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("expected error to name unknown importer, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "valid importers") {
		t.Errorf("expected error to list valid importers, got %q", err.Error())
	}
}

func TestImportedMemorySearchable(t *testing.T) {
	database := helperRegistryDB(t)
	r := helperRegistry(t, database)

	content := "Alice prefers dark mode in all applications"
	facts := []ImportedFact{
		{
			Content:   content,
			Source:    "test-importer",
			SessionID: "sess-search-001",
			Timestamp: time.Now().UTC(),
		},
	}

	if err := r.storeFacts(facts); err != nil {
		t.Fatalf("storeFacts failed: %v", err)
	}

	memories, err := database.ListMemories("", 0, 10)
	if err != nil {
		t.Fatalf("ListMemories failed: %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(memories))
	}

	m := memories[0]
	if len(m.Embedding) == 0 {
		t.Fatal("expected imported memory to have a non-empty embedding")
	}

	eg := extractor.NewEmbeddingsGenerator(config.Defaults())
	emb := eg.GenerateVector(content)
	queryVec := emb.Vector

	results, err := database.SearchMemories(queryVec, emb.Source, "", 10)
	if err != nil {
		t.Fatalf("SearchMemories failed: %v", err)
	}

	found := false
	for _, r := range results {
		if r.Memory.ID == m.ID {
			found = true
			if len(r.Memory.Embedding) == 0 {
				t.Error("search result should include embedding data")
			}
			break
		}
	}
	if !found {
		t.Errorf("imported memory not found via semantic search (LSH may not match for local hash vectors; embedding is %d-dim)", len(m.Embedding))
	}
}
