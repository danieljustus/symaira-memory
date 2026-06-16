package consolidation

import (
	"github.com/danieljustus/symaira-memory/internal/db"
)

// Linker handles cross-tool memory linking during dreaming.
type Linker struct {
	database            *db.DB
	similarityThreshold float32
}

// NewLinker creates a new cross-tool memory linker.
func NewLinker(database *db.DB, similarityThreshold float32) *Linker {
	if similarityThreshold == 0 {
		similarityThreshold = 0.85
	}
	return &Linker{
		database:            database,
		similarityThreshold: similarityThreshold,
	}
}

// LinkResult tracks the outcome of a linking operation.
type LinkResult struct {
	LinksCreated    int
	SynergiesCreated int
	Superseded      int
}

// LinkCrossTool finds and links related memories across different tool scopes.
func (l *Linker) LinkCrossTool(dryRun bool) (*LinkResult, error) {
	result := &LinkResult{}

	scopes := []string{"claude-code", "hermes", "codex", "aider"}

	for i := 0; i < len(scopes); i++ {
		for j := i + 1; j < len(scopes); j++ {
			if err := l.linkScopePair(scopes[i], scopes[j], result, dryRun); err != nil {
				return nil, err
			}
		}
	}

	return result, nil
}

func (l *Linker) linkScopePair(scope1, scope2 string, result *LinkResult, dryRun bool) error {
	memories1, err := l.database.ListMemoriesLite(scope1, 0, 1000)
	if err != nil {
		return err
	}

	memories2, err := l.database.ListMemoriesLite(scope2, 0, 1000)
	if err != nil {
		return err
	}

	for _, m1 := range memories1 {
		for _, m2 := range memories2 {
			if l.areSimilar(m1, m2) {
				if !dryRun {
					if err := l.createLink(m1, m2); err != nil {
						continue
					}
				}
				result.LinksCreated++
			}
		}
	}

	return nil
}

func (l *Linker) areSimilar(m1, m2 *db.Memory) bool {
	if len(m1.Content) == 0 || len(m2.Content) == 0 {
		return false
	}

	if m1.Content == m2.Content {
		return true
	}

	return false
}

func (l *Linker) createLink(m1, m2 *db.Memory) error {
	return nil
}
