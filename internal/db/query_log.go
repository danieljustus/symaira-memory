package db

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// QueryLogEntry represents a single query log row.
type QueryLogEntry struct {
	ID         string    `json:"id"`
	Tool       string    `json:"tool"`
	QueryText  string    `json:"query_text,omitempty"`
	Params     string    `json:"params,omitempty"`
	DurationMs int64     `json:"duration_ms"`
	CreatedAt  time.Time `json:"created_at"`
}

// QueryLogSummary holds aggregated stats for the query-log summary CLI.
type QueryLogSummary struct {
	TotalQueries   int               `json:"total_queries"`
	ToolBreakdown  map[string]int    `json:"tool_breakdown"`
	RecentEntries  []*QueryLogEntry  `json:"recent_entries"`
	PrunedCount    int               `json:"pruned_count,omitempty"`
}

const maxQueryLogEntries = 1000

// LogQuery inserts a new query log entry and prunes old entries
// when the table exceeds maxQueryLogEntries.
func (db *DB) LogQuery(tool, queryText, params string, durationMs int64) error {
	id := uuid.New().String()
	now := time.Now().UTC()

	if _, err := db.conn.Exec(
		`INSERT INTO query_log (id, tool, query_text, params, duration_ms, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id, tool, nullStr(queryText), nullStr(params), durationMs, now,
	); err != nil {
		return err
	}

	return db.pruneQueryLog()
}

// pruneQueryLog removes the oldest entries when the table exceeds maxQueryLogEntries.
func (db *DB) pruneQueryLog() error {
	var count int
	if err := db.conn.QueryRow("SELECT COUNT(*) FROM query_log").Scan(&count); err != nil {
		return err
	}
	if count <= maxQueryLogEntries {
		return nil
	}
	excess := count - maxQueryLogEntries
	_, err := db.conn.Exec(
		`DELETE FROM query_log WHERE id IN (
			SELECT id FROM query_log ORDER BY created_at ASC LIMIT ?
		)`, excess,
	)
	return err
}

// GetQueryLogEntries returns the most recent query log entries.
func (db *DB) GetQueryLogEntries(limit int) ([]*QueryLogEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > maxQueryLogEntries {
		limit = maxQueryLogEntries
	}

	rows, err := db.conn.Query(
		`SELECT id, tool, query_text, params, duration_ms, created_at
		 FROM query_log ORDER BY created_at DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*QueryLogEntry
	for rows.Next() {
		var e QueryLogEntry
		var qt, p sql.NullString
		if err := rows.Scan(&e.ID, &e.Tool, &qt, &p, &e.DurationMs, &e.CreatedAt); err != nil {
			return nil, err
		}
		if qt.Valid {
			e.QueryText = qt.String
		}
		if p.Valid {
			e.Params = p.String
		}
		entries = append(entries, &e)
	}
	return entries, nil
}

// GetQueryLogSummary aggregates query log data for the CLI summary.
func (db *DB) GetQueryLogSummary(limit int) (*QueryLogSummary, error) {
	var total int
	if err := db.conn.QueryRow("SELECT COUNT(*) FROM query_log").Scan(&total); err != nil {
		return nil, err
	}

	summary := &QueryLogSummary{
		TotalQueries:  total,
		ToolBreakdown: make(map[string]int),
	}

	// Tool breakdown
	rows, err := db.conn.Query("SELECT tool, COUNT(*) as cnt FROM query_log GROUP BY tool ORDER BY cnt DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var tool string
		var cnt int
		if err := rows.Scan(&tool, &cnt); err != nil {
			return nil, err
		}
		summary.ToolBreakdown[tool] = cnt
	}

	// Recent entries
	recent, err := db.GetQueryLogEntries(limit)
	if err != nil {
		return nil, err
	}
	summary.RecentEntries = recent

	return summary, nil
}

// QueryLogPruneCount returns how many entries would be pruned if the table
// exceeds maxQueryLogEntries. Used by diagnostics/CLI.
func (db *DB) QueryLogPruneCount() (int, error) {
	var count int
	if err := db.conn.QueryRow("SELECT COUNT(*) FROM query_log").Scan(&count); err != nil {
		return 0, err
	}
	if count <= maxQueryLogEntries {
		return 0, nil
	}
	return count - maxQueryLogEntries, nil
}
