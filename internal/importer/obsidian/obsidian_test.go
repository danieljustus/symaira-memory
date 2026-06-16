package obsidian

import (
	"testing"
)

func TestNewObsidianImporter(t *testing.T) {
	imp := NewObsidianImporter("/tmp/test", "", nil, nil, nil)

	if imp.Name() != "obsidian" {
		t.Errorf("Name() = %q, want %q", imp.Name(), "obsidian")
	}

	if imp.Category() != "notes" {
		t.Errorf("Category() = %q, want %q", imp.Category(), "notes")
	}

	if imp.PrivacyLevel() != "internal" {
		t.Errorf("PrivacyLevel() = %q, want %q", imp.PrivacyLevel(), "internal")
	}

	if imp.RequiresPIIGuard() {
		t.Error("RequiresPIIGuard() = true, want false")
	}
}

func TestExtractTags(t *testing.T) {
	imp := &ObsidianImporter{}

	content := `# Meeting Notes

This is about #project-x and #meeting.

Tags: #decision #action-item
`

	tags := imp.extractTags(content)
	if len(tags) != 4 {
		t.Errorf("extractTags() returned %d tags, want 4", len(tags))
	}

	expected := []string{"project-x", "meeting", "decision", "action-item"}
	for i, tag := range tags {
		if tag != expected[i] {
			t.Errorf("tags[%d] = %q, want %q", i, tag, expected[i])
		}
	}
}

func TestExtractWikilinks(t *testing.T) {
	imp := &ObsidianImporter{}

	content := `See [[Project Setup]] and [[Meeting Notes|meeting notes]].
Also check [[Architecture]] for details.
`

	links := imp.extractWikilinks(content)
	if len(links) != 3 {
		t.Errorf("extractWikilinks() returned %d links, want 3", len(links))
	}

	expected := []string{"Project Setup", "Meeting Notes", "Architecture"}
	for i, link := range links {
		if link != expected[i] {
			t.Errorf("links[%d] = %q, want %q", i, link, expected[i])
		}
	}
}

func TestExtractTitle(t *testing.T) {
	imp := &ObsidianImporter{}

	tests := []struct {
		name    string
		content string
		path    string
		want    string
	}{
		{
			name:    "with H1",
			content: "# My Meeting Notes\n\nSome content here.",
			path:    "/tmp/other.md",
			want:    "My Meeting Notes",
		},
		{
			name:    "without H1",
			content: "No heading here.",
			path:    "/tmp/Meeting Notes.md",
			want:    "Meeting Notes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := imp.extractTitle(tt.content, tt.path)
			if got != tt.want {
				t.Errorf("extractTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsExcludedFolder(t *testing.T) {
	imp := &ObsidianImporter{}

	tests := []struct {
		path string
		want bool
	}{
		{"Projects/MyProject.md", false},
		{".obsidian/config", true},
		{".trash/note.md", true},
		{"Templates/daily.md", true},
		{"Projects/.obsidian/config", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := imp.isExcludedFolder(tt.path); got != tt.want {
				t.Errorf("isExcludedFolder(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}
