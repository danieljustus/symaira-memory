package db

import (
	"fmt"
	"testing"
	"time"
)

// insertQueryLogRows seeds query_log rows directly with controllable
// created_at values (increasing by one minute per row, starting at base).
func insertQueryLogRows(t *testing.T, database *DB, n int, base time.Time, prefix string) {
	t.Helper()
	tx, err := database.conn.Begin()
	if err != nil {
		t.Fatalf("failed to begin tx: %v", err)
	}
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("%s-%04d", prefix, i)
		if _, err := tx.Exec(
			`INSERT INTO query_log (id, tool, query_text, params, duration_ms, created_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			id, "memory_search", nullStr("query "+id), nullStr(`{"k":"v"}`), int64(i), base.Add(time.Duration(i)*time.Minute),
		); err != nil {
			tx.Rollback()
			t.Fatalf("failed to insert query log row %s: %v", id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("failed to commit tx: %v", err)
	}
}

func TestLogQueryInsertsEntryWithToolAttribution(t *testing.T) {
	database := openTestDB(t)

	if err := database.LogQuery("memory_search", "what is symmemory", `{"limit":5}`, 42); err != nil {
		t.Fatalf("LogQuery: %v", err)
	}
	if err := database.LogQuery("memory_get", "", "", 7); err != nil {
		t.Fatalf("LogQuery: %v", err)
	}

	entries, err := database.GetQueryLogEntries(10)
	if err != nil {
		t.Fatalf("GetQueryLogEntries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	byTool := map[string]*QueryLogEntry{}
	for _, e := range entries {
		byTool[e.Tool] = e
	}

	search := byTool["memory_search"]
	if search == nil {
		t.Fatalf("expected memory_search entry, got %v", byTool)
	}
	if search.QueryText != "what is symmemory" || search.Params != `{"limit":5}` || search.DurationMs != 42 {
		t.Errorf("memory_search entry mismatch: %+v", search)
	}
	if search.ID == "" || search.CreatedAt.IsZero() {
		t.Errorf("expected non-empty id and created_at, got %+v", search)
	}

	// Empty query text and params are stored as NULL and read back as "".
	get := byTool["memory_get"]
	if get == nil {
		t.Fatalf("expected memory_get entry, got %v", byTool)
	}
	if get.QueryText != "" || get.Params != "" {
		t.Errorf("expected empty query/params for NULL columns, got %+v", get)
	}
}

func TestPruneQueryLogRetentionCap(t *testing.T) {
	database := openTestDB(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// 1005 rows: 5 more than the 1000-entry cap.
	insertQueryLogRows(t, database, maxQueryLogEntries+5, base, "prune")

	if err := database.pruneQueryLog(); err != nil {
		t.Fatalf("pruneQueryLog: %v", err)
	}

	var count int
	if err := database.conn.QueryRow(`SELECT COUNT(*) FROM query_log`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != maxQueryLogEntries {
		t.Fatalf("expected %d rows after prune, got %d", maxQueryLogEntries, count)
	}

	// The 5 oldest entries (ids prune-0000..prune-0004) must be gone.
	for _, id := range []string{"prune-0000", "prune-0002", "prune-0004"} {
		var exists int
		if err := database.conn.QueryRow(`SELECT COUNT(*) FROM query_log WHERE id = ?`, id).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if exists != 0 {
			t.Errorf("expected pruned id %s to be gone", id)
		}
	}
	// The first surviving entry must be prune-0005.
	var firstID string
	if err := database.conn.QueryRow(`SELECT id FROM query_log ORDER BY created_at ASC LIMIT 1`).Scan(&firstID); err != nil {
		t.Fatal(err)
	}
	if firstID != "prune-0005" {
		t.Errorf("expected prune-0005 as oldest survivor, got %s", firstID)
	}

	// Pruning at or below the cap is a no-op.
	if err := database.pruneQueryLog(); err != nil {
		t.Fatalf("pruneQueryLog (no-op): %v", err)
	}
	if err := database.conn.QueryRow(`SELECT COUNT(*) FROM query_log`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != maxQueryLogEntries {
		t.Errorf("expected count unchanged after no-op prune, got %d", count)
	}
}

func TestGetQueryLogEntriesLimitAndOrdering(t *testing.T) {
	database := openTestDB(t)
	base := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	insertQueryLogRows(t, database, 5, base, "ord")

	// Most recent first, newest id = ord-0004.
	entries, err := database.GetQueryLogEntries(3)
	if err != nil {
		t.Fatalf("GetQueryLogEntries(3): %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	for i, e := range entries {
		wantID := fmt.Sprintf("ord-%04d", 4-i)
		if e.ID != wantID {
			t.Errorf("entry %d: expected %s, got %s", i, wantID, e.ID)
		}
		if e.CreatedAt.IsZero() {
			t.Errorf("entry %d: zero created_at", i)
		}
	}

	// Non-positive limit defaults to 50 → all 5.
	for _, limit := range []int{0, -1} {
		all, err := database.GetQueryLogEntries(limit)
		if err != nil {
			t.Fatalf("GetQueryLogEntries(%d): %v", limit, err)
		}
		if len(all) != 5 {
			t.Errorf("GetQueryLogEntries(%d): expected 5, got %d", limit, len(all))
		}
	}

	// Oversized limit is capped at maxQueryLogEntries.
	all, err := database.GetQueryLogEntries(maxQueryLogEntries + 100)
	if err != nil {
		t.Fatalf("GetQueryLogEntries(cap): %v", err)
	}
	if len(all) != 5 {
		t.Errorf("capped limit: expected 5, got %d", len(all))
	}
}

func TestGetQueryLogSummaryAggregation(t *testing.T) {
	database := openTestDB(t)
	base := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	tx, err := database.conn.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		tool := "memory_search"
		if i >= 3 {
			tool = "memory_get"
		}
		if _, err := tx.Exec(
			`INSERT INTO query_log (id, tool, query_text, params, duration_ms, created_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			fmt.Sprintf("sum-%04d", i), tool, nullStr("q"), nullStr("{}"), int64(i), base.Add(time.Duration(i)*time.Minute),
		); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	summary, err := database.GetQueryLogSummary(10)
	if err != nil {
		t.Fatalf("GetQueryLogSummary: %v", err)
	}
	if summary.TotalQueries != 5 {
		t.Errorf("TotalQueries = %d, want 5", summary.TotalQueries)
	}
	if summary.ToolBreakdown["memory_search"] != 3 || summary.ToolBreakdown["memory_get"] != 2 {
		t.Errorf("ToolBreakdown = %v, want {memory_search:3 memory_get:2}", summary.ToolBreakdown)
	}
	if len(summary.RecentEntries) != 5 {
		t.Errorf("RecentEntries len = %d, want 5", len(summary.RecentEntries))
	}
	if summary.RecentEntries[0].ID != "sum-0004" {
		t.Errorf("newest recent entry = %s, want sum-0004", summary.RecentEntries[0].ID)
	}

	// Limit applies to recent entries only.
	limited, err := database.GetQueryLogSummary(2)
	if err != nil {
		t.Fatalf("GetQueryLogSummary(2): %v", err)
	}
	if limited.TotalQueries != 5 {
		t.Errorf("limited TotalQueries = %d, want 5", limited.TotalQueries)
	}
	if len(limited.RecentEntries) != 2 {
		t.Errorf("limited RecentEntries len = %d, want 2", len(limited.RecentEntries))
	}
}

func TestQueryLogPruneCount(t *testing.T) {
	database := openTestDB(t)
	base := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	// Empty table.
	n, err := database.QueryLogPruneCount()
	if err != nil {
		t.Fatalf("QueryLogPruneCount: %v", err)
	}
	if n != 0 {
		t.Errorf("empty table prune count = %d, want 0", n)
	}

	// Exactly at cap.
	insertQueryLogRows(t, database, maxQueryLogEntries, base, "pc-cap")
	n, err = database.QueryLogPruneCount()
	if err != nil {
		t.Fatalf("QueryLogPruneCount: %v", err)
	}
	if n != 0 {
		t.Errorf("at-cap prune count = %d, want 0", n)
	}

	// Over the cap by 5.
	insertQueryLogRows(t, database, 5, base.Add(time.Duration(maxQueryLogEntries)*time.Minute), "pc-over")
	n, err = database.QueryLogPruneCount()
	if err != nil {
		t.Fatalf("QueryLogPruneCount: %v", err)
	}
	if n != 5 {
		t.Errorf("over-cap prune count = %d, want 5", n)
	}
}
