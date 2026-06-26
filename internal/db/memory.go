package db

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// Memory represents a single saved fact or context snippet.
type Memory struct {
	ID                  string            `json:"id"`
	Content             string            `json:"content"`
	Scope               string            `json:"scope"`     // global, project, agent, user, session
	Metadata            map[string]string `json:"metadata"`  // key-value metadata
	Embedding           []float32         `json:"embedding"` // semantic embedding
	EmbeddingSource     string            `json:"embedding_source,omitempty"`
	EmbeddingModel      string            `json:"embedding_model,omitempty"`
	ContentHash         string            `json:"content_hash,omitempty"`
	CreatedAt           time.Time         `json:"created_at"`
	UpdatedAt           time.Time         `json:"updated_at"`
	CreatedBy           string            `json:"created_by,omitempty"`
	UpdatedBy           string            `json:"updated_by,omitempty"`
	CreatedSession      string            `json:"created_session,omitempty"`
	UpdatedSession      string            `json:"updated_session,omitempty"`
	Entities            []string          `json:"entities,omitempty"` // linked entity names
	ConsolidationStatus string            `json:"consolidation_status"`
	ConsolidatedIntoID  string            `json:"consolidated_into_id,omitempty"`
	Importance          float64           `json:"importance"` // 0.0–1.0, default 0.5
	ValidFrom           *time.Time        `json:"valid_from,omitempty"`
	ValidTo             *time.Time        `json:"valid_to,omitempty"`
	SupersededBy        string            `json:"superseded_by,omitempty"`
}

// SearchResult wraps a Memory with its similarity score without mutating the original.
type SearchResult struct {
	Memory *Memory `json:"memory"`
	Score  float32 `json:"similarity_score"`
}

// RankingWeights holds configurable weights for composite retrieval scoring.
type RankingWeights struct {
	RelevanceWeight  float64
	RecencyWeight    float64
	ImportanceWeight float64
	RecencyHalfLife  float64 // days
}

// TrustFilter defines retrieval filters for trust-aware memory search.
// Empty fields are ignored (no filtering on that dimension).
type TrustFilter struct {
	MinConfidence      string // "low", "medium", "high" — skip memories below this
	VerificationStatus string // "verified", "unverified", "stale" — filter by verification
	ExcludeSuperseded  bool   // when true, skip memories with non-empty superseded_by
	MaxAge             time.Duration // when non-zero, skip memories older than this
}

// PolicyFilter defines sensitivity and sharing policy filters for memory retrieval.
// Empty fields are ignored (no filtering on that dimension).
type PolicyFilter struct {
	MaxSensitivity  string // "public", "internal", "confidential", "secret" — skip memories above this
	MinSharingLevel string // "private", "team", "org", "public" — skip memories below this
	ClientID        string // when non-empty, check against allowed_clients metadata
}

// ComputeContentHash returns the SHA-256 hex digest of the given content string.
func ComputeContentHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", h)
}

// saveMemoryExec is the shared implementation for SaveMemory and SaveMemoryTx.
func saveMemoryExec(execer SQLExecer, m *Memory) error {
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

	// Compute content hash if not already set.
	contentHash := m.ContentHash
	if contentHash == "" {
		contentHash = ComputeContentHash(m.Content)
	}

	status := m.ConsolidationStatus
	if status == "" {
		status = "raw"
	}
	var consolidatedInto sql.NullString
	if m.ConsolidatedIntoID != "" {
		consolidatedInto.String = m.ConsolidatedIntoID
		consolidatedInto.Valid = true
	}
	var validFrom, validTo sql.NullTime
	if m.ValidFrom != nil {
		validFrom.Time = *m.ValidFrom
		validFrom.Valid = true
	} else {
		now := time.Now().UTC()
		validFrom.Time = now
		validFrom.Valid = true
	}
	if m.ValidTo != nil {
		validTo.Time = *m.ValidTo
		validTo.Valid = true
	}
	var supersededBy sql.NullString
	if m.SupersededBy != "" {
		supersededBy.String = m.SupersededBy
		supersededBy.Valid = true
	}

	query := `INSERT INTO memories (id, content, scope, metadata, embedding, embedding_dim, embedding_source, embedding_model, content_hash, lsh_hash, created_at, updated_at, created_by, updated_by, created_session, updated_session, consolidation_status, consolidated_into_id, importance, valid_from, valid_to, superseded_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			content=excluded.content,
			scope=excluded.scope,
			metadata=excluded.metadata,
			embedding=excluded.embedding,
			embedding_dim=excluded.embedding_dim,
			embedding_source=excluded.embedding_source,
			embedding_model=excluded.embedding_model,
			content_hash=excluded.content_hash,
			lsh_hash=excluded.lsh_hash,
			updated_at=excluded.updated_at,
			updated_by=excluded.updated_by,
			updated_session=excluded.updated_session,
			consolidation_status=excluded.consolidation_status,
			consolidated_into_id=excluded.consolidated_into_id,
			importance=excluded.importance,
			valid_from=excluded.valid_from,
			valid_to=excluded.valid_to,
			superseded_by=excluded.superseded_by`

	now := time.Now().UTC()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	m.UpdatedAt = now

	_, err = execer.Exec(query, m.ID, m.Content, m.Scope, string(metadataJSON), string(embeddingJSON), embeddingDim, m.EmbeddingSource, m.EmbeddingModel, contentHash, lshHash, m.CreatedAt, m.UpdatedAt, m.CreatedBy, m.UpdatedBy, m.CreatedSession, m.UpdatedSession, status, consolidatedInto, m.Importance, validFrom, validTo, supersededBy)
	return err
}

// SaveMemory inserts or updates a memory.
func (db *DB) SaveMemory(m *Memory) error {
	return saveMemoryExec(db.conn, m)
}

// SaveMemoryTx inserts or updates a memory within a transaction.
func (db *DB) SaveMemoryTx(tx *sql.Tx, m *Memory) error {
	return saveMemoryExec(tx, m)
}

// UpdateMemoryStatusTx updates the consolidation status and parent ID of a memory within a transaction.
func (db *DB) UpdateMemoryStatusTx(tx *sql.Tx, id string, status string, parentID string) error {
	var consolidatedInto sql.NullString
	if parentID != "" {
		consolidatedInto.String = parentID
		consolidatedInto.Valid = true
	}
	query := `UPDATE memories SET consolidation_status = ?, consolidated_into_id = ?, updated_at = ? WHERE id = ?`
	_, err := tx.Exec(query, status, consolidatedInto, time.Now().UTC(), id)
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
	var consolidatedInto sql.NullString
	var validFrom, validTo sql.NullTime
	var supersededBy sql.NullString
	err := db.conn.QueryRow(
		"SELECT id, content, scope, metadata, embedding, embedding_source, embedding_model, created_at, updated_at, created_by, updated_by, created_session, updated_session, consolidation_status, consolidated_into_id, importance, valid_from, valid_to, superseded_by FROM memories WHERE id = ?",
		id,
	).Scan(&m.ID, &m.Content, &m.Scope, &metaStr, &embStr, &m.EmbeddingSource, &m.EmbeddingModel, &m.CreatedAt, &m.UpdatedAt, &m.CreatedBy, &m.UpdatedBy, &m.CreatedSession, &m.UpdatedSession, &m.ConsolidationStatus, &consolidatedInto, &m.Importance, &validFrom, &validTo, &supersededBy)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := populateMemoryFields(&m, metaStr, consolidatedInto, validFrom, validTo, supersededBy); err != nil {
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

func populateMemoryFields(m *Memory, metaStr string, consolidatedInto sql.NullString, validFrom, validTo sql.NullTime, supersededBy sql.NullString) error {
	if err := json.Unmarshal([]byte(metaStr), &m.Metadata); err != nil {
		return err
	}
	m.ConsolidatedIntoID = consolidatedInto.String
	if validFrom.Valid {
		m.ValidFrom = &validFrom.Time
	}
	if validTo.Valid {
		m.ValidTo = &validTo.Time
	}
	m.SupersededBy = supersededBy.String
	return nil
}

// scanMemory scans a full Memory row (including embedding) from *sql.Rows.
func scanMemory(rows *sql.Rows) (*Memory, error) {
	var m Memory
	var metaStr, embStr string
	var consolidatedInto sql.NullString
	var validFrom, validTo sql.NullTime
	var supersededBy sql.NullString
	if err := rows.Scan(&m.ID, &m.Content, &m.Scope, &metaStr, &embStr, &m.EmbeddingSource, &m.EmbeddingModel, &m.CreatedAt, &m.UpdatedAt, &m.CreatedBy, &m.UpdatedBy, &m.CreatedSession, &m.UpdatedSession, &m.ConsolidationStatus, &consolidatedInto, &m.Importance, &validFrom, &validTo, &supersededBy); err != nil {
		return nil, err
	}
	if err := populateMemoryFields(&m, metaStr, consolidatedInto, validFrom, validTo, supersededBy); err != nil {
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
	var consolidatedInto sql.NullString
	var validFrom, validTo sql.NullTime
	var supersededBy sql.NullString
	if err := rows.Scan(&m.ID, &m.Content, &m.Scope, &metaStr, &m.CreatedAt, &m.UpdatedAt, &m.CreatedBy, &m.UpdatedBy, &m.CreatedSession, &m.UpdatedSession, &m.ConsolidationStatus, &consolidatedInto, &m.Importance, &validFrom, &validTo, &supersededBy); err != nil {
		return nil, err
	}
	if err := populateMemoryFields(&m, metaStr, consolidatedInto, validFrom, validTo, supersededBy); err != nil {
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
		query = "SELECT id, content, scope, metadata, embedding, embedding_source, embedding_model, created_at, updated_at, created_by, updated_by, created_session, updated_session, consolidation_status, consolidated_into_id, importance, valid_from, valid_to, superseded_by FROM memories WHERE scope = ? AND consolidation_status != 'archived' ORDER BY created_at DESC LIMIT ? OFFSET ?"
		rows, err = db.conn.Query(query, scope, limit, offset)
	} else {
		query = "SELECT id, content, scope, metadata, embedding, embedding_source, embedding_model, created_at, updated_at, created_by, updated_by, created_session, updated_session, consolidation_status, consolidated_into_id, importance, valid_from, valid_to, superseded_by FROM memories WHERE consolidation_status != 'archived' ORDER BY created_at DESC LIMIT ? OFFSET ?"
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
		query = "SELECT id, content, scope, metadata, created_at, updated_at, created_by, updated_by, created_session, updated_session, consolidation_status, consolidated_into_id, importance, valid_from, valid_to, superseded_by FROM memories WHERE scope = ? AND consolidation_status != 'archived' ORDER BY created_at DESC LIMIT ? OFFSET ?"
		rows, err = db.conn.Query(query, scope, limit, offset)
	} else {
		query = "SELECT id, content, scope, metadata, created_at, updated_at, created_by, updated_by, created_session, updated_session, consolidation_status, consolidated_into_id, importance, valid_from, valid_to, superseded_by FROM memories WHERE consolidation_status != 'archived' ORDER BY created_at DESC LIMIT ? OFFSET ?"
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
		query = "SELECT id, content, scope, metadata, created_at, updated_at, created_by, updated_by, created_session, updated_session, consolidation_status, consolidated_into_id, importance, valid_from, valid_to, superseded_by FROM memories WHERE scope = ? AND consolidation_status != 'archived' AND id IN (" + inClause + ") ORDER BY created_at DESC LIMIT ? OFFSET ?"
		args = append([]interface{}{scope}, args...)
		args = append(args, limit, offset)
	} else {
		query = "SELECT id, content, scope, metadata, created_at, updated_at, created_by, updated_by, created_session, updated_session, consolidation_status, consolidated_into_id, importance, valid_from, valid_to, superseded_by FROM memories WHERE consolidation_status != 'archived' AND id IN (" + inClause + ") ORDER BY created_at DESC LIMIT ? OFFSET ?"
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
	return db.GetMemoriesSinceCursor(t, 0)
}

// GetMemoriesSinceCursor returns memories updated after since, with cursor-based pagination.
// When includeEmbeddings is true, the full embedding vector is loaded (needed for sync transfer).
func (db *DB) GetMemoriesSinceCursor(since time.Time, limit int, includeEmbeddings ...bool) ([]*Memory, error) {
	if limit <= 0 {
		limit = 50000
	}
	includeEmb := len(includeEmbeddings) > 0 && includeEmbeddings[0]

	var query string
	if includeEmb {
		query = "SELECT id, content, scope, metadata, embedding, embedding_source, embedding_model, created_at, updated_at, created_by, updated_by, created_session, updated_session, consolidation_status, consolidated_into_id, importance, valid_from, valid_to, superseded_by FROM memories WHERE updated_at > ? ORDER BY updated_at ASC LIMIT ?"
	} else {
		query = "SELECT id, content, scope, metadata, created_at, updated_at, created_by, updated_by, created_session, updated_session, consolidation_status, consolidated_into_id, importance, valid_from, valid_to, superseded_by FROM memories WHERE updated_at > ? ORDER BY updated_at ASC LIMIT ?"
	}

	rows, err := db.conn.Query(query, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var memories []*Memory
	for rows.Next() {
		var m *Memory
		if includeEmb {
			m, err = scanMemory(rows)
		} else {
			m, err = scanMemoryLite(rows)
		}
		if err != nil {
			return nil, err
		}
		memories = append(memories, m)
	}
	return memories, nil
}

// GetRawMemories returns all memories with consolidation_status = 'raw'.
func (db *DB) GetRawMemories() ([]*Memory, error) {
	query := "SELECT id, content, scope, metadata, embedding, embedding_source, embedding_model, created_at, updated_at, created_by, updated_by, created_session, updated_session, consolidation_status, consolidated_into_id, importance, valid_from, valid_to, superseded_by FROM memories WHERE consolidation_status = 'raw' ORDER BY created_at ASC"
	rows, err := db.conn.Query(query)
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

	contentHash := m.ContentHash
	if contentHash == "" {
		contentHash = ComputeContentHash(m.Content)
	}

	status := m.ConsolidationStatus
	if status == "" {
		status = "raw"
	}
	var consolidatedInto sql.NullString
	if m.ConsolidatedIntoID != "" {
		consolidatedInto.String = m.ConsolidatedIntoID
		consolidatedInto.Valid = true
	}

	var existingUpdated time.Time
	err = db.conn.QueryRow("SELECT updated_at FROM memories WHERE id = ?", m.ID).Scan(&existingUpdated)
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}

	if err == sql.ErrNoRows {
		_, err = db.conn.Exec(
			`INSERT INTO memories (id, content, scope, metadata, embedding, embedding_dim, embedding_source, embedding_model, content_hash, lsh_hash, created_at, updated_at, created_by, updated_by, created_session, updated_session, consolidation_status, consolidated_into_id, importance, valid_from, valid_to, superseded_by)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			m.ID, m.Content, m.Scope, string(metadataJSON), string(embeddingJSON), embeddingDim, m.EmbeddingSource, m.EmbeddingModel, contentHash, lshHash, m.CreatedAt, m.UpdatedAt, m.CreatedBy, m.UpdatedBy, m.CreatedSession, m.UpdatedSession, status, consolidatedInto, m.Importance, m.ValidFrom, m.ValidTo, nullStr(m.SupersededBy),
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
		`UPDATE memories SET content=?, scope=?, metadata=?, embedding=?, embedding_dim=?, embedding_source=?, embedding_model=?, content_hash=?, lsh_hash=?, updated_at=?, updated_by=?, updated_session=?, consolidation_status=?, consolidated_into_id=?, importance=? WHERE id=?`,
		m.Content, m.Scope, string(metadataJSON), string(embeddingJSON), embeddingDim, m.EmbeddingSource, m.EmbeddingModel, contentHash, lshHash, m.UpdatedAt, m.UpdatedBy, m.UpdatedSession, status, consolidatedInto, m.Importance, m.ID,
	)
	if err != nil {
		return false, err
	}
	return true, nil
}

// DefaultRankingWeights returns the default ranking configuration.
func DefaultRankingWeights() RankingWeights {
	return RankingWeights{
		RelevanceWeight:  0.6,
		RecencyWeight:    0.2,
		ImportanceWeight: 0.2,
		RecencyHalfLife:  30,
	}
}

// SearchMemories uses LSH bucket pre-filtering to avoid full table scans,
// then ranks the reduced candidate set by cosine similarity.
func (db *DB) SearchMemories(queryVec []float32, querySource string, scope string, limit int) ([]SearchResult, error) {
	return db.SearchMemoriesFiltered(queryVec, querySource, scope, limit, "", DefaultRankingWeights())
}

// SearchMemoriesFiltered extends SearchMemories with an optional entity filter.
// When entityID is non-empty, only memories linked to that entity are returned.
// Only memories whose embedding_source matches querySource are scored; rows
// from a different embedding space are silently skipped.
func (db *DB) SearchMemoriesFiltered(queryVec []float32, querySource string, scope string, limit int, entityID string, weights ...RankingWeights) ([]SearchResult, error) {
	return db.SearchMemoriesFilteredWithTrust(queryVec, querySource, scope, limit, entityID, TrustFilter{}, PolicyFilter{}, weights...)
}

// SearchMemoriesFilteredWithTrust extends SearchMemoriesFiltered with trust-aware
// and policy-aware filtering. Memories that don't match the trust or policy filter
// are excluded from results.
func (db *DB) SearchMemoriesFilteredWithTrust(queryVec []float32, querySource string, scope string, limit int, entityID string, trustFilter TrustFilter, policyFilter PolicyFilter, weights ...RankingWeights) ([]SearchResult, error) {
	w := DefaultRankingWeights()
	if len(weights) > 0 {
		w = weights[0]
	}
	const maxCandidates = 2000
	const batchSize = 64
	type scored struct {
		m     *Memory
		score float32
	}
	var results []scored

	queryLSH := ComputeLSH(queryVec)
	buckets := LSHNeighbors(queryLSH, 2)

	var candidateIDs []string

	for i := 0; i < len(buckets) && len(candidateIDs) < maxCandidates; i += batchSize {
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
			query = "SELECT id FROM memories WHERE scope = ? AND consolidation_status != 'archived' AND lsh_hash IN (" + inClause + ")"
			args = append([]interface{}{scope}, args...)
		} else {
			query = "SELECT id FROM memories WHERE consolidation_status != 'archived' AND lsh_hash IN (" + inClause + ")"
		}
		if entityID != "" {
			query += " AND id IN (SELECT memory_id FROM memory_entities WHERE entity_id = ?)"
			args = append(args, entityID)
		}
		query += " ORDER BY created_at DESC"

		rows, err := db.conn.Query(query, args...)
		if err != nil {
			return nil, err
		}

		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			candidateIDs = append(candidateIDs, id)
			if len(candidateIDs) >= maxCandidates {
				break
			}
		}
		rows.Close()
	}

	if len(candidateIDs) == 0 {
		return nil, nil
	}

	for i := 0; i < len(candidateIDs); i += batchSize {
		end := i + batchSize
		if end > len(candidateIDs) {
			end = len(candidateIDs)
		}
		chunk := candidateIDs[i:end]

		placeholders := make([]string, len(chunk))
		args := make([]interface{}, 0, len(chunk)+1)
		for j, id := range chunk {
			placeholders[j] = "?"
			args = append(args, id)
		}
		inClause := strings.Join(placeholders, ", ")

		query := "SELECT id, content, scope, metadata, embedding, embedding_source, embedding_model, created_at, updated_at, created_by, updated_by, created_session, updated_session, consolidation_status, consolidated_into_id, importance, valid_from, valid_to, superseded_by FROM memories WHERE id IN (" + inClause + ")"

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
			if len(m.Embedding) > 0 && m.EmbeddingSource == querySource {
				if !passesTrustFilter(m, trustFilter) {
					continue
				}
				if !passesPolicyFilter(m, policyFilter) {
					continue
				}
				relevance := CosineSimilarity(queryVec, m.Embedding)
				score := float32(CompositeScore(relevance, m.CreatedAt, float64(m.Importance)/10.0, w))
				results = append(results, scored{m: m, score: score})
			}
		}
		rows.Close()
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

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

func passesTrustFilter(m *Memory, f TrustFilter) bool {
	if f.ExcludeSuperseded && m.SupersededBy != "" {
		return false
	}

	meta := m.Metadata
	if meta == nil {
		meta = make(map[string]string)
	}

	if f.MinConfidence != "" {
		confidence := meta["confidence"]
		if confidence == "" {
			confidence = "medium"
		}
		if !confidenceMeetsMinimum(confidence, f.MinConfidence) {
			return false
		}
	}

	if f.VerificationStatus != "" {
		status := meta["verification_status"]
		if status == "" {
			status = "unverified"
		}
		if status != f.VerificationStatus {
			return false
		}
	}

	if f.MaxAge > 0 {
		if time.Since(m.CreatedAt) > f.MaxAge {
			return false
		}
	}

	return true
}

func confidenceMeetsMinimum(actual, minimum string) bool {
	rank := map[string]int{
		"low":    0,
		"medium": 1,
		"high":   2,
	}
	actualRank, ok := rank[actual]
	if !ok {
		actualRank = 1
	}
	minRank, ok := rank[minimum]
	if !ok {
		minRank = 1
	}
	return actualRank >= minRank
}

func passesPolicyFilter(m *Memory, f PolicyFilter) bool {
	if f.MaxSensitivity == "" && f.MinSharingLevel == "" && f.ClientID == "" {
		return true
	}

	meta := m.Metadata
	if meta == nil {
		meta = make(map[string]string)
	}

	if f.MaxSensitivity != "" {
		sensitivity := meta["sensitivity"]
		if sensitivity == "" {
			sensitivity = "internal"
		}
		if !sensitivityWithinLimit(sensitivity, f.MaxSensitivity) {
			return false
		}
	}

	if f.MinSharingLevel != "" {
		sharing := meta["sharing_level"]
		if sharing == "" {
			sharing = "private"
		}
		if !sharingMeetsMinimum(sharing, f.MinSharingLevel) {
			return false
		}
	}

	if f.ClientID != "" {
		allowed := meta["allowed_clients"]
		denied := meta["denied_clients"]
		if denied != "" {
			for _, c := range splitCSV(denied) {
				if c == f.ClientID {
					return false
				}
			}
		}
		if allowed != "" {
			found := false
			for _, c := range splitCSV(allowed) {
				if c == f.ClientID {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}

	return true
}

func sensitivityWithinLimit(actual, max string) bool {
	rank := map[string]int{
		"public":       0,
		"internal":     1,
		"confidential": 2,
		"secret":       3,
	}
	actualRank, ok := rank[actual]
	if !ok {
		actualRank = 1
	}
	maxRank, ok := rank[max]
	if !ok {
		maxRank = 1
	}
	return actualRank <= maxRank
}

func sharingMeetsMinimum(actual, minimum string) bool {
	rank := map[string]int{
		"private": 0,
		"team":    1,
		"org":     2,
		"public":  3,
	}
	actualRank, ok := rank[actual]
	if !ok {
		actualRank = 0
	}
	minRank, ok := rank[minimum]
	if !ok {
		minRank = 0
	}
	return actualRank >= minRank
}

func splitCSV(s string) []string {
	var result []string
	for _, part := range strings.Split(s, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
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

// CompositeScore computes a composite retrieval score: w_r * relevance + w_t * recency + w_i * importance.
// recencyDecay applies exponential decay based on age in days.
func CompositeScore(relevance float32, memoryAge time.Time, importance float64, weights RankingWeights) float64 {
	age := time.Since(memoryAge).Hours() / 24.0
	halfLife := weights.RecencyHalfLife
	if halfLife <= 0 {
		halfLife = 30
	}
	recency := math.Exp(-0.693 * age / halfLife)

	totalWeight := weights.RelevanceWeight + weights.RecencyWeight + weights.ImportanceWeight
	if totalWeight == 0 {
		totalWeight = 1.0
	}

	score := (weights.RelevanceWeight*float64(relevance) +
		weights.RecencyWeight*recency +
		weights.ImportanceWeight*importance) / totalWeight

	return score
}

// RankSearchResults re-ranks search results using composite scoring.
func RankSearchResults(results []SearchResult, weights RankingWeights) []SearchResult {
	if len(results) == 0 {
		return results
	}
	type scoredResult struct {
		result    SearchResult
		composite float64
	}
	scored := make([]scoredResult, len(results))
	for i, r := range results {
		scored[i] = scoredResult{
			result:    r,
			composite: CompositeScore(r.Score, r.Memory.UpdatedAt, r.Memory.Importance, weights),
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].composite > scored[j].composite
	})

	ranked := make([]SearchResult, len(scored))
	for i, s := range scored {
		ranked[i] = s.result
	}
	return ranked
}

// FactExists checks if a memory with the given content hash already exists.
func (db *DB) FactExists(contentHash string) (bool, error) {
	var count int
	err := db.conn.QueryRow(
		"SELECT 1 FROM memories WHERE content_hash = ? LIMIT 1",
		contentHash,
	).Scan(&count)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// SetMemoryEmbedding updates only the embedding columns of an existing memory,
// leaving all other fields (identity, timestamps, content, metadata) untouched.
func (db *DB) SetMemoryEmbedding(id string, embedding []float32, source, model string) error {
	embeddingJSON, err := json.Marshal(embedding)
	if err != nil {
		return fmt.Errorf("failed to marshal embedding: %w", err)
	}
	embeddingDim := len(embedding)
	lshHash := ComputeLSH(embedding)

	_, err = db.conn.Exec(
		`UPDATE memories SET embedding = ?, embedding_dim = ?, embedding_source = ?, embedding_model = ?, lsh_hash = ? WHERE id = ?`,
		string(embeddingJSON), embeddingDim, source, model, lshHash, id,
	)
	return err
}

// SupersedeFact marks an older memory as superseded by a newer one.
// Sets valid_to on the superseded memory and records the superseding ID.
func (db *DB) SupersedeFact(supersededID, supersededByID string) error {
	now := time.Now().UTC()
	_, err := db.conn.Exec(
		`UPDATE memories SET valid_to = ?, superseded_by = ?, updated_at = ? WHERE id = ? AND (valid_to IS NULL OR valid_to > ?)`,
		now, supersededByID, now, supersededID, now,
	)
	return err
}

// GetSupersededHistory returns all memories that were superseded by the given ID.
func (db *DB) GetSupersededHistory(supersededByID string) ([]*Memory, error) {
	rows, err := db.conn.Query(
		"SELECT id, content, scope, metadata, created_at, updated_at, created_by, updated_by, created_session, updated_session, consolidation_status, consolidated_into_id, importance, valid_from, valid_to, superseded_by FROM memories WHERE superseded_by = ? ORDER BY valid_from DESC",
		supersededByID,
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
