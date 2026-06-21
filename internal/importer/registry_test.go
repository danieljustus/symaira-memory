package importer

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/db"
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

func TestStoreFactsPIIRedaction(t *testing.T) {
	database := helperRegistryDB(t)
	r := NewRegistry(database)

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
	r := NewRegistry(database)

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
	r := NewRegistry(database)

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
