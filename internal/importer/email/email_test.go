package email

import (
	"testing"
)

func TestNewEmailImporter(t *testing.T) {
	imp := NewEmailImporter("INBOX", "high", 2000)

	if imp.Name() != "email" {
		t.Errorf("Name() = %q, want %q", imp.Name(), "email")
	}

	if imp.Category() != "communication" {
		t.Errorf("Category() = %q, want %q", imp.Category(), "communication")
	}

	if imp.PrivacyLevel() != "confidential" {
		t.Errorf("PrivacyLevel() = %q, want %q", imp.PrivacyLevel(), "confidential")
	}

	if !imp.RequiresPIIGuard() {
		t.Error("RequiresPIIGuard() = false, want true")
	}
}

func TestNewEmailImporterDefaults(t *testing.T) {
	imp := NewEmailImporter("", "", 0)

	if imp.folder != "" {
		t.Errorf("folder = %q, want empty", imp.folder)
	}

	if imp.importance != "" {
		t.Errorf("importance = %q, want empty", imp.importance)
	}

	if imp.maxBody != 2000 {
		t.Errorf("maxBody = %d, want %d", imp.maxBody, 2000)
	}
}

func TestMatchesSender(t *testing.T) {
	tests := []struct {
		name           string
		includeSenders []string
		excludeSenders []string
		from           string
		want           bool
	}{
		{
			name:           "no filters",
			includeSenders: nil,
			excludeSenders: nil,
			from:           "test@example.com",
			want:           true,
		},
		{
			name:           "include match",
			includeSenders: []string{"example.com"},
			excludeSenders: nil,
			from:           "test@example.com",
			want:           true,
		},
		{
			name:           "include no match",
			includeSenders: []string{"other.com"},
			excludeSenders: nil,
			from:           "test@example.com",
			want:           false,
		},
		{
			name:           "exclude match",
			includeSenders: nil,
			excludeSenders: []string{"spam.com"},
			from:           "test@spam.com",
			want:           false,
		},
		{
			name:           "exclude no match",
			includeSenders: nil,
			excludeSenders: []string{"spam.com"},
			from:           "test@example.com",
			want:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			imp := &EmailImporter{
				includeSenders: tt.includeSenders,
				excludeSenders: tt.excludeSenders,
			}
			if got := imp.matchesSender(tt.from); got != tt.want {
				t.Errorf("matchesSender(%q) = %v, want %v", tt.from, got, tt.want)
			}
		})
	}
}
