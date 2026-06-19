package consolidation

import (
	"fmt"
	"os"
	"strings"

	"github.com/danieljustus/symaira-memory/internal/db"
)

// Linker handles cross-tool memory linking during dreaming.
type Linker struct {
	database            *db.DB
	similarityThreshold float32
	contentThreshold    float64
}

// NewLinker creates a new cross-tool memory linker.
func NewLinker(database *db.DB, similarityThreshold float32) *Linker {
	if similarityThreshold == 0 {
		similarityThreshold = 0.85
	}
	return &Linker{
		database:            database,
		similarityThreshold: similarityThreshold,
		contentThreshold:    0.80,
	}
}

// LinkResult tracks the outcome of a linking operation.
type LinkResult struct {
	LinksCreated     int
	SynergiesCreated int
	Superseded       int
}

// allToolScopes lists the tool scopes that participate in cross-tool linking.
// These match the scopes used by each importer in internal/importer/.
var allToolScopes = []string{
	"claude-code",
	"hermes",
	"codex",
	"aider",
	"openmemory",
	"chatgpt",
	"mem0",
}

// LinkCrossTool finds and links related memories across different tool scopes.
func (l *Linker) LinkCrossTool(dryRun bool) (*LinkResult, error) {
	result := &LinkResult{}

	for i := 0; i < len(allToolScopes); i++ {
		for j := i + 1; j < len(allToolScopes); j++ {
			if err := l.linkScopePair(allToolScopes[i], allToolScopes[j], result, dryRun); err != nil {
				return nil, fmt.Errorf("linking %s<->%s: %w", allToolScopes[i], allToolScopes[j], err)
			}
		}
	}

	return result, nil
}

// linkScopePair compares every non-archived memory in scope1 against scope2.
func (l *Linker) linkScopePair(scope1, scope2 string, result *LinkResult, dryRun bool) error {
	// Load full memories (with embeddings) for cosine similarity.
	memories1, err := l.database.ListMemories(scope1, 0, 1000)
	if err != nil {
		return err
	}
	if len(memories1) == 0 {
		return nil
	}

	memories2, err := l.database.ListMemories(scope2, 0, 1000)
	if err != nil {
		return err
	}
	if len(memories2) == 0 {
		return nil
	}

	for _, m1 := range memories1 {
		for _, m2 := range memories2 {
			sim := l.similarityScore(m1, m2)
			if sim < l.similarityThreshold {
				continue
			}

			if dryRun {
				result.LinksCreated++
				continue
			}

			if err := l.createLink(m1, m2, sim, result); err != nil {
				// Log but continue — a single failed link should not abort the run.
				fmt.Fprintf(os.Stderr, "linker: skip link %s<->%s: %v\n", m1.ID[:8], m2.ID[:8], err)
				continue
			}
		}
	}

	return nil
}

// similarityScore returns a value in [0,1] combining embedding cosine similarity
// with a text-content fallback when embeddings are unavailable.
func (l *Linker) similarityScore(m1, m2 *db.Memory) float32 {
	// Fast path: exact content match is always a perfect score.
	if m1.Content == m2.Content {
		return 1.0
	}

	// Prefer embedding cosine similarity when both embeddings are present.
	if len(m1.Embedding) > 0 && len(m2.Embedding) > 0 {
		return db.CosineSimilarity(m1.Embedding, m2.Embedding)
	}

	// Fall back to Jaccard token similarity on content.
	return float32(l.jaccardContent(m1.Content, m2.Content))
}

// jaccardContent computes Jaccard similarity over whitespace-split tokens.
func (l *Linker) jaccardContent(a, b string) float64 {
	tokensA := tokenize(a)
	tokensB := tokenize(b)
	if len(tokensA) == 0 || len(tokensB) == 0 {
		return 0
	}

	setA := make(map[string]struct{}, len(tokensA))
	for _, t := range tokensA {
		setA[t] = struct{}{}
	}

	intersection := 0
	for _, t := range tokensB {
		if _, ok := setA[t]; ok {
			intersection++
			delete(setA, t) // count each token once
		}
	}

	union := len(tokensA) + len(tokensB) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// tokenize splits on whitespace and lowercases.
func tokenize(s string) []string {
	parts := strings.Fields(strings.ToLower(s))
	return parts
}

// createLink handles the linking of two highly similar memories from different scopes.
//
// Strategy:
//   - Near-duplicates (>=0.95): supersede the older memory, pointing it to the newer
//     via consolidated_into_id. This prevents storage bloat.
//   - Similar (>=threshold, <0.95): annotate both memories' metadata with a
//     cross-tool reference so the user can see the relationship. These are "synergies"
//     — related facts from different tools that complement each other.
func (l *Linker) createLink(m1, m2 *db.Memory, score float32, result *LinkResult) error {
	// Skip if either is already archived (already superseded).
	if m1.ConsolidationStatus == "archived" || m2.ConsolidationStatus == "archived" {
		return nil
	}

	const supersedeThreshold = 0.95

	if score >= supersedeThreshold {
		return l.supersede(m1, m2, result)
	}
	return l.createSynergy(m1, m2, score, result)
}

// supersede marks the older memory as archived and points it at the newer one.
func (l *Linker) supersede(m1, m2 *db.Memory, result *LinkResult) error {
	tx, err := l.database.BeginTransaction()
	if err != nil {
		return err
	}

	// Determine which is older (supersede target) vs newer (canonical).
	var target, canonical *db.Memory
	if m1.CreatedAt.Before(m2.CreatedAt) {
		target, canonical = m1, m2
	} else {
		target, canonical = m2, m1
	}

	if err := l.database.UpdateMemoryStatusTx(tx, target.ID, "archived", canonical.ID); err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to supersede %s: %w", target.ID[:8], err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit supersede: %w", err)
	}

	result.Superseded++
	return nil
}

// createSynergy annotates both memories with cross-tool references.
func (l *Linker) createSynergy(m1, m2 *db.Memory, score float32, result *LinkResult) error {
	tx, err := l.database.BeginTransaction()
	if err != nil {
		return err
	}

	// Add cross-ref to m1's metadata pointing to m2
	if m1.Metadata == nil {
		m1.Metadata = make(map[string]string)
	}
	if m2.Metadata == nil {
		m2.Metadata = make(map[string]string)
	}

	// Avoid duplicate annotations.
	crossRefKey := fmt.Sprintf("cross_ref_%s", m2.Scope)
	if _, exists := m1.Metadata[crossRefKey]; !exists {
		m1.Metadata[crossRefKey] = m2.ID
		m1.UpdatedAt = m1.CreatedAt
		if err := l.database.SaveMemoryTx(tx, m1); err != nil {
			tx.Rollback()
			return fmt.Errorf("annotate m1: %w", err)
		}
	}

	crossRefKey2 := fmt.Sprintf("cross_ref_%s", m1.Scope)
	if _, exists := m2.Metadata[crossRefKey2]; !exists {
		m2.Metadata[crossRefKey2] = m1.ID
		m2.UpdatedAt = m2.CreatedAt
		if err := l.database.SaveMemoryTx(tx, m2); err != nil {
			tx.Rollback()
			return fmt.Errorf("annotate m2: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit synergy: %w", err)
	}

	result.SynergiesCreated++
	return nil
}
