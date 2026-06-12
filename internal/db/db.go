package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/danieljustus/symaira-memory/internal/config"
	_ "modernc.org/sqlite"
)

// Memory represents a single saved fact or context snippet.
type Memory struct {
	ID             string            `json:"id"`
	Content        string            `json:"content"`
	Scope          string            `json:"scope"`     // global, project, agent, user, session
	Metadata       map[string]string `json:"metadata"`  // key-value metadata
	Embedding      []float32         `json:"embedding"` // semantic embedding
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
	CreatedBy      string            `json:"created_by,omitempty"`
	UpdatedBy      string            `json:"updated_by,omitempty"`
	CreatedSession string            `json:"created_session,omitempty"`
	UpdatedSession string            `json:"updated_session,omitempty"`
	Entities       []string          `json:"entities,omitempty"` // linked entity names
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

// Open initializes the SQLite database at the standard XDG path,
// or at the path specified in the supplied configuration. The caller
// (typically cmd/) is responsible for loading configuration via
// config.Load(); library code never reads from disk directly.
func Open(cfg *config.Config) (*DB, error) {
	if cfg == nil {
		cfg = config.Defaults()
	}

	var dbPath string
	if cfg.Database.Path != "" {
		dbPath = cfg.Database.Path
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get user home dir: %w", err)
		}
		dir := filepath.Join(home, ".local", "share", "symmemory")
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, fmt.Errorf("failed to create db directory: %w", err)
		}
		dbPath = filepath.Join(dir, "default.db")
	}

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

	// Restrict database file permissions to owner-only (after migrations create the file)
	if _, err := os.Stat(dbPath); err == nil {
		if err := os.Chmod(dbPath, 0600); err != nil {
			conn.Close()
			return nil, fmt.Errorf("failed to set db file permissions: %w", err)
		}
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		sibling := dbPath + suffix
		if _, err := os.Stat(sibling); err == nil {
			_ = os.Chmod(sibling, 0600)
		}
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

	query := `INSERT INTO memories (id, content, scope, metadata, embedding, embedding_dim, lsh_hash, created_at, updated_at, created_by, updated_by, created_session, updated_session)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			content=excluded.content,
			scope=excluded.scope,
			metadata=excluded.metadata,
			embedding=excluded.embedding,
			embedding_dim=excluded.embedding_dim,
			lsh_hash=excluded.lsh_hash,
			updated_at=excluded.updated_at,
			updated_by=excluded.updated_by,
			updated_session=excluded.updated_session`

	now := time.Now().UTC()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	m.UpdatedAt = now

	_, err = db.conn.Exec(query, m.ID, m.Content, m.Scope, string(metadataJSON), string(embeddingJSON), embeddingDim, lshHash, m.CreatedAt, m.UpdatedAt, m.CreatedBy, m.UpdatedBy, m.CreatedSession, m.UpdatedSession)
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
		"SELECT id, content, scope, metadata, embedding, created_at, updated_at, created_by, updated_by, created_session, updated_session FROM memories WHERE id = ?",
		id,
	).Scan(&m.ID, &m.Content, &m.Scope, &metaStr, &embStr, &m.CreatedAt, &m.UpdatedAt, &m.CreatedBy, &m.UpdatedBy, &m.CreatedSession, &m.UpdatedSession)
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

	entities, err := db.EntitiesForMemory(m.ID)
	if err == nil && len(entities) > 0 {
		for _, e := range entities {
			m.Entities = append(m.Entities, e.Name)
		}
	}

	return &m, nil
}

// scanMemory scans a full Memory row (including embedding) from *sql.Rows.
func scanMemory(rows *sql.Rows) (*Memory, error) {
	var m Memory
	var metaStr, embStr string
	if err := rows.Scan(&m.ID, &m.Content, &m.Scope, &metaStr, &embStr, &m.CreatedAt, &m.UpdatedAt, &m.CreatedBy, &m.UpdatedBy, &m.CreatedSession, &m.UpdatedSession); err != nil {
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

// scanMemoryLite scans a Memory row without embedding data from *sql.Rows.
func scanMemoryLite(rows *sql.Rows) (*Memory, error) {
	var m Memory
	var metaStr string
	if err := rows.Scan(&m.ID, &m.Content, &m.Scope, &metaStr, &m.CreatedAt, &m.UpdatedAt, &m.CreatedBy, &m.UpdatedBy, &m.CreatedSession, &m.UpdatedSession); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(metaStr), &m.Metadata); err != nil {
		return nil, err
	}
	return &m, nil
}

// ListMemories returns memories with pagination, optionally filtered by scope.
func (db *DB) ListMemories(scope string, offset, limit int) ([]*Memory, error) {
	var query string
	var rows *sql.Rows
	var err error

	if scope != "" {
		query = "SELECT id, content, scope, metadata, embedding, created_at, updated_at, created_by, updated_by, created_session, updated_session FROM memories WHERE scope = ? ORDER BY created_at DESC LIMIT ? OFFSET ?"
		rows, err = db.conn.Query(query, scope, limit, offset)
	} else {
		query = "SELECT id, content, scope, metadata, embedding, created_at, updated_at, created_by, updated_by, created_session, updated_session FROM memories ORDER BY created_at DESC LIMIT ? OFFSET ?"
		rows, err = db.conn.Query(query, limit, offset)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []*Memory
	for rows.Next() {
		m, err := scanMemory(rows)
		if err != nil {
			return nil, err
		}
		memories = append(memories, m)
	}

	return memories, nil
}

// ListMemoriesLite returns memories without embedding data, with pagination.
func (db *DB) ListMemoriesLite(scope string, offset, limit int) ([]*Memory, error) {
	var query string
	var rows *sql.Rows
	var err error

	if scope != "" {
		query = "SELECT id, content, scope, metadata, created_at, updated_at, created_by, updated_by, created_session, updated_session FROM memories WHERE scope = ? ORDER BY created_at DESC LIMIT ? OFFSET ?"
		rows, err = db.conn.Query(query, scope, limit, offset)
	} else {
		query = "SELECT id, content, scope, metadata, created_at, updated_at, created_by, updated_by, created_session, updated_session FROM memories ORDER BY created_at DESC LIMIT ? OFFSET ?"
		rows, err = db.conn.Query(query, limit, offset)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []*Memory
	for rows.Next() {
		m, err := scanMemoryLite(rows)
		if err != nil {
			return nil, err
		}
		memories = append(memories, m)
	}

	return memories, nil
}

// ListMemoriesFiltered returns memories without embedding data, filtered by scope and optionally by entity.
func (db *DB) ListMemoriesFiltered(scope, entityID string, offset, limit int) ([]*Memory, error) {
	if entityID == "" {
		return db.ListMemoriesLite(scope, offset, limit)
	}

	memoryIDs, err := db.MemoryIDsForEntity(entityID)
	if err != nil {
		return nil, err
	}
	if len(memoryIDs) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(memoryIDs))
	args := make([]interface{}, 0, len(memoryIDs)+2)
	for i, id := range memoryIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	inClause := strings.Join(placeholders, ", ")

	var query string
	if scope != "" {
		query = "SELECT id, content, scope, metadata, created_at, updated_at, created_by, updated_by, created_session, updated_session FROM memories WHERE scope = ? AND id IN (" + inClause + ") ORDER BY created_at DESC LIMIT ? OFFSET ?"
		args = append([]interface{}{scope}, args...)
		args = append(args, limit, offset)
	} else {
		query = "SELECT id, content, scope, metadata, created_at, updated_at, created_by, updated_by, created_session, updated_session FROM memories WHERE id IN (" + inClause + ") ORDER BY created_at DESC LIMIT ? OFFSET ?"
		args = append(args, limit, offset)
	}

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []*Memory
	for rows.Next() {
		m, err := scanMemoryLite(rows)
		if err != nil {
			return nil, err
		}
		memories = append(memories, m)
	}
	return memories, nil
}

// GetMemoriesSince returns all memories with updated_at strictly after t.
// Embedding data is omitted (sync payloads do not need vectors).
func (db *DB) GetMemoriesSince(t time.Time) ([]*Memory, error) {
	rows, err := db.conn.Query(
		"SELECT id, content, scope, metadata, created_at, updated_at, created_by, updated_by, created_session, updated_session FROM memories WHERE updated_at > ? ORDER BY updated_at ASC",
		t,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []*Memory
	for rows.Next() {
		m, err := scanMemoryLite(rows)
		if err != nil {
			return nil, err
		}
		memories = append(memories, m)
	}
	return memories, nil
}

// UpsertMemoryIfNewer inserts or updates a memory only if the incoming
// updated_at is strictly newer than the stored row. Returns true when the
// row was inserted or overwritten, false when it was skipped.
func (db *DB) UpsertMemoryIfNewer(m *Memory) (bool, error) {
	metadataJSON, err := json.Marshal(m.Metadata)
	if err != nil {
		return false, fmt.Errorf("failed to marshal metadata: %w", err)
	}

	embeddingJSON, err := json.Marshal(m.Embedding)
	if err != nil {
		return false, fmt.Errorf("failed to marshal embedding: %w", err)
	}

	embeddingDim := len(m.Embedding)
	lshHash := ComputeLSH(m.Embedding)

	var existingUpdated time.Time
	err = db.conn.QueryRow("SELECT updated_at FROM memories WHERE id = ?", m.ID).Scan(&existingUpdated)
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}

	if err == sql.ErrNoRows {
		_, err = db.conn.Exec(
			`INSERT INTO memories (id, content, scope, metadata, embedding, embedding_dim, lsh_hash, created_at, updated_at, created_by, updated_by, created_session, updated_session)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			m.ID, m.Content, m.Scope, string(metadataJSON), string(embeddingJSON), embeddingDim, lshHash, m.CreatedAt, m.UpdatedAt, m.CreatedBy, m.UpdatedBy, m.CreatedSession, m.UpdatedSession,
		)
		if err != nil {
			return false, err
		}
		return true, nil
	}

	if !m.UpdatedAt.After(existingUpdated) {
		return false, nil
	}

	_, err = db.conn.Exec(
		`UPDATE memories SET content=?, scope=?, metadata=?, embedding=?, embedding_dim=?, lsh_hash=?, updated_at=?, updated_by=?, updated_session=? WHERE id=?`,
		m.Content, m.Scope, string(metadataJSON), string(embeddingJSON), embeddingDim, lshHash, m.UpdatedAt, m.UpdatedBy, m.UpdatedSession, m.ID,
	)
	if err != nil {
		return false, err
	}
	return true, nil
}

// SearchResult wraps a Memory with its similarity score without mutating the original.
type SearchResult struct {
	Memory *Memory `json:"memory"`
	Score  float32 `json:"similarity_score"`
}

// SearchMemories uses LSH bucket pre-filtering to avoid full table scans,
// then ranks the reduced candidate set by cosine similarity.
func (db *DB) SearchMemories(queryVec []float32, scope string, limit int) ([]SearchResult, error) {
	return db.SearchMemoriesFiltered(queryVec, scope, limit, "")
}

// SearchMemoriesFiltered extends SearchMemories with an optional entity filter.
// When entityID is non-empty, only memories linked to that entity are returned.
func (db *DB) SearchMemoriesFiltered(queryVec []float32, scope string, limit int, entityID string) ([]SearchResult, error) {
	const maxCandidates = 2000
	const batchSize = 64
	type scored struct {
		m     *Memory
		score float32
	}
	var results []scored

	queryLSH := ComputeLSH(queryVec)
	buckets := LSHNeighbors(queryLSH, 2)

	for i := 0; i < len(buckets) && len(results) < maxCandidates; i += batchSize {
		end := i + batchSize
		if end > len(buckets) {
			end = len(buckets)
		}
		chunk := buckets[i:end]

		placeholders := make([]string, len(chunk))
		args := make([]interface{}, 0, len(chunk)+1)
		for j, h := range chunk {
			placeholders[j] = "?"
			args = append(args, h)
		}
		inClause := strings.Join(placeholders, ", ")

		var query string
		if scope != "" {
			query = "SELECT id, content, scope, metadata, embedding, created_at, updated_at, created_by, updated_by, created_session, updated_session FROM memories WHERE scope = ? AND lsh_hash IN (" + inClause + ") ORDER BY created_at DESC"
			args = append([]interface{}{scope}, args...)
		} else {
			query = "SELECT id, content, scope, metadata, embedding, created_at, updated_at, created_by, updated_by, created_session, updated_session FROM memories WHERE lsh_hash IN (" + inClause + ") ORDER BY created_at DESC"
		}

		rows, err := db.conn.Query(query, args...)
		if err != nil {
			return nil, err
		}

		for rows.Next() {
			m, err := scanMemory(rows)
			if err != nil {
				rows.Close()
				return nil, err
			}
			if len(m.Embedding) > 0 {
				score := CosineSimilarity(queryVec, m.Embedding)
				results = append(results, scored{m: m, score: score})
			}
			if len(results) >= maxCandidates {
				break
			}
		}
		rows.Close()
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	if entityID != "" {
		allowedIDs, err := db.MemoryIDsForEntity(entityID)
		if err != nil {
			return nil, err
		}
		allowed := make(map[string]bool, len(allowedIDs))
		for _, id := range allowedIDs {
			allowed[id] = true
		}
		var filtered []scored
		for _, r := range results {
			if allowed[r.m.ID] {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}

	if limit > len(results) {
		limit = len(results)
	}

	var final []SearchResult
	for i := 0; i < limit; i++ {
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

// RevokeToken persists a JWT ID to the revocation table so it cannot be used
// across process restarts.
func (db *DB) RevokeToken(jti string) error {
	_, err := db.conn.Exec(
		"INSERT OR IGNORE INTO jwt_revocations (jti) VALUES (?)",
		jti,
	)
	return err
}

// IsTokenRevoked checks whether a JWT ID has been persisted as revoked.
func (db *DB) IsTokenRevoked(jti string) (bool, error) {
	var count int
	err := db.conn.QueryRow(
		"SELECT COUNT(*) FROM jwt_revocations WHERE jti = ?",
		jti,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetSyncCursor returns the last sync timestamp for a given remote URL.
// Returns zero time if no cursor has been stored yet.
func (db *DB) GetSyncCursor(remote string) (time.Time, error) {
	var t time.Time
	err := db.conn.QueryRow(
		"SELECT last_sync FROM sync_state WHERE remote = ?", remote,
	).Scan(&t)
	if err == sql.ErrNoRows {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	return t, nil
}

// SetSyncCursor upserts the last sync timestamp for a given remote URL.
func (db *DB) SetSyncCursor(remote string, t time.Time) error {
	_, err := db.conn.Exec(
		`INSERT INTO sync_state (remote, last_sync) VALUES (?, ?)
		 ON CONFLICT(remote) DO UPDATE SET last_sync = excluded.last_sync`,
		remote, t,
	)
	return err
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
	UpdatedAt time.Time         `json:"updated_at"`
	CreatedBy string            `json:"created_by,omitempty"`
	UpdatedBy string            `json:"updated_by,omitempty"`
}

// SaveRule inserts or updates a procedural memory rule.
func (db *DB) SaveRule(r *Rule) error {
	metadataJSON, err := json.Marshal(r.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `INSERT INTO rules (id, content, scope, metadata, created_at, updated_at, created_by, updated_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			content=excluded.content,
			scope=excluded.scope,
			metadata=excluded.metadata,
			updated_at=excluded.updated_at,
			updated_by=excluded.updated_by`

	now := time.Now().UTC()
	if r.CreatedAt.IsZero() {
		r.CreatedAt = now
	}
	if r.UpdatedAt.IsZero() {
		r.UpdatedAt = now
	}

	_, err = db.conn.Exec(query, r.ID, r.Content, r.Scope, string(metadataJSON), r.CreatedAt, r.UpdatedAt, r.CreatedBy, r.UpdatedBy)
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
		query = "SELECT id, content, scope, metadata, created_at, updated_at, created_by, updated_by FROM rules WHERE scope = ? ORDER BY created_at DESC"
		rows, err = db.conn.Query(query, scope)
	} else {
		query = "SELECT id, content, scope, metadata, created_at, updated_at, created_by, updated_by FROM rules ORDER BY created_at DESC"
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
		if err := rows.Scan(&r.ID, &r.Content, &r.Scope, &metaStr, &r.CreatedAt, &r.UpdatedAt, &r.CreatedBy, &r.UpdatedBy); err != nil {
			return nil, err
		}

		if err := json.Unmarshal([]byte(metaStr), &r.Metadata); err != nil {
			return nil, err
		}

		rules = append(rules, &r)
	}

	return rules, nil
}
