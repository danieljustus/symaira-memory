package codex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/danieljustus/symaira-memory/internal/importer"
)

// CodexImporter imports sessions from Codex CLI.
type CodexImporter struct {
	customPath string
}

// CodexSession represents a Codex CLI session JSON file.
type CodexSession struct {
	Messages []CodexMessage `json:"messages"`
}

type CodexMessage struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

func NewCodexImporter(customPath string) *CodexImporter {
	return &CodexImporter{customPath: customPath}
}

func (c *CodexImporter) Name() string {
	return "codex"
}

func (c *CodexImporter) DiscoverSessions(since time.Time) ([]importer.SessionRef, error) {
	basePath := c.customPath
	if basePath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		basePath = filepath.Join(home, ".codex")
	}

	var sessions []importer.SessionRef

	err := filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}
		if info.ModTime().Before(since) {
			return nil
		}

		sessions = append(sessions, importer.SessionRef{
			Tool:       "codex",
			SessionID:  filepath.Base(path),
			Path:       path,
			ModifiedAt: info.ModTime(),
			Metadata:   map[string]string{},
		})
		return nil
	})

	return sessions, err
}

func (c *CodexImporter) ImportSession(ref importer.SessionRef) ([]importer.ImportedFact, error) {
	data, err := os.ReadFile(ref.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to read session file: %w", err)
	}

	var session CodexSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to parse session JSON: %w", err)
	}

	var facts []importer.ImportedFact

	for _, msg := range session.Messages {
		if msg.Role == "assistant" && len(msg.Content) > 50 {
			facts = append(facts, importer.ImportedFact{
				Content:   msg.Content,
				Source:    "codex",
				SessionID: ref.SessionID,
				Timestamp: msg.Timestamp,
				Metadata:  map[string]string{},
			})
		}
	}

	return facts, nil
}
