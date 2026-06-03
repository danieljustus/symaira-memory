package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
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
	if err := db.runMigrations(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return db, nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.conn.Close()
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

	embeddingDim := len(m.Embedding)
	lshHash := ComputeLSH(m.Embedding)

	query := `INSERT INTO memories (id, content, scope, metadata, embedding, embedding_dim, lsh_hash, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			content=excluded.content,
			scope=excluded.scope,
			metadata=excluded.metadata,
			embedding=excluded.embedding,
			embedding_dim=excluded.embedding_dim,
			lsh_hash=excluded.lsh_hash,
			updated_at=excluded.updated_at`

	now := time.Now()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	m.UpdatedAt = now

	_, err = db.conn.Exec(query, m.ID, m.Content, m.Scope, string(metadataJSON), string(embeddingJSON), embeddingDim, lshHash, m.CreatedAt, m.UpdatedAt)
	return err
}

// DeleteMemory removes a memory by ID.
func (db *DB) DeleteMemory(id string) error {
	_, err := db.conn.Exec("DELETE FROM memories WHERE id = ?", id)
	return err
}

// GetMemory retrieves a single memory by its ID using a direct index lookup.
func (db *DB) GetMemory(id string) (*Memory, error) {
	var m Memory
	var metaStr, embStr string
	err := db.conn.QueryRow(
		"SELECT id, content, scope, metadata, embedding, created_at, updated_at FROM memories WHERE id = ?",
		id,
	).Scan(&m.ID, &m.Content, &m.Scope, &metaStr, &embStr, &m.CreatedAt, &m.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(metaStr), &m.Metadata); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(embStr), &m.Embedding); err != nil {
		return nil, err
	}
	return &m, nil
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

// SearchResult wraps a Memory with its similarity score without mutating the original.
type SearchResult struct {
	Memory *Memory `json:"memory"`
	Score  float32 `json:"similarity_score"`
}

// SearchMemories uses LSH bucket pre-filtering to avoid full table scans,
// then ranks the reduced candidate set by cosine similarity.
func (db *DB) SearchMemories(queryVec []float32, scope string, limit int) ([]SearchResult, error) {
	const chunkSize = 200
	type scored struct {
		m     *Memory
		score float32
	}
	var results []scored

	queryLSH := ComputeLSH(queryVec)

	offset := 0
	for {
		var rows *sql.Rows
		var err error
		if scope != "" {
			rows, err = db.conn.Query(
				"SELECT id, content, scope, metadata, embedding, created_at, updated_at FROM memories WHERE scope = ? AND lsh_hash = ? ORDER BY created_at DESC LIMIT ? OFFSET ?",
				scope, queryLSH, chunkSize, offset,
			)
		} else {
			rows, err = db.conn.Query(
				"SELECT id, content, scope, metadata, embedding, created_at, updated_at FROM memories WHERE lsh_hash = ? ORDER BY created_at DESC LIMIT ? OFFSET ?",
				queryLSH, chunkSize, offset,
			)
		}
		if err != nil {
			return nil, err
		}

		rowCount := 0
		for rows.Next() {
			var m Memory
			var metaStr, embStr string
			if err := rows.Scan(&m.ID, &m.Content, &m.Scope, &metaStr, &embStr, &m.CreatedAt, &m.UpdatedAt); err != nil {
				rows.Close()
				return nil, err
			}
			if err := json.Unmarshal([]byte(metaStr), &m.Metadata); err != nil {
				rows.Close()
				return nil, err
			}
			if err := json.Unmarshal([]byte(embStr), &m.Embedding); err != nil {
				rows.Close()
				return nil, err
			}
			if len(m.Embedding) > 0 {
				score := CosineSimilarity(queryVec, m.Embedding)
				results = append(results, scored{m: &m, score: score})
			}
			rowCount++
		}
		rows.Close()

		if rowCount < chunkSize {
			break
		}
		offset += chunkSize
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	if limit > len(results) {
		limit = len(results)
	}

	var final []SearchResult
	for i := 0; i < limit; i++ {
		metaCopy := make(map[string]string, len(results[i].m.Metadata)+1)
		for k, v := range results[i].m.Metadata {
			metaCopy[k] = v
		}
		metaCopy["similarity_score"] = fmt.Sprintf("%.4f", results[i].score)
		final = append(final, SearchResult{
			Memory: results[i].m,
			Score:  results[i].score,
		})
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

// Rule represents a stored procedural behavioral instruction.
type Rule struct {
	ID        string            `json:"id"`
	Content   string            `json:"content"`
	Scope     string            `json:"scope"`
	Metadata  map[string]string `json:"metadata"`
	CreatedAt time.Time         `json:"created_at"`
}

// SaveRule inserts or updates a procedural memory rule.
func (db *DB) SaveRule(r *Rule) error {
	metadataJSON, err := json.Marshal(r.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `INSERT INTO rules (id, content, scope, metadata, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			content=excluded.content,
			scope=excluded.scope,
			metadata=excluded.metadata`

	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now()
	}

	_, err = db.conn.Exec(query, r.ID, r.Content, r.Scope, string(metadataJSON), r.CreatedAt)
	return err
}

// DeleteRule removes a procedural rule by ID.
func (db *DB) DeleteRule(id string) error {
	_, err := db.conn.Exec("DELETE FROM rules WHERE id = ?", id)
	return err
}

// ListRules retrieves stored rules, optionally filtered by scope.
func (db *DB) ListRules(scope string) ([]*Rule, error) {
	var query string
	var rows *sql.Rows
	var err error

	if scope != "" {
		query = "SELECT id, content, scope, metadata, created_at FROM rules WHERE scope = ? ORDER BY created_at DESC"
		rows, err = db.conn.Query(query, scope)
	} else {
		query = "SELECT id, content, scope, metadata, created_at FROM rules ORDER BY created_at DESC"
		rows, err = db.conn.Query(query)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []*Rule
	for rows.Next() {
		var r Rule
		var metaStr string
		if err := rows.Scan(&r.ID, &r.Content, &r.Scope, &metaStr, &r.CreatedAt); err != nil {
			return nil, err
		}

		if err := json.Unmarshal([]byte(metaStr), &r.Metadata); err != nil {
			return nil, err
		}

		rules = append(rules, &r)
	}

	return rules, nil
}
