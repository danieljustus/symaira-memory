package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/db"
)

// agingTestSetup builds a temp DB and wires it into the CLI context,
// mirroring the other command test setups.
func agingTestSetup(t *testing.T) *db.DB {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "symmemory-aging-test-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tempDir) })
	setTestHome(t, tempDir)

	database := helperTestDB(t)
	SetConfig(config.Defaults())
	SetDB(database)
	return database
}

func TestAgingRunDryRun(t *testing.T) {
	agingTestSetup(t)
	if err := agingRunCmd.Flags().Set("dry-run", "true"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = agingRunCmd.Flags().Set("dry-run", "false") })

	out := captureCmdOutput(func() {
		if err := agingRunCmd.RunE(agingRunCmd, nil); err != nil {
			t.Errorf("aging run error: %v", err)
		}
	})
	if !strings.Contains(out, "aging pass") || !strings.Contains(out, "[dry-run") {
		t.Fatalf("aging dry-run output missing report marker, got %q", out)
	}
}

func TestAgingRunDatabaseUnavailable(t *testing.T) {
	prev := GetDB()
	SetDB(nil)
	t.Cleanup(func() { SetDB(prev) })

	err := agingRunCmd.RunE(agingRunCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "database not available") {
		t.Fatalf("aging run without a database must fail with a clear error, got %v", err)
	}
}
