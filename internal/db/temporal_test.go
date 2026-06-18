package db

import (
	"testing"
	"time"

	"github.com/danieljustus/symaira-memory/internal/config"
)

func TestTemporalFields_RoundTrip(t *testing.T) {
	cfg := config.Defaults()
	cfg.Database.Path = t.TempDir() + "/test.db"
	database, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()
	m := &Memory{
		ID:        "temporal-test",
		Content:   "User lives in Berlin",
		Scope:     "global",
		Metadata:  map[string]string{},
		ValidFrom: &now,
	}
	if err := database.SaveMemory(m); err != nil {
		t.Fatal(err)
	}

	got, err := database.GetMemory("temporal-test")
	if err != nil {
		t.Fatal(err)
	}
	if got.ValidFrom == nil {
		t.Fatal("expected valid_from to be set")
	}
	if got.ValidTo != nil {
		t.Error("expected valid_to to be nil for current fact")
	}
	if got.SupersededBy != "" {
		t.Error("expected superseded_by to be empty")
	}
}

func TestSupersedeFact(t *testing.T) {
	cfg := config.Defaults()
	cfg.Database.Path = t.TempDir() + "/test.db"
	database, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	old := &Memory{
		ID:       "old-fact",
		Content:  "User lives in Berlin",
		Scope:    "global",
		Metadata: map[string]string{},
	}
	if err := database.SaveMemory(old); err != nil {
		t.Fatal(err)
	}

	new_ := &Memory{
		ID:       "new-fact",
		Content:  "User lives in Munich",
		Scope:    "global",
		Metadata: map[string]string{},
	}
	if err := database.SaveMemory(new_); err != nil {
		t.Fatal(err)
	}

	if err := database.SupersedeFact("old-fact", "new-fact"); err != nil {
		t.Fatal(err)
	}

	superseded, err := database.GetMemory("old-fact")
	if err != nil {
		t.Fatal(err)
	}
	if superseded.SupersededBy != "new-fact" {
		t.Errorf("expected superseded_by='new-fact', got %q", superseded.SupersededBy)
	}
	if superseded.ValidTo == nil {
		t.Error("expected valid_to to be set on superseded fact")
	}

	history, err := database.GetSupersededHistory("new-fact")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 {
		t.Fatalf("expected 1 superseded history entry, got %d", len(history))
	}
	if history[0].ID != "old-fact" {
		t.Errorf("expected history entry to be 'old-fact', got %q", history[0].ID)
	}
}

func TestTemporalFields_NilDefaults(t *testing.T) {
	cfg := config.Defaults()
	cfg.Database.Path = t.TempDir() + "/test.db"
	database, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	m := &Memory{
		ID:       "no-temporal",
		Content:  "A simple fact",
		Scope:    "global",
		Metadata: map[string]string{},
	}
	if err := database.SaveMemory(m); err != nil {
		t.Fatal(err)
	}

	got, err := database.GetMemory("no-temporal")
	if err != nil {
		t.Fatal(err)
	}
	if got.ValidFrom == nil {
		t.Error("expected valid_from to be auto-set on save")
	}
}
