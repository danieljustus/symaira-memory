package git

import (
	"testing"
)

func TestNewGitImporter(t *testing.T) {
	imp := NewGitImporter("/tmp/test", "testuser")

	if imp.Name() != "git" {
		t.Errorf("Name() = %q, want %q", imp.Name(), "git")
	}

	if imp.Category() != "code" {
		t.Errorf("Category() = %q, want %q", imp.Category(), "code")
	}

	if imp.PrivacyLevel() != "public" {
		t.Errorf("PrivacyLevel() = %q, want %q", imp.PrivacyLevel(), "public")
	}

	if imp.RequiresPIIGuard() {
		t.Error("RequiresPIIGuard() = true, want false")
	}
}

func TestNewGitImporterDefaults(t *testing.T) {
	imp := NewGitImporter("", "")

	if imp.repoPath != "" {
		t.Errorf("repoPath = %q, want empty", imp.repoPath)
	}

	if imp.author != "" {
		t.Errorf("author = %q, want empty", imp.author)
	}
}
