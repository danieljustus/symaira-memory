package importer

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/google/uuid"
)

// Registry manages multiple session importers and orchestrates import runs.
type Registry struct {
	importers map[string]SessionImporter
	database  *db.DB
	state     *ImportState
	stderr    *os.File
}

// NewRegistry creates a new importer registry.
func NewRegistry(database *db.DB) *Registry {
	return &Registry{
		importers: make(map[string]SessionImporter),
		database:  database,
		state:     NewImportState(database.Conn()),
		stderr:    os.Stderr,
	}
}

// Register adds a session importer to the registry.
func (r *Registry) Register(importer SessionImporter) {
	r.importers[importer.Name()] = importer
}

// Get returns an importer by name, or nil if not found.
func (r *Registry) Get(name string) SessionImporter {
	return r.importers[name]
}

// List returns all registered importer names.
func (r *Registry) List() []string {
	names := make([]string, 0, len(r.importers))
	for name := range r.importers {
		names = append(names, name)
	}
	return names
}

// ListByCategory returns importer names filtered by category.
// Only importers implementing Categorizable are matched.
func (r *Registry) ListByCategory(category string) []string {
	var names []string
	for _, imp := range r.importers {
		if cat, ok := imp.(Categorizable); ok && cat.Category() == category {
			names = append(names, imp.Name())
		}
	}
	return names
}

// ListByPrivacyLevel returns importer names whose privacy level is at or below
// the specified threshold. Only importers implementing PrivacyAware are matched.
func (r *Registry) ListByPrivacyLevel(maxLevel PrivacyLevel) []string {
	levels := map[PrivacyLevel]int{
		PrivacyPublic:       0,
		PrivacyInternal:     1,
		PrivacyConfidential: 2,
		PrivacySecret:       3,
	}
	max := levels[maxLevel]

	var names []string
	for _, imp := range r.importers {
		if pa, ok := imp.(PrivacyAware); ok {
			if levels[pa.PrivacyLevel()] <= max {
				names = append(names, imp.Name())
			}
		} else {
			names = append(names, imp.Name())
		}
	}
	return names
}

// ImportResult tracks the outcome of an import run.
type ImportResult struct {
	Tool     string
	Sessions int
	Facts    int
	Skipped  int
	Errors   int
	DryRun   bool
}

// RunImport runs import for specified tools (or all if empty).
func (r *Registry) RunImport(ctx context.Context, tools []string, dryRun bool) ([]ImportResult, error) {
	if len(tools) == 0 {
		tools = r.List()
	}

	var results []ImportResult

	for _, toolName := range tools {
		importer := r.importers[toolName]
		if importer == nil {
			fmt.Fprintf(r.stderr, "Warning: unknown importer %q, skipping\n", toolName)
			continue
		}

		result, err := r.runToolImport(ctx, importer, dryRun)
		if err != nil {
			fmt.Fprintf(r.stderr, "Error importing from %s: %v\n", toolName, err)
			result = &ImportResult{Tool: toolName, Errors: 1, DryRun: dryRun}
		}
		results = append(results, *result)
	}

	return results, nil
}

// runToolImport runs import for a single tool.
func (r *Registry) runToolImport(ctx context.Context, importer SessionImporter, dryRun bool) (*ImportResult, error) {
	result := &ImportResult{
		Tool:   importer.Name(),
		DryRun: dryRun,
	}

	// Get last import time for incremental discovery
	lastImport, err := r.state.GetLastImportTime(importer.Name())
	if err != nil {
		return nil, fmt.Errorf("failed to get last import time: %w", err)
	}

	// If no previous import, start from 30 days ago
	if lastImport.IsZero() {
		lastImport = time.Now().AddDate(0, 0, -30)
	}

	// Discover sessions
	sessions, err := importer.DiscoverSessions(lastImport)
	if err != nil {
		return nil, fmt.Errorf("failed to discover sessions: %w", err)
	}

	fmt.Fprintf(r.stderr, "Found %d sessions from %s\n", len(sessions), importer.Name())

	for _, session := range sessions {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		// Check if already imported
		imported, err := r.state.IsImported(importer.Name(), session.SessionID)
		if err != nil {
			fmt.Fprintf(r.stderr, "Error checking import state for %s: %v\n", session.SessionID, err)
			result.Errors++
			continue
		}

		if imported {
			result.Skipped++
			continue
		}

		// Import the session
		facts, err := importer.ImportSession(session)
		if err != nil {
			fmt.Fprintf(r.stderr, "Error importing session %s: %v\n", session.SessionID, err)
			result.Errors++
			continue
		}

		result.Sessions++
		result.Facts += len(facts)

		if !dryRun {
			// Store facts in database
			if err := r.storeFacts(facts); err != nil {
				fmt.Fprintf(r.stderr, "Error storing facts for session %s: %v\n", session.SessionID, err)
				result.Errors++
				continue
			}

			// Mark as imported
			if err := r.state.MarkImported(importer.Name(), session.SessionID, len(facts)); err != nil {
				fmt.Fprintf(r.stderr, "Error marking session %s as imported: %v\n", session.SessionID, err)
				result.Errors++
			}
		}
	}

	return result, nil
}

// storeFacts stores imported facts in the database.
func (r *Registry) storeFacts(facts []ImportedFact) error {
	for _, fact := range facts {
		now := time.Now().UTC()

		metadata := fact.Metadata
		if metadata == nil {
			metadata = make(map[string]string)
		}
		metadata["source"] = fact.Source
		metadata["session_id"] = fact.SessionID
		metadata["imported_at"] = now.Format(time.RFC3339)

		memory := &db.Memory{
			ID:                  uuid.New().String(),
			Content:             fact.Content,
			Scope:               "agent",
			Metadata:            metadata,
			CreatedAt:           fact.Timestamp,
			UpdatedAt:           now,
			ConsolidationStatus: "raw",
		}

		if err := r.database.SaveMemory(memory); err != nil {
			return fmt.Errorf("failed to save memory: %w", err)
		}
	}
	return nil
}
