package aider

import (
	"testing"
)

func TestNewAiderImporter(t *testing.T) {
	imp := NewAiderImporter([]string{"/tmp/project1", "/tmp/project2"})

	if imp.Name() != "aider" {
		t.Errorf("Name() = %q, want %q", imp.Name(), "aider")
	}
}

func TestNewAiderImporterDefaults(t *testing.T) {
	imp := NewAiderImporter(nil)

	if len(imp.customPaths) != 0 {
		t.Errorf("customPaths = %v, want empty", imp.customPaths)
	}
}
