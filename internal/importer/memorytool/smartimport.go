package memorytool

import (
	"crypto/sha256"
	"fmt"

	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/security"
	"github.com/google/uuid"
)

// SmartImporter handles intelligent deduplication and linking during import.
type SmartImporter struct {
	database *db.DB
}

// NewSmartImporter creates a new smart importer.
func NewSmartImporter(database *db.DB) *SmartImporter {
	return &SmartImporter{database: database}
}

// ImportFacts intelligently imports facts with deduplication.
func (s *SmartImporter) ImportFacts(facts []ImportedFact, dryRun bool) (*ImportResult, error) {
	result := &ImportResult{}

	for _, fact := range facts {
		contentHash := computeContentHash(fact.Content)

		exists, err := s.database.FactExists(contentHash)
		if err != nil {
			return nil, fmt.Errorf("failed to check fact existence: %w", err)
		}

		if exists {
			result.Skipped++
			continue
		}

		if !dryRun {
			if err := s.storeFact(fact); err != nil {
				return nil, fmt.Errorf("failed to store fact: %w", err)
			}
		}

		result.Created++
	}

	return result, nil
}

func (s *SmartImporter) storeFact(fact ImportedFact) error {
	metadata := make(map[string]string)
	for k, v := range fact.Metadata {
		metadata[fmt.Sprintf("%v", k)] = fmt.Sprintf("%v", v)
	}

	memory := &db.Memory{
		ID:                  uuid.New().String(),
		Content:             security.Redact(fact.Content),
		Scope:               "agent",
		Metadata:            security.RedactMap(metadata),
		CreatedAt:           fact.Timestamp,
		ConsolidationStatus: "raw",
	}

	return s.database.SaveMemory(memory)
}

func computeContentHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", h)
}
