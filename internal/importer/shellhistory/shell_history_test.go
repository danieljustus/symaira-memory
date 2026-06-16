package shellhistory

import (
	"testing"
)

func TestNewShellHistoryImporter(t *testing.T) {
	imp := NewShellHistoryImporter("/tmp/history", true, []string{"git"})

	if imp.Name() != "shell-history" {
		t.Errorf("Name() = %q, want %q", imp.Name(), "shell-history")
	}

	if imp.Category() != "code" {
		t.Errorf("Category() = %q, want %q", imp.Category(), "code")
	}

	if imp.PrivacyLevel() != "confidential" {
		t.Errorf("PrivacyLevel() = %q, want %q", imp.PrivacyLevel(), "confidential")
	}

	if !imp.RequiresPIIGuard() {
		t.Error("RequiresPIIGuard() = false, want true")
	}
}

func TestNewShellHistoryImporterDefaults(t *testing.T) {
	imp := NewShellHistoryImporter("", false, nil)

	if imp.successOnly {
		t.Error("successOnly = true, want false")
	}

	if len(imp.filters) != 0 {
		t.Errorf("filters = %v, want empty", imp.filters)
	}
}

func TestTagCommand(t *testing.T) {
	tests := []struct {
		cmd  string
		want string
	}{
		{"git status", "vcs"},
		{"npm install", "package-manager"},
		{"brew install foo", "package-manager"},
		{"docker ps", "container"},
		{"make build", "build"},
		{"ls -la", ""},
	}

	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			got := tagCommand(tt.cmd)
			if got != tt.want {
				t.Errorf("tagCommand(%q) = %q, want %q", tt.cmd, got, tt.want)
			}
		})
	}
}
