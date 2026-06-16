package claudecode

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/danieljustus/symaira-memory/internal/importer"
)

// ClaudeCodeImporter imports sessions from Claude Code.
type ClaudeCodeImporter struct {
	customPath string
}

// ClaudeCodeSession represents a Claude Code session JSON file.
type ClaudeCodeSession struct {
	Messages []ClaudeCodeMessage `json:"messages"`
	Project  string              `json:"project"`
	Model    string              `json:"model"`
}

type ClaudeCodeMessage struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

func NewClaudeCodeImporter(customPath string) *ClaudeCodeImporter {
	return &ClaudeCodeImporter{customPath: customPath}
}

func (c *ClaudeCodeImporter) Name() string {
	return "claude-code"
}

func (c *ClaudeCodeImporter) DiscoverSessions(since time.Time) ([]importer.SessionRef, error) {
	basePath := c.customPath
	if basePath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		basePath = filepath.Join(home, ".claude", "projects")
	}

	var sessions []importer.SessionRef

	err := filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}
		if !strings.Contains(path, "sessions") {
			return nil
		}
		if info.ModTime().Before(since) {
			return nil
		}

		sessions = append(sessions, importer.SessionRef{
			Tool:       "claude-code",
			SessionID:  filepath.Base(path),
			Path:       path,
			ModifiedAt: info.ModTime(),
			Metadata:   map[string]string{},
		})
		return nil
	})

	return sessions, err
}

func (c *ClaudeCodeImporter) ImportSession(ref importer.SessionRef) ([]importer.ImportedFact, error) {
	data, err := os.ReadFile(ref.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to read session file: %w", err)
	}

	var session ClaudeCodeSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to parse session JSON: %w", err)
	}

	var facts []importer.ImportedFact

	for _, msg := range session.Messages {
		if msg.Role == "assistant" && len(msg.Content) > 50 {
			facts = append(facts, importer.ImportedFact{
				Content:   msg.Content,
				Source:    "claude-code",
				SessionID: ref.SessionID,
				Timestamp: msg.Timestamp,
				Metadata: map[string]string{
					"project": session.Project,
					"model":   session.Model,
				},
			})
		}
	}

	return facts, nil
}
