package db

import (
	"fmt"
	"time"
)

// MaxGraphDepth bounds how far GraphNeighbors will traverse, so a query
// against a large, densely connected graph cannot run away.
const MaxGraphDepth = 3

// EntityRelation is a directed, typed relationship between two entities,
// e.g. "Daniel works-with Musterland Bank".
type EntityRelation struct {
	FromEntityID string    `json:"from_entity_id"`
	ToEntityID   string    `json:"to_entity_id"`
	RelationType string    `json:"relation_type"`
	CreatedBy    string    `json:"created_by,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// SaveEntityRelation creates a directed relation between two entities. It is
// idempotent: creating the same (from, to, type) triple again is a no-op.
func (db *DB) SaveEntityRelation(r *EntityRelation) error {
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	_, err := db.conn.Exec(
		`INSERT OR IGNORE INTO entity_relations (from_entity_id, to_entity_id, relation_type, created_by, created_at)
			VALUES (?, ?, ?, ?, ?)`,
		r.FromEntityID, r.ToEntityID, r.RelationType, r.CreatedBy, r.CreatedAt,
	)
	return err
}

// DeleteEntityRelation removes a specific directed relation.
func (db *DB) DeleteEntityRelation(fromEntityID, toEntityID, relationType string) error {
	_, err := db.conn.Exec(
		"DELETE FROM entity_relations WHERE from_entity_id = ? AND to_entity_id = ? AND relation_type = ?",
		fromEntityID, toEntityID, relationType,
	)
	return err
}

// OutgoingRelations returns every relation where entityID is the source.
func (db *DB) OutgoingRelations(entityID string) ([]*EntityRelation, error) {
	return db.queryRelations(
		"SELECT from_entity_id, to_entity_id, relation_type, created_by, created_at FROM entity_relations WHERE from_entity_id = ? ORDER BY relation_type, to_entity_id",
		entityID,
	)
}

// IncomingRelations returns every relation where entityID is the target.
func (db *DB) IncomingRelations(entityID string) ([]*EntityRelation, error) {
	return db.queryRelations(
		"SELECT from_entity_id, to_entity_id, relation_type, created_by, created_at FROM entity_relations WHERE to_entity_id = ? ORDER BY relation_type, from_entity_id",
		entityID,
	)
}

// RelationsForEntity returns every relation touching entityID, in either
// direction (outgoing first, then incoming).
func (db *DB) RelationsForEntity(entityID string) ([]*EntityRelation, error) {
	out, err := db.OutgoingRelations(entityID)
	if err != nil {
		return nil, err
	}
	in, err := db.IncomingRelations(entityID)
	if err != nil {
		return nil, err
	}
	return append(out, in...), nil
}

// GraphNeighbors performs a breadth-first traversal of entity_relations
// starting at entityID out to depth hops (1 means direct relations only).
// It returns every entity reached (including the starting entity) and every
// distinct edge traversed. Cycles are handled: a node or edge is never
// visited/returned twice regardless of how many paths reach it.
func (db *DB) GraphNeighbors(entityID string, depth int) ([]*Entity, []*EntityRelation, error) {
	if depth < 1 {
		depth = 1
	}
	if depth > MaxGraphDepth {
		return nil, nil, fmt.Errorf("depth %d exceeds maximum allowed depth %d", depth, MaxGraphDepth)
	}

	visited := map[string]bool{entityID: true}
	edgeSeen := map[string]bool{}
	var edges []*EntityRelation

	frontier := []string{entityID}
	for hop := 0; hop < depth && len(frontier) > 0; hop++ {
		var next []string
		for _, id := range frontier {
			rels, err := db.RelationsForEntity(id)
			if err != nil {
				return nil, nil, err
			}
			for _, r := range rels {
				key := r.FromEntityID + "\x00" + r.ToEntityID + "\x00" + r.RelationType
				if !edgeSeen[key] {
					edgeSeen[key] = true
					edges = append(edges, r)
				}
				other := r.ToEntityID
				if other == id {
					other = r.FromEntityID
				}
				if !visited[other] {
					visited[other] = true
					next = append(next, other)
				}
			}
		}
		frontier = next
	}

	nodes := make([]*Entity, 0, len(visited))
	for id := range visited {
		e, err := db.GetEntityByID(id)
		if err != nil {
			return nil, nil, err
		}
		if e != nil {
			nodes = append(nodes, e)
		}
	}

	return nodes, edges, nil
}

func (db *DB) queryRelations(query, entityID string) ([]*EntityRelation, error) {
	rows, err := db.conn.Query(query, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var relations []*EntityRelation
	for rows.Next() {
		var r EntityRelation
		if err := rows.Scan(&r.FromEntityID, &r.ToEntityID, &r.RelationType, &r.CreatedBy, &r.CreatedAt); err != nil {
			return nil, err
		}
		relations = append(relations, &r)
	}
	return relations, rows.Err()
}
