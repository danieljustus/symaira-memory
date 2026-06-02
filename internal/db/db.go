package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Memory represents a single saved fact or context snippet.
type Memory struct {
	ID        string            `json:"id"`
	Content   string            `json:"content"`
	Scope     string            `json:"scope"`     // global, project, agent, user, session
	Metadata  map[string]string `json:"metadata"`  // key-value metadata
	Embedding []float32         `json:"embedding"` // semantic embedding
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// Session represents a compressed summary of a chat session.
type Session struct {
	ID        string    `json:"id"`
	Summary   string    `json:"summary"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DB wraps the SQL connection.
type DB struct {
	conn *sql.DB
}

// Open initializes the SQLite database at the standard XDG path.
func Open() (*DB, error) {
	// Find XDG compliant path
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home dir: %w", err)
	}

	dir := filepath.Join(home, ".local", "share", "symmemory")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	dbPath := filepath.Join(dir, "default.db")
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// Enable WAL mode for concurrent reads/writes
	if _, err := conn.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to enable WAL: %w", err)
	}

	db := &DB{conn: conn}
	if err := db.initSchema(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return db, nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// initSchema creates the tables if they do not exist.
func (db *DB) initSchema() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS memories (
			id TEXT PRIMARY KEY,
			content TEXT NOT NULL,
			scope TEXT NOT NULL,
			metadata TEXT NOT NULL,
			embedding TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			summary TEXT NOT NULL,
			updated_at DATETIME NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_memories_scope ON memories(scope);`,
	}

	for _, q := range queries {
		if _, err := db.conn.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// SaveMemory inserts or updates a memory.
func (db *DB) SaveMemory(m *Memory) error {
	metadataJSON, err := json.Marshal(m.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	embeddingJSON, err := json.Marshal(m.Embedding)
	if err != nil {
		return fmt.Errorf("failed to marshal embedding: %w", err)
	}

	query := `INSERT INTO memories (id, content, scope, metadata, embedding, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			content=excluded.content,
			scope=excluded.scope,
			metadata=excluded.metadata,
			embedding=excluded.embedding,
			updated_at=excluded.updated_at`

	now := time.Now()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	m.UpdatedAt = now

	_, err = db.conn.Exec(query, m.ID, m.Content, m.Scope, string(metadataJSON), string(embeddingJSON), m.CreatedAt, m.UpdatedAt)
	return err
}

// DeleteMemory removes a memory by ID.
func (db *DB) DeleteMemory(id string) error {
	_, err := db.conn.Exec("DELETE FROM memories WHERE id = ?", id)
	return err
}

// ListMemories returns all memories, optionally filtered by scope.
func (db *DB) ListMemories(scope string) ([]*Memory, error) {
	var query string
	var rows *sql.Rows
	var err error

	if scope != "" {
		query = "SELECT id, content, scope, metadata, embedding, created_at, updated_at FROM memories WHERE scope = ? ORDER BY created_at DESC"
		rows, err = db.conn.Query(query, scope)
	} else {
		query = "SELECT id, content, scope, metadata, embedding, created_at, updated_at FROM memories ORDER BY created_at DESC"
		rows, err = db.conn.Query(query)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []*Memory
	for rows.Next() {
		var m Memory
		var metaStr, embStr string
		if err := rows.Scan(&m.ID, &m.Content, &m.Scope, &metaStr, &embStr, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}

		if err := json.Unmarshal([]byte(metaStr), &m.Metadata); err != nil {
			return nil, err
		}

		if err := json.Unmarshal([]byte(embStr), &m.Embedding); err != nil {
			return nil, err
		}

		memories = append(memories, &m)
	}

	return memories, nil
}

// SearchMemories queries memories in Go by calculating cosine similarity over stored vectors.
func (db *DB) SearchMemories(queryVec []float32, scope string, limit int) ([]*Memory, error) {
	memories, err := db.ListMemories(scope)
	if err != nil {
		return nil, err
	}

	type searchResult struct {
		m     *Memory
		score float32
	}

	var results []searchResult
	for _, m := range memories {
		if len(m.Embedding) == 0 {
			continue
		}
		score := CosineSimilarity(queryVec, m.Embedding)
		results = append(results, searchResult{m: m, score: score})
	}

	// Sort by score descending (simple bubble/insertion sort or slice sorting)
	// Since slice sorting is easiest:
	// We sort the results manually:
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].score > results[i].score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	// Apply limit
	if limit > len(results) {
		limit = len(results)
	}

	var final []*Memory
	for i := 0; i < limit; i++ {
		// Embed the similarity score in the metadata for transparency
		if results[i].m.Metadata == nil {
			results[i].m.Metadata = make(map[string]string)
		}
		results[i].m.Metadata["similarity_score"] = fmt.Sprintf("%.4f", results[i].score)
		final = append(final, results[i].m)
	}

	return final, nil
}

// CosineSimilarity calculates the cosine similarity between two float vectors.
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dotProduct, normA, normB float64
	for i := 0; i < len(a); i++ {
		dotProduct += float64(a[i] * b[i])
		normA += float64(a[i] * a[i])
		normB += float64(b[i] * b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return float32(dotProduct / (math.Sqrt(normA) * math.Sqrt(normB)))
}

// SaveSessionSummary inserts or updates a session summary.
func (db *DB) SaveSessionSummary(id, summary string) error {
	query := `INSERT INTO sessions (id, summary, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			summary=excluded.summary,
			updated_at=excluded.updated_at`

	_, err := db.conn.Exec(query, id, summary, time.Now())
	return err
}

// GetSessionSummary retrieves a session summary by ID.
func (db *DB) GetSessionSummary(id string) (string, error) {
	var summary string
	err := db.conn.QueryRow("SELECT summary FROM sessions WHERE id = ?", id).Scan(&summary)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return summary, err
}
