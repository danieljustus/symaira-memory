package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareEmptyScopeDefaultsToGlobal(t *testing.T) {
	mem, err := Prepare("test content", "", nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mem.Scope != "global" {
		t.Errorf("expected scope 'global', got '%s'", mem.Scope)
	}
}

func TestPrepareInvalidScopeReturnsError(t *testing.T) {
	mem, err := Prepare("test content", "banana", nil, false)
	if err == nil {
		t.Fatal("expected error for invalid scope, got nil")
	}
	if mem != nil {
		t.Errorf("expected nil memory on error, got %+v", mem)
	}
}

func TestPreparePIIRedaction(t *testing.T) {
	content := "contact me at john@example.com for details"
	mem, err := Prepare(content, "global", nil, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(mem.Content, "john@example.com") {
		t.Errorf("expected PII to be redacted, but content still contains email: %s", mem.Content)
	}
}

func TestPreparePIIDisabled(t *testing.T) {
	content := "contact me at john@example.com for details"
	mem, err := Prepare(content, "global", nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mem.Content != content {
		t.Errorf("expected content unchanged when PII disabled, got '%s'", mem.Content)
	}
}

func TestPrepareNilMetaBecomesEmptyMap(t *testing.T) {
	mem, err := Prepare("test content", "global", nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mem.Metadata == nil {
		t.Error("expected non-nil metadata map, got nil")
	}
	if len(mem.Metadata) != 0 {
		t.Errorf("expected empty metadata map, got %d entries", len(mem.Metadata))
	}
}

func TestPrepareProjectScopeSetsProjectName(t *testing.T) {
	// Create temp dir with .git directory to simulate project root
	tempDir, err := os.MkdirTemp("", "symmemory-memory-test-*")
	if err != nil {
		t.Fatalf("failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	gitDir := filepath.Join(tempDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("failed to create .git directory: %v", err)
	}

	// Save original cwd and switch to temp dir
	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}
	defer os.Chdir(oldCwd)

	mem, err := Prepare("test content", "project", nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mem.Metadata["project_name"] == "" {
		t.Error("expected project_name to be set in metadata for project scope")
	}
}
