package db

import (
	"testing"
	"time"

	"github.com/danieljustus/symaira-memory/internal/config"
)

func TestAuditLogRoundTrip(t *testing.T) {
	cfg := config.Defaults()
	cfg.Database.Path = t.TempDir() + "/test.db"
	database, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	if err := database.LogAudit("set", "mem-1", "global", "session-1", "user-1", `{"key":"val"}`); err != nil {
		t.Fatal(err)
	}

	events, err := database.GetAuditLogs("set", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Action != "set" {
		t.Errorf("expected action 'set', got %q", events[0].Action)
	}
	if events[0].MemoryID != "mem-1" {
		t.Errorf("expected memory_id 'mem-1', got %q", events[0].MemoryID)
	}
}

func TestPurgeExpiredMemories(t *testing.T) {
	cfg := config.Defaults()
	cfg.Database.Path = t.TempDir() + "/test.db"
	database, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	old := &Memory{
		ID:        "old-session",
		Content:   "old fact",
		Scope:     "session",
		Metadata:  map[string]string{},
		CreatedAt: time.Now().Add(-48 * time.Hour),
	}
	if err := database.SaveMemory(old); err != nil {
		t.Fatal(err)
	}

	new_ := &Memory{
		ID:       "new-session",
		Content:  "new fact",
		Scope:    "session",
		Metadata: map[string]string{},
	}
	if err := database.SaveMemory(new_); err != nil {
		t.Fatal(err)
	}

	n, err := database.PurgeExpiredMemories(24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("expected 1 purged, got %d", n)
	}

	remaining, _ := database.GetMemory("old-session")
	if remaining != nil {
		t.Errorf("expected old-session to be purged")
	}
	still, _ := database.GetMemory("new-session")
	if still == nil {
		t.Errorf("expected new-session to remain")
	}
}
