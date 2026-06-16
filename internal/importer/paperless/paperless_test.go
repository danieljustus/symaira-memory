package paperless

import (
	"testing"
)

func TestNewPaperlessImporter(t *testing.T) {
	imp := NewPaperlessImporter("http://localhost:8000", "token123", "", "", 1000)

	if imp.Name() != "paperless" {
		t.Errorf("Name() = %q, want %q", imp.Name(), "paperless")
	}

	if imp.Category() != "documents" {
		t.Errorf("Category() = %q, want %q", imp.Category(), "documents")
	}

	if imp.PrivacyLevel() != "confidential" {
		t.Errorf("PrivacyLevel() = %q, want %q", imp.PrivacyLevel(), "confidential")
	}

	if !imp.RequiresPIIGuard() {
		t.Error("RequiresPIIGuard() = false, want true")
	}
}

func TestNewPaperlessImporterDefaults(t *testing.T) {
	imp := NewPaperlessImporter("", "", "", "", 0)

	if imp.maxContent != 1000 {
		t.Errorf("maxContent = %d, want %d", imp.maxContent, 1000)
	}
}
