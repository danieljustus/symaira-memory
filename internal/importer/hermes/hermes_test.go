package hermes

import (
	"testing"
)

func TestNewHermesImporter(t *testing.T) {
	imp := NewHermesImporter("/tmp/hermes")

	if imp.Name() != "hermes" {
		t.Errorf("Name() = %q, want %q", imp.Name(), "hermes")
	}
}

func TestNewHermesImporterDefaults(t *testing.T) {
	imp := NewHermesImporter("")

	if imp.customPath != "" {
		t.Errorf("customPath = %q, want empty", imp.customPath)
	}
}
