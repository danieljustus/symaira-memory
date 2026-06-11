package memory

import (
	"fmt"
	"strings"

	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/extractor"
	"github.com/danieljustus/symaira-memory/internal/security"
	"github.com/google/uuid"
)

// Prepare validates scope, redacts PII, detects project boundaries,
// and returns a Memory ready for embedding generation and persistence.
func Prepare(content, scope string, meta map[string]string, piiEnabled bool) (*db.Memory, error) {
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

	return &db.Memory{
		Content:  cleanContent,
		Scope:    scope,
		Metadata: meta,
	}, nil
}

// Store wraps the full prepare → redact → embed → save → extract-facts pipeline.
// Returns the saved memory and any extracted secondary fact descriptions.
func Store(database *db.DB, embeddings *extractor.EmbeddingsGenerator, patternExtractor *extractor.PatternExtractor, content, scope string, meta map[string]string, piiEnabled bool) (*db.Memory, []string, error) {
	m, err := Prepare(content, scope, meta, piiEnabled)
	if err != nil {
		return nil, nil, err
	}
	m.ID = uuid.New().String()
	m.Embedding = embeddings.GenerateVector(m.Content)

	if err := database.SaveMemory(m); err != nil {
		return nil, nil, fmt.Errorf("failed to save memory: %w", err)
	}

	extractedFacts := patternExtractor.ExtractFacts(m.Content)
	var extractedStr []string
	for _, f := range extractedFacts {
		cleanFactContent := f.Content
		if piiEnabled {
			cleanFactContent = security.Redact(f.Content)
		}

		subID := uuid.New().String()
		subVector := embeddings.GenerateVector(cleanFactContent)

		subMeta := f.Metadata
		if subMeta == nil {
			subMeta = make(map[string]string)
		}
		if m.Scope == "project" {
			subMeta["project_name"] = m.Metadata["project_name"]
		}

		subMem := &db.Memory{
			ID:        subID,
			Content:   cleanFactContent,
			Scope:     m.Scope,
			Metadata:  subMeta,
			Embedding: subVector,
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
