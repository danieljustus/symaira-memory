package working

import (
	"context"
	"testing"
	"time"

	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/consolidation"
	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/extractor"
)

func helperWorkingDB(t *testing.T) *db.DB {
	t.Helper()
	cfg := config.Defaults()
	cfg.Database.Path = t.TempDir() + "/test.db"
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestCompactWorkingMemories_NoExpired(t *testing.T) {
	database := helperWorkingDB(t)
	cfg := config.Defaults()
	embeddings := extractor.NewEmbeddingsGenerator(cfg)
	engine := consolidation.NewEngine(database, embeddings, "", "", "", false)

	ev := NewEvictor(database, embeddings, engine, false)

	futureExpiry := time.Now().UTC().Add(24 * time.Hour)
	if err := database.SaveMemory(&db.Memory{
		ID:        "wm-active",
		Content:   "Active working memory",
		Scope:     "project",
		Tier:      "working",
		ExpiresAt: &futureExpiry,
		Metadata:  map[string]string{},
	}); err != nil {
		t.Fatalf("SaveMemory failed: %v", err)
	}

	result, err := ev.CompactWorkingMemories(context.Background(), true)
	if err != nil {
		t.Fatalf("CompactWorkingMemories failed: %v", err)
	}

	if result.ExpiredCount != 0 {
		t.Errorf("expected 0 expired, got %d", result.ExpiredCount)
	}
	if result.EvictedCount != 0 {
		t.Errorf("expected 0 evicted, got %d", result.EvictedCount)
	}
}

func TestCompactWorkingMemories_DryRun(t *testing.T) {
	database := helperWorkingDB(t)
	cfg := config.Defaults()
	embeddings := extractor.NewEmbeddingsGenerator(cfg)
	engine := consolidation.NewEngine(database, embeddings, "", "", "", false)

	ev := NewEvictor(database, embeddings, engine, false)

	pastExpiry := time.Now().UTC().Add(-1 * time.Hour)
	if err := database.SaveMemory(&db.Memory{
		ID:        "wm-expired",
		Content:   "Expired working memory",
		Scope:     "project",
		Tier:      "working",
		ExpiresAt: &pastExpiry,
		Metadata:  map[string]string{},
	}); err != nil {
		t.Fatalf("SaveMemory failed: %v", err)
	}

	result, err := ev.CompactWorkingMemories(context.Background(), true)
	if err != nil {
		t.Fatalf("CompactWorkingMemories failed: %v", err)
	}

	if result.ExpiredCount != 1 {
		t.Errorf("expected 1 expired, got %d", result.ExpiredCount)
	}
	if result.EvictedCount != 0 {
		t.Errorf("expected 0 evicted in dry run, got %d", result.EvictedCount)
	}

	got, err := database.GetMemory("wm-expired")
	if err != nil {
		t.Fatalf("GetMemory failed: %v", err)
	}
	if got == nil {
		t.Error("expired memory should NOT be deleted in dry run")
	}
}

func TestCompactWorkingMemories_RealRun(t *testing.T) {
	database := helperWorkingDB(t)
	cfg := config.Defaults()
	embeddings := extractor.NewEmbeddingsGenerator(cfg)
	engine := consolidation.NewEngine(database, embeddings, "", "", "", false)

	ev := NewEvictor(database, embeddings, engine, false)

	pastExpiry := time.Now().UTC().Add(-1 * time.Hour)
	if err := database.SaveMemory(&db.Memory{
		ID:        "wm-expired",
		Content:   "Expired working memory",
		Scope:     "project",
		Tier:      "working",
		ExpiresAt: &pastExpiry,
		Metadata:  map[string]string{},
	}); err != nil {
		t.Fatalf("SaveMemory failed: %v", err)
	}

	result, err := ev.CompactWorkingMemories(context.Background(), false)
	if err != nil {
		t.Fatalf("CompactWorkingMemories failed: %v", err)
	}

	if result.ExpiredCount != 1 {
		t.Errorf("expected 1 expired, got %d", result.ExpiredCount)
	}
	if result.EvictedCount != 1 {
		t.Errorf("expected 1 evicted, got %d", result.EvictedCount)
	}

	got, err := database.GetMemory("wm-expired")
	if err != nil {
		t.Fatalf("GetMemory failed: %v", err)
	}
	if got != nil {
		t.Error("expired memory should be deleted after real compaction")
	}
}

func TestEvictExpiredWorkingMemories(t *testing.T) {
	database := helperWorkingDB(t)
	cfg := config.Defaults()
	embeddings := extractor.NewEmbeddingsGenerator(cfg)
	engine := consolidation.NewEngine(database, embeddings, "", "", "", false)

	ev := NewEvictor(database, embeddings, engine, false)

	pastExpiry := time.Now().UTC().Add(-1 * time.Hour)
	futureExpiry := time.Now().UTC().Add(24 * time.Hour)

	if err := database.SaveMemory(&db.Memory{
		ID: "expired-1", Content: "Expired", Scope: "project",
		Tier: "working", ExpiresAt: &pastExpiry, Metadata: map[string]string{},
	}); err != nil {
		t.Fatalf("SaveMemory failed: %v", err)
	}
	if err := database.SaveMemory(&db.Memory{
		ID: "active-1", Content: "Active", Scope: "project",
		Tier: "working", ExpiresAt: &futureExpiry, Metadata: map[string]string{},
	}); err != nil {
		t.Fatalf("SaveMemory failed: %v", err)
	}

	count, err := ev.EvictExpired()
	if err != nil {
		t.Fatalf("EvictExpired failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 eviction, got %d", count)
	}

	got, err := database.GetMemory("expired-1")
	if err != nil {
		t.Fatalf("GetMemory failed: %v", err)
	}
	if got != nil {
		t.Error("expired memory should be deleted")
	}
}
