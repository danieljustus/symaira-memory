package cmd

import (
	"strings"
	"testing"

	"github.com/danieljustus/symaira-memory/internal/config"
)

// TestConsolidateNoMemories covers the empty-result path: no raw memories to
// consolidate produces the plain text message and no error.
func TestConsolidateNoMemories(t *testing.T) {
	database := helperTestDB(t)
	SetConfig(config.Defaults())
	SetDB(database)
	t.Cleanup(func() { SetDB(nil) })

	consolidateDryRun = false
	consolidateUndo = false
	consolidateScope = ""

	out := captureCmdOutput(func() {
		if err := consolidateCmd.RunE(consolidateCmd, nil); err != nil {
			t.Fatalf("RunE: %v", err)
		}
	})
	if !strings.Contains(out, "No raw memories to consolidate") {
		t.Errorf("expected empty-state message, got %q", out)
	}
}

// TestConsolidateUndoNoRun covers the undo path when no completed run exists:
// the command surfaces the engine error instead of panicking.
func TestConsolidateUndoNoRun(t *testing.T) {
	database := helperTestDB(t)
	SetConfig(config.Defaults())
	SetDB(database)
	t.Cleanup(func() { SetDB(nil) })

	consolidateUndo = true
	t.Cleanup(func() { consolidateUndo = false })

	err := consolidateCmd.RunE(consolidateCmd, nil)
	if err == nil {
		t.Fatal("expected error when undoing without a completed run")
	}
}
