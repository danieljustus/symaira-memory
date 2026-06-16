package claudecode

import (
	"testing"
)

func TestNewClaudeCodeImporter(t *testing.T) {
	imp := NewClaudeCodeImporter("/tmp/claude")

	if imp.Name() != "claude-code" {
		t.Errorf("Name() = %q, want %q", imp.Name(), "claude-code")
	}
}

func TestNewClaudeCodeImporterDefaults(t *testing.T) {
	imp := NewClaudeCodeImporter("")

	if imp.customPath != "" {
		t.Errorf("customPath = %q, want empty", imp.customPath)
	}
}
