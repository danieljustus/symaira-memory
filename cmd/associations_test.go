package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/db"
)

func TestAssociationsSeedCLI(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "symmemory-assoc-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(tempDir) })
	setTestHome(t, tempDir)

	database := helperTestDB(t)
	SetConfig(config.Defaults())
	SetDB(database)

	// Two memories returned together by one query → one co-retrieval edge.
	m1 := &db.Memory{ID: "m1", Content: "one", Scope: "global", Metadata: map[string]string{}}
	m2 := &db.Memory{ID: "m2", Content: "two", Scope: "global", Metadata: map[string]string{}}
	for _, m := range []*db.Memory{m1, m2} {
		if err := database.SaveMemory(m); err != nil {
			t.Fatal(err)
		}
	}
	qid, err := database.LogQuery("cli-test", "global", "sess", "memory_search", "q", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.RecordQueryResults(qid, []db.QueryResultRef{
		{MemoryID: "m1", Rank: 0, Score: 0.9},
		{MemoryID: "m2", Rank: 1, Score: 0.8},
	}); err != nil {
		t.Fatal(err)
	}

	out := captureCmdOutput(func() {
		if err := associationsSeedCmd.RunE(associationsSeedCmd, nil); err != nil {
			t.Errorf("associations seed error: %v", err)
		}
	})
	if !strings.Contains(out, "0 -> 1") {
		t.Fatalf("seed output missing edge count, got %q", out)
	}
	n, err := database.AssociationCount()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("association count = %d, want 1", n)
	}
}

func TestAssociationsSeedDryRun(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "symmemory-assoc-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(tempDir) })
	setTestHome(t, tempDir)

	database := helperTestDB(t)
	SetConfig(config.Defaults())
	SetDB(database)

	if err := associationsSeedCmd.Flags().Set("dry-run", "true"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = associationsSeedCmd.Flags().Set("dry-run", "false") })

	out := captureCmdOutput(func() {
		if err := associationsSeedCmd.RunE(associationsSeedCmd, nil); err != nil {
			t.Errorf("associations seed dry-run error: %v", err)
		}
	})
	if !strings.Contains(out, "dry-run") || !strings.Contains(out, "0 edge(s)") {
		t.Fatalf("dry-run output missing marker, got %q", out)
	}
	n, err := database.AssociationCount()
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("dry-run must not create edges, got %d", n)
	}
}

func TestAssociationsSeedDatabaseUnavailable(t *testing.T) {
	prev := GetDB()
	SetDB(nil)
	t.Cleanup(func() { SetDB(prev) })

	err := associationsSeedCmd.RunE(associationsSeedCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "database not available") {
		t.Fatalf("associations seed without a database must fail with a clear error, got %v", err)
	}
}
