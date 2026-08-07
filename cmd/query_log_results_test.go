package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/db"
)

func TestQueryLogResultsCommandRegistered(t *testing.T) {
	for _, sub := range queryLogCmd.Commands() {
		if sub.Use == "results [query-id]" || sub.Name() == "results" {
			if sub.Short == "" {
				t.Error("query-log results command has empty Short description")
			}
			return
		}
	}
	t.Error("query-log results subcommand not registered")
}

func TestQueryLogResultsOutputsReturnedMemories(t *testing.T) {
	database := helperTestDB(t)
	SetConfig(config.Defaults())
	SetDB(database)
	t.Cleanup(func() {
		SetDB(nil)
		SetConfig(nil)
	})

	if err := database.SaveMemory(&db.Memory{
		ID:       "mem-results-cmd",
		Content:  "Traceable fact.",
		Scope:    "global",
		Metadata: map[string]string{},
	}); err != nil {
		t.Fatalf("SaveMemory failed: %v", err)
	}

	queryID, err := database.LogQuery("mcp", "global", "", "memory_search", "traceable", "", 3)
	if err != nil {
		t.Fatalf("LogQuery failed: %v", err)
	}
	if err := database.RecordQueryResults(queryID, []db.QueryResultRef{{MemoryID: "mem-results-cmd", Rank: 0, Score: 0.87}}); err != nil {
		t.Fatalf("RecordQueryResults failed: %v", err)
	}

	oldJSON := queryLogJSON
	queryLogJSON = false
	defer func() { queryLogJSON = oldJSON }()

	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = queryLogResultsCmd.RunE(queryLogResultsCmd, []string{queryID})

	_ = w.Close()
	os.Stdout = oldStdout
	buf.ReadFrom(r)

	if err != nil {
		t.Fatalf("query-log results returned error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "mem-results-cmd") {
		t.Errorf("output missing the returned memory id:\n%s", out)
	}
	if !strings.Contains(out, "0.8700") {
		t.Errorf("output missing the score:\n%s", out)
	}
}

func TestQueryLogResultsUnknownQuery(t *testing.T) {
	database := helperTestDB(t)
	SetConfig(config.Defaults())
	SetDB(database)
	t.Cleanup(func() {
		SetDB(nil)
		SetConfig(nil)
	})

	oldJSON := queryLogJSON
	queryLogJSON = true
	defer func() { queryLogJSON = oldJSON }()

	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := queryLogResultsCmd.RunE(queryLogResultsCmd, []string{"no-such-query"})

	_ = w.Close()
	os.Stdout = oldStdout
	buf.ReadFrom(r)

	if err != nil {
		t.Fatalf("query-log results returned error for unknown id: %v", err)
	}
	out := strings.TrimSpace(buf.String())
	if out != "[]" {
		t.Errorf("expected empty JSON array for unknown query, got %q", out)
	}
}
