package codex

import (
	"testing"
)

func TestNewCodexImporter(t *testing.T) {
	imp := NewCodexImporter("/tmp/codex")

	if imp.Name() != "codex" {
		t.Errorf("Name() = %q, want %q", imp.Name(), "codex")
	}
}

func TestNewCodexImporterDefaults(t *testing.T) {
	imp := NewCodexImporter("")

	if imp.customPath != "" {
		t.Errorf("customPath = %q, want empty", imp.customPath)
	}
}
