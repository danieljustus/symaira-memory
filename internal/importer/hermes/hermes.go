package hermes

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/danieljustus/symaira-memory/internal/importer"
	_ "modernc.org/sqlite"
)

// HermesImporter imports sessions from Hermes Agent.
type HermesImporter struct {
	customPath string
}

// HermesMessage represents a message from Hermes session database.
type HermesMessage struct {
	SessionID string
	Role      string
	Content   string
	Timestamp time.Time
}

func NewHermesImporter(customPath string) *HermesImporter {
	return &HermesImporter{customPath: customPath}
}

func (h *HermesImporter) Name() string {
	return "hermes"
}

func (h *HermesImporter) DiscoverSessions(since time.Time) ([]importer.SessionRef, error) {
	dbPath := h.customPath
	if dbPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dbPath = filepath.Join(home, ".hermes", "sessions.db")
	}

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, nil
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open hermes database: %w", err)
	}
	defer db.Close()

	rows, err := db.Query(
		"SELECT DISTINCT session_id FROM messages WHERE timestamp > ? ORDER BY timestamp",
		since,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query sessions: %w", err)
	}
	defer rows.Close()

	var sessions []importer.SessionRef
	for rows.Next() {
		var sessionID string
		if err := rows.Scan(&sessionID); err != nil {
			continue
		}
		sessions = append(sessions, importer.SessionRef{
			Tool:       "hermes",
			SessionID:  sessionID,
			Path:       dbPath,
			ModifiedAt: time.Now(),
			Metadata:   map[string]string{},
		})
	}

	return sessions, nil
}

func (h *HermesImporter) ImportSession(ref importer.SessionRef) ([]importer.ImportedFact, error) {
	db, err := sql.Open("sqlite", ref.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to open hermes database: %w", err)
	}
	defer db.Close()

	rows, err := db.Query(
		"SELECT role, content, timestamp FROM messages WHERE session_id = ? ORDER BY timestamp",
		ref.SessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	var facts []importer.ImportedFact

	for rows.Next() {
		var role, content string
		var timestamp time.Time
		if err := rows.Scan(&role, &content, &timestamp); err != nil {
			continue
		}
		if role == "assistant" && len(content) > 50 {
			facts = append(facts, importer.ImportedFact{
				Content:   content,
				Source:    "hermes",
				SessionID: ref.SessionID,
				Timestamp: timestamp,
				Metadata:  map[string]string{},
			})
		}
	}

	return facts, nil
}
