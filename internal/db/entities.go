package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/danieljustus/symaira-memory/internal/entity"
)

// Entity represents a person, project, organization, or other named entity
// that can be linked to multiple memories for cross-referencing.
type Entity struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"` // person | project | org | other
	Aliases     []string  `json:"aliases"`
	Description string    `json:"description"`
	CreatedBy   string    `json:"created_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// SaveEntity inserts or updates an entity.
func (db *DB) SaveEntity(e *Entity) error {
	aliasesJSON, err := json.Marshal(e.Aliases)
	if err != nil {
		return fmt.Errorf("marshal entity aliases: %w", err)
	}

	now := time.Now().UTC()
	if e.CreatedAt.IsZero() {
		e.CreatedAt = now
	}
	e.UpdatedAt = now

	query := `INSERT INTO entities (id, name, type, aliases, description, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name,
			type=excluded.type,
			aliases=excluded.aliases,
			description=excluded.description,
			updated_at=excluded.updated_at`

	_, err = db.conn.Exec(query, e.ID, e.Name, e.Type, string(aliasesJSON), e.Description, e.CreatedBy, e.CreatedAt, e.UpdatedAt)
	if err != nil {
		return err
	}

	_, _ = db.conn.Exec("DELETE FROM entities_aliases WHERE entity_id = ?", e.ID)
	for _, alias := range e.Aliases {
		_, _ = db.conn.Exec("INSERT OR IGNORE INTO entities_aliases (entity_id, alias) VALUES (?, ?)", e.ID, alias)
	}
	return nil
}

// ResolveEntity finds an entity by name (case-insensitive) or by alias match.
// Returns nil, nil if not found.
func (db *DB) ResolveEntity(nameOrAlias string) (*Entity, error) {
	query := `SELECT id, name, type, aliases, description, created_by, created_at, updated_at
		FROM entities WHERE name = ? COLLATE NOCASE`

	var e Entity
	var aliasesStr string
	err := db.conn.QueryRow(query, nameOrAlias).Scan(
		&e.ID, &e.Name, &e.Type, &aliasesStr, &e.Description,
		&e.CreatedBy, &e.CreatedAt, &e.UpdatedAt,
	)
	if err == nil {
		if err := json.Unmarshal([]byte(aliasesStr), &e.Aliases); err != nil {
			return nil, err
		}
		return &e, nil
	}
	if err != sql.ErrNoRows {
		return nil, err
	}

	aliasQuery := `SELECT e.id, e.name, e.type, e.aliases, e.description, e.created_by, e.created_at, e.updated_at
		FROM entities e
		INNER JOIN entities_aliases ea ON ea.entity_id = e.id
		WHERE ea.alias = ? COLLATE NOCASE`

	err = db.conn.QueryRow(aliasQuery, nameOrAlias).Scan(
		&e.ID, &e.Name, &e.Type, &aliasesStr, &e.Description,
		&e.CreatedBy, &e.CreatedAt, &e.UpdatedAt,
	)
	if err == nil {
		if err := json.Unmarshal([]byte(aliasesStr), &e.Aliases); err != nil {
			return nil, err
		}
		return &e, nil
	}
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return nil, err
}

// EntityCandidate is a scored, explainable entity match returned by
// ResolveEntityCandidates. Unlike ResolveEntity, it can represent ambiguity
// by returning multiple candidates and never mutates entity state.
type EntityCandidate struct {
	EntityID    string   `json:"entity_id"`
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Aliases     []string `json:"aliases"`
	Score       float64  `json:"score"`
	MatchKind   string   `json:"match_kind"`
	MatchReason string   `json:"match_reason"`
}

// ResolveEntityCandidates returns every entity matching query — or one of
// the optional aliasHints — by exact or normalized name/alias comparison,
// ranked by match strength (highest score first). entityType, when
// non-empty, strictly restricts results to that exact type. aliasHints are
// comparison hints only: they are never stored, and any hint that looks like
// an email address or phone number is dropped before matching so contact
// identifiers never become implicit Memory data. Matching is read-only and
// never mutates entity aliases or timestamps.
func (db *DB) ResolveEntityCandidates(query string, entityType string, aliasHints []string, limit int) ([]EntityCandidate, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query must not be empty")
	}
	if len(query) > entity.MaxHintLength {
		return nil, fmt.Errorf("query exceeds maximum length of %d characters", entity.MaxHintLength)
	}

	hints := make([]string, 0, len(aliasHints)+1)
	hints = append(hints, query)
	for _, a := range aliasHints {
		a = strings.TrimSpace(a)
		if a == "" || len(a) > entity.MaxHintLength || entity.IsPII(a) {
			continue
		}
		hints = append(hints, a)
	}

	entities, err := db.ListEntities()
	if err != nil {
		return nil, err
	}

	var candidates []EntityCandidate
	for _, e := range entities {
		if entityType != "" && e.Type != entityType {
			continue
		}
		kind, reason, score, ok := entity.BestMatch(hints, e.Name, e.Aliases)
		if !ok {
			continue
		}
		candidates = append(candidates, EntityCandidate{
			EntityID:    e.ID,
			Name:        e.Name,
			Type:        e.Type,
			Aliases:     e.Aliases,
			Score:       score,
			MatchKind:   string(kind),
			MatchReason: reason,
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		if candidates[i].MatchKind != candidates[j].MatchKind {
			return candidates[i].MatchKind < candidates[j].MatchKind
		}
		ni, nj := entity.Normalize(candidates[i].Name), entity.Normalize(candidates[j].Name)
		if ni != nj {
			return ni < nj
		}
		return candidates[i].EntityID < candidates[j].EntityID
	})

	if limit > 0 && len(candidates) > limit {
		candidates = candidates[:limit]
	}
	return candidates, nil
}

// GetEntityByName retrieves an entity by its exact name (case-insensitive).
func (db *DB) GetEntityByName(name string) (*Entity, error) {
	query := `SELECT id, name, type, aliases, description, created_by, created_at, updated_at
		FROM entities WHERE name = ? COLLATE NOCASE`

	var e Entity
	var aliasesStr string
	err := db.conn.QueryRow(query, name).Scan(
		&e.ID, &e.Name, &e.Type, &aliasesStr, &e.Description,
		&e.CreatedBy, &e.CreatedAt, &e.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(aliasesStr), &e.Aliases); err != nil {
		return nil, err
	}
	return &e, nil
}

// GetEntityByID retrieves an entity by its ID. Returns nil, nil if not found.
func (db *DB) GetEntityByID(id string) (*Entity, error) {
	query := `SELECT id, name, type, aliases, description, created_by, created_at, updated_at
		FROM entities WHERE id = ?`

	var e Entity
	var aliasesStr string
	err := db.conn.QueryRow(query, id).Scan(
		&e.ID, &e.Name, &e.Type, &aliasesStr, &e.Description,
		&e.CreatedBy, &e.CreatedAt, &e.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(aliasesStr), &e.Aliases); err != nil {
		return nil, err
	}
	return &e, nil
}

// ListEntities returns all stored entities.
func (db *DB) ListEntities() ([]*Entity, error) {
	query := `SELECT id, name, type, aliases, description, created_by, created_at, updated_at
		FROM entities ORDER BY name COLLATE NOCASE`

	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entities []*Entity
	for rows.Next() {
		var e Entity
		var aliasesStr string
		if err := rows.Scan(&e.ID, &e.Name, &e.Type, &aliasesStr, &e.Description, &e.CreatedBy, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(aliasesStr), &e.Aliases); err != nil {
			return nil, err
		}
		entities = append(entities, &e)
	}
	return entities, nil
}

// DeleteEntity removes an entity by ID and its memory links.
func (db *DB) DeleteEntity(id string) error {
	_, _ = db.conn.Exec("DELETE FROM memory_entities WHERE entity_id = ?", id)
	_, _ = db.conn.Exec("DELETE FROM entities_aliases WHERE entity_id = ?", id)
	_, err := db.conn.Exec("DELETE FROM entities WHERE id = ?", id)
	return err
}

// LinkMemoryToEntity creates a link between a memory and an entity.
func (db *DB) LinkMemoryToEntity(memoryID, entityID string) error {
	_, err := db.conn.Exec(
		"INSERT OR IGNORE INTO memory_entities (memory_id, entity_id) VALUES (?, ?)",
		memoryID, entityID,
	)
	return err
}

// UnlinkMemoryFromEntity removes a link between a memory and an entity.
func (db *DB) UnlinkMemoryFromEntity(memoryID, entityID string) error {
	_, err := db.conn.Exec(
		"DELETE FROM memory_entities WHERE memory_id = ? AND entity_id = ?",
		memoryID, entityID,
	)
	return err
}

// EntitiesForMemory returns all entities linked to a given memory.
func (db *DB) EntitiesForMemory(memoryID string) ([]*Entity, error) {
	query := `SELECT e.id, e.name, e.type, e.aliases, e.description, e.created_by, e.created_at, e.updated_at
		FROM entities e
		INNER JOIN memory_entities me ON me.entity_id = e.id
		WHERE me.memory_id = ?
		ORDER BY e.name COLLATE NOCASE`

	rows, err := db.conn.Query(query, memoryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entities []*Entity
	for rows.Next() {
		var e Entity
		var aliasesStr string
		if err := rows.Scan(&e.ID, &e.Name, &e.Type, &aliasesStr, &e.Description, &e.CreatedBy, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(aliasesStr), &e.Aliases); err != nil {
			return nil, err
		}
		entities = append(entities, &e)
	}
	return entities, nil
}

// MemoryIDsForEntity returns all memory IDs linked to a given entity.
func (db *DB) MemoryIDsForEntity(entityID string) ([]string, error) {
	rows, err := db.conn.Query(
		"SELECT memory_id FROM memory_entities WHERE entity_id = ?",
		entityID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}
