package security

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectScopeDetection(t *testing.T) {
	detector := NewProjectScopeDetector()
	
	// Test on standard local directory stat
	active := detector.DetectActiveProject()
	if active == "" {
		t.Errorf("scope detector returned empty project name")
	}

	// Mock parent directory scan in a temporary test directory
	tempDir, err := os.MkdirTemp("", "symmemory-scope-*")
	if err != nil {
		t.Fatalf("failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a nested subfolder mimicking an active project root
	projectRoot := filepath.Join(tempDir, "symaira-test-repo")
	nestedFolder := filepath.Join(projectRoot, "internal", "nested", "src")
	
	if err := os.MkdirAll(nestedFolder, 0755); err != nil {
		t.Fatalf("failed to create nested subfolder structure: %v", err)
	}

	// Create a mock .symmemory.toml at projectRoot to establish project boundary
	tomlFile := filepath.Join(projectRoot, ".symmemory.toml")
	if err := os.WriteFile(tomlFile, []byte("# Mock project configuration"), 0644); err != nil {
		t.Fatalf("failed to write mock .symmemory.toml: %v", err)
	}

	// Save original Cwd
	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}

	// Shift working dir to nested subfolder
	if err := os.Chdir(nestedFolder); err != nil {
		t.Fatalf("failed to shift Cwd to nested subfolder: %v", err)
	}
	defer os.Chdir(oldCwd)

	// Detect and verify!
	detectedProject := detector.DetectActiveProject()
	if detectedProject != "symaira-test-repo" {
		t.Errorf("expected detected project 'symaira-test-repo', got '%s'", detectedProject)
	}
}
