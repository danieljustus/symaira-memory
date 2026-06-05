package memory

import (
	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/security"
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
		piiGuard := security.NewPIIGuard()
		cleanContent = piiGuard.Redact(content)
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
