package importer

import (
	"time"
)

// SessionRef represents a discovered session from an external tool.
type SessionRef struct {
	Tool       string            // "claude-code", "codex", "hermes", "aider"
	SessionID  string            // Unique identifier for the session
	Path       string            // File path or DB location
	ModifiedAt time.Time         // Last modification time
	Metadata   map[string]string // Tool-specific metadata
}

// ImportedFact represents a memory fact extracted from an external session.
type ImportedFact struct {
	Content   string            // The memory content
	Source    string            // Tool name (e.g., "claude-code")
	SessionID string            // Session identifier
	Timestamp time.Time         // When the fact was created
	Metadata  map[string]string // Tool-specific context
}

// SessionImporter defines the interface for importing sessions from external tools.
type SessionImporter interface {
	// Name returns the tool name (e.g., "claude-code", "hermes")
	Name() string

	// DiscoverSessions finds sessions modified since the given time.
	DiscoverSessions(since time.Time) ([]SessionRef, error)

	// ImportSession imports a single session and returns extracted facts.
	ImportSession(ref SessionRef) ([]ImportedFact, error)
}
