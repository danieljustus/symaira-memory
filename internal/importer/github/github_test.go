package github

import (
	"testing"
)

func TestNewGitHubImporter(t *testing.T) {
	imp := NewGitHubImporter("owner", "repo", "token123")

	if imp.Name() != "github" {
		t.Errorf("Name() = %q, want %q", imp.Name(), "github")
	}

	if imp.Category() != "code" {
		t.Errorf("Category() = %q, want %q", imp.Category(), "code")
	}

	if imp.PrivacyLevel() != "internal" {
		t.Errorf("PrivacyLevel() = %q, want %q", imp.PrivacyLevel(), "internal")
	}

	if imp.RequiresPIIGuard() {
		t.Error("RequiresPIIGuard() = true, want false")
	}
}

func TestNewGitHubImporterDefaults(t *testing.T) {
	imp := NewGitHubImporter("", "", "")

	if imp.owner != "" {
		t.Errorf("owner = %q, want empty", imp.owner)
	}

	if imp.repo != "" {
		t.Errorf("repo = %q, want empty", imp.repo)
	}

	if imp.token != "" {
		t.Errorf("token = %q, want empty", imp.token)
	}
}
