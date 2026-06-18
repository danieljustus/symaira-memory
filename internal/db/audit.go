package db

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type AuditEvent struct {
	ID        string    `json:"id"`
	Action    string    `json:"action"`
	MemoryID  string    `json:"memory_id,omitempty"`
	Scope     string    `json:"scope,omitempty"`
	Session   string    `json:"session,omitempty"`
	Actor     string    `json:"actor,omitempty"`
	Detail    string    `json:"detail,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func (db *DB) LogAudit(action, memoryID, scope, session, actor, detail string) error {
	if _, err := db.conn.Exec(
		`INSERT INTO audit_log (id, action, memory_id, scope, session, actor, detail, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), action, nullStr(memoryID), nullStr(scope),
		nullStr(session), nullStr(actor), nullStr(detail), time.Now().UTC(),
	); err != nil {
		return err
	}
	return nil
}

func (db *DB) GetAuditLogs(action string, limit int) ([]*AuditEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	var rows *sql.Rows
	var err error
	if action != "" {
		rows, err = db.conn.Query(
			"SELECT id, action, memory_id, scope, session, actor, detail, created_at FROM audit_log WHERE action = ? ORDER BY created_at DESC LIMIT ?",
			action, limit,
		)
	} else {
		rows, err = db.conn.Query(
			"SELECT id, action, memory_id, scope, session, actor, detail, created_at FROM audit_log ORDER BY created_at DESC LIMIT ?",
			limit,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*AuditEvent
	for rows.Next() {
		var e AuditEvent
		var memID, sc, sess, act, det sql.NullString
		if err := rows.Scan(&e.ID, &e.Action, &memID, &sc, &sess, &act, &det, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.MemoryID = memID.String
		e.Scope = sc.String
		e.Session = sess.String
		e.Actor = act.String
		e.Detail = det.String
		events = append(events, &e)
	}
	return events, nil
}

func (db *DB) PurgeExpiredMemories(ttl time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-ttl)
	result, err := db.conn.Exec(
		"DELETE FROM memories WHERE scope = 'session' AND created_at < ? AND consolidation_status != 'archived'",
		cutoff,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (db *DB) PurgeExpiredSessions(ttl time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-ttl)
	result, err := db.conn.Exec(
		"DELETE FROM sessions WHERE updated_at < ?",
		cutoff,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (db *DB) PurgeByScope(scope string) (int64, error) {
	result, err := db.conn.Exec(
		"DELETE FROM memories WHERE scope = ? AND consolidation_status != 'archived'",
		scope,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (db *DB) PurgeByID(id string) (bool, error) {
	result, err := db.conn.Exec("DELETE FROM memories WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	n, _ := result.RowsAffected()
	return n > 0, nil
}

func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
