package memory

import (
	"fmt"
	"strings"
	"time"

	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/extractor"
	"github.com/danieljustus/symaira-memory/internal/security"
	"github.com/google/uuid"
)

// Attribution carries provenance information about who created or updated a
// memory and in which session.
type Attribution struct {
	Author    string // Profile name, "cli:<user>" or JWT Subject
	SessionID string // e.g. Hermes session ID, empty if unknown
}

// Prepare validates scope, redacts PII, detects project boundaries,
// and returns a Memory ready for embedding generation and persistence.
func Prepare(content, scope string, meta map[string]string, piiEnabled bool, attr Attribution) (*db.Memory, error) {
	if scope == "" {
		scope = "global"
	}
	if err := security.ValidateScope(scope); err != nil {
		return nil, err
	}

	cleanContent := content
	if piiEnabled {
		cleanContent = security.Redact(content)
	}

	if meta == nil {
		meta = make(map[string]string)
	}

	if scope == "project" {
		detector := security.NewProjectScopeDetector()
		meta["project_name"] = detector.DetectActiveProject()
	}

	cleanMeta := meta
	if piiEnabled {
		cleanMeta = security.RedactMap(meta)
	}

	return &db.Memory{
		Content:        cleanContent,
		Scope:          scope,
		Metadata:       cleanMeta,
		CreatedBy:      attr.Author,
		UpdatedBy:      attr.Author,
		CreatedSession: attr.SessionID,
		UpdatedSession: attr.SessionID,
		ValidFrom:      ptrTime(time.Now().UTC()),
	}, nil
}

// Store wraps the full prepare → redact → embed → save → extract-facts pipeline.
// Returns the saved memory and any extracted secondary fact descriptions.
// The entities parameter contains entity names to link to the saved memory.
func Store(database *db.DB, embeddings *extractor.EmbeddingsGenerator, patternExtractor *extractor.PatternExtractor, content, scope string, meta map[string]string, piiEnabled bool, attr Attribution, entities []string) (*db.Memory, []string, error) {
	m, err := Prepare(content, scope, meta, piiEnabled, attr)
	if err != nil {
		return nil, nil, err
	}
	m.ID = uuid.New().String()
	emb := embeddings.GenerateVector(m.Content)
	m.Embedding = emb.Vector
	m.EmbeddingSource = emb.Source
	m.EmbeddingModel = emb.Model

	if err := database.SaveMemory(m); err != nil {
		return nil, nil, fmt.Errorf("failed to save memory: %w", err)
	}

	for _, name := range entities {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		entity, err := database.ResolveEntity(name)
		if err != nil {
			continue
		}
		if entity == nil {
			entity = &db.Entity{
				ID:        uuid.New().String(),
				Name:      name,
				Type:      "other",
				Aliases:   []string{},
				CreatedBy: attr.Author,
				CreatedAt: time.Now().UTC(),
			}
			if err := database.SaveEntity(entity); err != nil {
				continue
			}
		}
		_ = database.LinkMemoryToEntity(m.ID, entity.ID)
		m.Entities = append(m.Entities, entity.Name)
	}

	extractedFacts := patternExtractor.ExtractFacts(m.Content)
	var extractedStr []string
	for _, f := range extractedFacts {
		cleanFactContent := f.Content
		if piiEnabled {
			cleanFactContent = security.Redact(f.Content)
		}

		subID := uuid.New().String()
		subEmb := embeddings.GenerateVector(cleanFactContent)
		subVector := subEmb.Vector

		subMeta := f.Metadata
		if subMeta == nil {
			subMeta = make(map[string]string)
		}
		if m.Scope == "project" {
			subMeta["project_name"] = m.Metadata["project_name"]
		}
		if piiEnabled {
			subMeta = security.RedactMap(subMeta)
		}

		subMem := &db.Memory{
			ID:              subID,
			Content:         cleanFactContent,
			Scope:           m.Scope,
			Metadata:        subMeta,
			Embedding:       subVector,
			EmbeddingSource: subEmb.Source,
			EmbeddingModel:  subEmb.Model,
			CreatedBy:       attr.Author,
			UpdatedBy:       attr.Author,
			CreatedSession:  attr.SessionID,
			UpdatedSession:  attr.SessionID,
		}
		if err := database.SaveMemory(subMem); err == nil {
			extractedStr = append(extractedStr, fmt.Sprintf("  - [Fact Extracted] %s (ID: %s)", cleanFactContent, subID))
		}
	}

	return m, extractedStr, nil
}

// FormatStoreSuccess builds a human-readable success message for a stored memory.
func FormatStoreSuccess(m *db.Memory, extractedStr []string) string {
	responseMsg := fmt.Sprintf("Successfully saved memory!\nMemory ID: %s\nContent: %s\nScope: %s", m.ID, m.Content, m.Scope)
	if m.Scope == "project" {
		responseMsg += fmt.Sprintf("\nProject: %s", m.Metadata["project_name"])
	}
	if len(extractedStr) > 0 {
		responseMsg += "\n\nAdditionally, secondary facts were successfully extracted:\n" + strings.Join(extractedStr, "\n")
	}
	return responseMsg
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
