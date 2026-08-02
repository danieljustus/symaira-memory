package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/db"
)

// TestSortedKeysOrdering covers cmd/query_log.go sortedKeys: alphabetical
// ordering, empty map, single key.
func TestSortedKeysOrdering(t *testing.T) {
	if got := sortedKeys(map[string]int{}); len(got) != 0 {
		t.Errorf("expected empty slice for empty map, got %v", got)
	}

	m := map[string]int{"zebra": 1, "alpha": 2, "mike": 3}
	got := sortedKeys(m)
	want := []string{"alpha", "mike", "zebra"}
	if len(got) != len(want) {
		t.Fatalf("expected %d keys, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("keys[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

// TestQueryLogCmdJSONOutput runs the query-log command in JSON mode against a
// seeded database and verifies the envelope and breakdown fields.
func TestQueryLogCmdJSONOutput(t *testing.T) {
	database := helperTestDB(t)
	SetConfig(config.Defaults())
	SetDB(database)
	t.Cleanup(func() { SetDB(nil) })

	if err := database.LogQuery("memory_search", "hello world", "scope-a", 12); err != nil {
		t.Fatalf("LogQuery: %v", err)
	}
	if err := database.LogQuery("entity_resolve", "alice", "scope-b", 5); err != nil {
		t.Fatalf("LogQuery: %v", err)
	}

	queryLogJSON = true
	queryLogLimit = 10
	t.Cleanup(func() { queryLogJSON = false })

	out := captureCmdOutput(func() {
		if err := queryLogCmd.RunE(queryLogCmd, nil); err != nil {
			t.Fatalf("RunE: %v", err)
		}
	})

	var summary db.QueryLogSummary
	if err := json.Unmarshal([]byte(out), &summary); err != nil {
		t.Fatalf("invalid JSON output %q: %v", out, err)
	}
	if summary.TotalQueries != 2 {
		t.Errorf("TotalQueries = %d, want 2", summary.TotalQueries)
	}
	if summary.ToolBreakdown["memory_search"] != 1 {
		t.Errorf("ToolBreakdown[memory_search] = %d, want 1", summary.ToolBreakdown["memory_search"])
	}
	if summary.ToolBreakdown["entity_resolve"] != 1 {
		t.Errorf("ToolBreakdown[entity_resolve] = %d, want 1", summary.ToolBreakdown["entity_resolve"])
	}
}

// TestQueryLogCmdTableOutput verifies the human-readable table contains the
// expected headers and tool names in sorted order.
func TestQueryLogCmdTableOutput(t *testing.T) {
	database := helperTestDB(t)
	SetConfig(config.Defaults())
	SetDB(database)
	t.Cleanup(func() { SetDB(nil) })

	if err := database.LogQuery("zeta_tool", "q", "s", 1); err != nil {
		t.Fatalf("LogQuery: %v", err)
	}
	if err := database.LogQuery("alpha_tool", "q", "s", 2); err != nil {
		t.Fatalf("LogQuery: %v", err)
	}

	queryLogJSON = false
	queryLogLimit = 10

	out := captureCmdOutput(func() {
		if err := queryLogCmd.RunE(queryLogCmd, nil); err != nil {
			t.Fatalf("RunE: %v", err)
		}
	})

	for _, want := range []string{"Query Log Summary", "Tool Breakdown", "alpha_tool", "zeta_tool"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in table output, got:\n%s", want, out)
		}
	}
	if strings.Index(out, "alpha_tool") > strings.Index(out, "zeta_tool") {
		t.Errorf("expected alpha_tool before zeta_tool (sorted), got:\n%s", out)
	}
}
