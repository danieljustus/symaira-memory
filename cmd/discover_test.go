package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/discovery"
	"github.com/danieljustus/symaira-memory/internal/importer"
)

// stubImporter is a minimal SessionImporter that returns canned discovery
// results. It does NOT implement PrivacyAware.
type stubImporter struct {
	name string
	refs []importer.SessionRef
	err  error
}

func (s *stubImporter) Name() string { return s.name }

func (s *stubImporter) DiscoverSessions(time.Time) ([]importer.SessionRef, error) {
	return s.refs, s.err
}

func (s *stubImporter) ImportSession(importer.SessionRef) ([]importer.ImportedFact, error) {
	return nil, nil
}

// awareStubImporter additionally implements PrivacyAware with a fixed level.
type awareStubImporter struct {
	stubImporter
	level importer.PrivacyLevel
}

func (s *awareStubImporter) PrivacyLevel() importer.PrivacyLevel { return s.level }

func (s *awareStubImporter) RequiresPIIGuard() bool { return false }

// --------------------------------------------------------------------------
// runDiscoverSources end-to-end
// --------------------------------------------------------------------------

func TestDiscoverSourcesJSONEnvelope(t *testing.T) {
	tempDir := t.TempDir()
	setTestHome(t, tempDir)

	// Real fixtures discoverable by the default providers: a Claude Code
	// session transcript and a Hermes curated memory file.
	claudeDir := filepath.Join(tempDir, ".claude", "projects", "demo")
	if err := os.MkdirAll(claudeDir, 0700); err != nil {
		t.Fatalf("failed to create claude fixture dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(claudeDir, "mem1.jsonl"), []byte("{}\n"), 0600); err != nil {
		t.Fatalf("failed to write claude fixture: %v", err)
	}
	hermesDir := filepath.Join(tempDir, ".hermes", "memories")
	if err := os.MkdirAll(hermesDir, 0700); err != nil {
		t.Fatalf("failed to create hermes fixture dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(hermesDir, "MEMORY.md"), []byte("# Memory\n"), 0600); err != nil {
		t.Fatalf("failed to write hermes fixture: %v", err)
	}

	output := captureCmdOutput(func() {
		if err := runDiscoverSources(discoverSourcesCmd, nil); err != nil {
			t.Errorf("runDiscoverSources failed: %v", err)
		}
	})

	if !json.Valid([]byte(output)) {
		t.Fatalf("expected valid JSON output, got: %s", output)
	}

	var resp discovery.Response
	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.SchemaVersion != discovery.SchemaVersion {
		t.Errorf("schema_version = %d, want %d", resp.SchemaVersion, discovery.SchemaVersion)
	}
	if len(resp.Sources) != 2 {
		t.Fatalf("expected 2 sources, got %d: %+v", len(resp.Sources), resp.Sources)
	}

	var claude, curated *discovery.Source
	for i := range resp.Sources {
		switch resp.Sources[i].SourceID {
		case "symmemory:claude-code":
			claude = &resp.Sources[i]
		case "symmemory:curated-memory:hermes":
			curated = &resp.Sources[i]
		}
	}
	if claude == nil {
		t.Fatal("expected symmemory:claude-code source")
	}
	if curated == nil {
		t.Fatal("expected symmemory:curated-memory:hermes source")
	}

	if claude.Tool != "symmemory" || claude.Kind != "session-data" {
		t.Errorf("claude tool/kind = %q/%q, want symmemory/session-data", claude.Tool, claude.Kind)
	}
	if claude.DisplayName != "Claude Code Sessions" {
		t.Errorf("claude display_name = %q, want %q", claude.DisplayName, "Claude Code Sessions")
	}
	if claude.Location != "~/.claude/projects" {
		t.Errorf("claude location = %q, want %q", claude.Location, "~/.claude/projects")
	}
	if claude.ItemCount != 1 {
		t.Errorf("claude item_count = %d, want 1", claude.ItemCount)
	}
	if claude.PrivacyHint != "may_contain_personal_data" {
		t.Errorf("claude privacy_hint = %q, want %q", claude.PrivacyHint, "may_contain_personal_data")
	}
	if len(claude.Capabilities) != 1 || claude.Capabilities[0] != "import" {
		t.Errorf("claude capabilities = %v, want [import]", claude.Capabilities)
	}
	if _, err := time.Parse(time.RFC3339, claude.LastSeen); err != nil {
		t.Errorf("claude last_seen %q is not RFC3339: %v", claude.LastSeen, err)
	}

	if curated.Tool != "symmemory" || curated.Kind != "curated-memory" {
		t.Errorf("curated tool/kind = %q/%q, want symmemory/curated-memory", curated.Tool, curated.Kind)
	}
	if curated.Location != "~/.hermes/memories" {
		t.Errorf("curated location = %q, want %q", curated.Location, "~/.hermes/memories")
	}
	if curated.PrivacyHint != "sensitive" {
		t.Errorf("curated privacy_hint = %q, want %q", curated.PrivacyHint, "sensitive")
	}
	if curated.ItemCount != 1 {
		t.Errorf("curated item_count = %d, want 1", curated.ItemCount)
	}
}

func TestDiscoverSourcesEmpty(t *testing.T) {
	tempDir := t.TempDir()
	setTestHome(t, tempDir)

	output := captureCmdOutput(func() {
		if err := runDiscoverSources(discoverSourcesCmd, nil); err != nil {
			t.Errorf("runDiscoverSources failed: %v", err)
		}
	})

	var resp discovery.Response
	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.SchemaVersion != 1 {
		t.Errorf("schema_version = %d, want 1", resp.SchemaVersion)
	}
	if len(resp.Sources) != 0 {
		t.Errorf("expected no sources on empty home, got %d: %+v", len(resp.Sources), resp.Sources)
	}
	if !strings.Contains(output, `"sources": []`) {
		t.Errorf("expected empty sources array in output, got: %s", output)
	}
}

// --------------------------------------------------------------------------
// buildDiscoveryProviders
// --------------------------------------------------------------------------

func toolConfig(path, token string, options map[string]string) config.ImportToolConfig {
	return config.ImportToolConfig{Path: path, Token: token, Options: options}
}

func TestDiscoverBuildProvidersConfiguredTools(t *testing.T) {
	// Redirect HOME so displayLocation resolves absolute/URL locations
	// deterministically instead of collapsing them relative to the real home.
	setTestHome(t, t.TempDir())

	cfg := &config.Config{
		Import: config.ImportConfig{
			Tools: map[string]config.ImportToolConfig{
				"obsidian":  toolConfig("/vault/notes", "", nil),
				"git":       toolConfig("/repo", "", map[string]string{"author": "alice"}),
				"paperless": toolConfig("http://paperless.local:8000", "tok-123", map[string]string{"tag": "mem", "correspondent": "corr", "max_content": "500"}),
				"github":    toolConfig("octo/repo", "placeholder-token-not-a-real-ghp", nil),
			},
		},
	}

	providers, err := buildDiscoveryProviders(cfg)
	if err != nil {
		t.Fatalf("buildDiscoveryProviders failed: %v", err)
	}
	if len(providers) != 11 {
		t.Fatalf("expected 11 providers, got %d", len(providers))
	}

	byID := make(map[string]discovery.Provider, len(providers))
	for _, p := range providers {
		byID[p.ID] = p
	}

	obsidian, ok := byID["obsidian"]
	if !ok {
		t.Fatal("expected configured obsidian provider")
	}
	if obsidian.Kind != "document-repository" || obsidian.Location != "/vault/notes" {
		t.Errorf("obsidian kind/location = %q/%q, want document-repository//vault/notes", obsidian.Kind, obsidian.Location)
	}

	gitProv, ok := byID["git"]
	if !ok {
		t.Fatal("expected configured git provider")
	}
	if gitProv.Kind != "repository" || gitProv.Location != "/repo" {
		t.Errorf("git kind/location = %q/%q, want repository//repo", gitProv.Kind, gitProv.Location)
	}

	paperless, ok := byID["paperless"]
	if !ok {
		t.Fatal("expected configured paperless provider")
	}
	if paperless.DisplayName != "Paperless Documents" {
		t.Errorf("paperless display_name = %q, want %q", paperless.DisplayName, "Paperless Documents")
	}

	github, ok := byID["github"]
	if !ok {
		t.Fatal("expected configured github provider")
	}
	if github.Location != "https://github.com/octo/repo" {
		t.Errorf("github location = %q, want %q", github.Location, "https://github.com/octo/repo")
	}
	if github.Kind != "repository" {
		t.Errorf("github kind = %q, want repository", github.Kind)
	}

	// Default providers are always present, configured or not.
	for _, id := range []string{"claude-code", "codex", "hermes", "aider", "opencode", "curated-memory:claude-code", "curated-memory:hermes"} {
		if _, ok := byID[id]; !ok {
			t.Errorf("expected default provider %q", id)
		}
	}
}

func TestDiscoverBuildProvidersSkipsUnconfigured(t *testing.T) {
	cfg := &config.Config{
		Import: config.ImportConfig{
			Tools: map[string]config.ImportToolConfig{
				// github path without owner/repo split → skipped
				"github": toolConfig("no-slash", "tok", nil),
				// paperless without token → skipped
				"paperless": toolConfig("http://paperless.local:8000", "", nil),
			},
		},
	}

	providers, err := buildDiscoveryProviders(cfg)
	if err != nil {
		t.Fatalf("buildDiscoveryProviders failed: %v", err)
	}

	byID := make(map[string]bool, len(providers))
	for _, p := range providers {
		byID[p.ID] = true
	}
	if byID["github"] {
		t.Error("expected github provider to be skipped for invalid path")
	}
	if byID["paperless"] {
		t.Error("expected paperless provider to be skipped without token")
	}
	if byID["obsidian"] || byID["git"] {
		t.Error("expected obsidian/git providers to be skipped when unconfigured")
	}
	if len(providers) != 7 {
		t.Errorf("expected only the 7 default providers, got %d", len(providers))
	}
}

func TestDiscoverBuildProvidersNilConfig(t *testing.T) {
	providers, err := buildDiscoveryProviders(nil)
	if err != nil {
		t.Fatalf("buildDiscoveryProviders(nil) failed: %v", err)
	}
	if len(providers) != 7 {
		t.Errorf("expected 7 default providers for nil config, got %d", len(providers))
	}
}

// --------------------------------------------------------------------------
// localProvider / filteredProvider
// --------------------------------------------------------------------------

func TestDiscoverLocalProvider(t *testing.T) {
	refs := []importer.SessionRef{{Tool: "stub", SessionID: "s1", Path: "/x"}}
	stub := &awareStubImporter{stubImporter: stubImporter{name: "stub", refs: refs}, level: importer.PrivacyInternal}

	provider := localProvider("t1", "session-data", "Test One", "/some/path", stub)
	if provider.ID != "t1" || provider.Tool != "symmemory" || provider.Kind != "session-data" || provider.DisplayName != "Test One" {
		t.Errorf("provider metadata = %+v", provider)
	}
	if provider.Location != "/some/path" {
		t.Errorf("location = %q, want /some/path", provider.Location)
	}
	if provider.PrivacyHint != "may_contain_personal_data" {
		t.Errorf("privacy_hint = %q, want may_contain_personal_data", provider.PrivacyHint)
	}

	got, err := provider.Discover()
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}
	if len(got) != 1 || got[0].SessionID != "s1" {
		t.Errorf("Discover returned %+v, want [s1]", got)
	}
}

func TestDiscoverFilteredProvider(t *testing.T) {
	refs := []importer.SessionRef{
		{Tool: "curated-memory", SessionID: "hermes:MEMORY.md", Metadata: map[string]string{"source_tool": "hermes"}},
		{Tool: "curated-memory", SessionID: "claude-code:x", Metadata: map[string]string{"source_tool": "claude-code"}},
		{Tool: "curated-memory", SessionID: "none", Metadata: map[string]string{}},
	}
	stub := &stubImporter{name: "curated-memory", refs: refs}

	provider := filteredProvider("cm", "curated-memory", "CM", "/x", "sensitive", stub, "hermes")
	if provider.PrivacyHint != "sensitive" {
		t.Errorf("privacy_hint = %q, want sensitive", provider.PrivacyHint)
	}

	got, err := provider.Discover()
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}
	if len(got) != 1 || got[0].SessionID != "hermes:MEMORY.md" {
		t.Errorf("Discover filtered to %+v, want only hermes ref", got)
	}

	// Error propagation from the underlying importer.
	stub.err = errors.New("boom")
	if _, err := provider.Discover(); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("expected underlying error to propagate, got %v", err)
	}
}

// --------------------------------------------------------------------------
// configuredTool / configuredPath / configuredPaths / configuredOrDefaultPath
// --------------------------------------------------------------------------

func TestDiscoverConfiguredTool(t *testing.T) {
	if _, ok := configuredTool(nil, "git"); ok {
		t.Error("expected nil config to report not configured")
	}
	if _, ok := configuredTool(&config.Config{}, "git"); ok {
		t.Error("expected empty import config to report not configured")
	}
	cfg := &config.Config{Import: config.ImportConfig{Tools: map[string]config.ImportToolConfig{"git": {Path: "/repo"}}}}
	if _, ok := configuredTool(cfg, "missing"); ok {
		t.Error("expected missing tool to report not configured")
	}
	tool, ok := configuredTool(cfg, "git")
	if !ok || tool.Path != "/repo" {
		t.Errorf("configuredTool = %+v, %v; want /repo, true", tool, ok)
	}
}

func TestDiscoverConfiguredPath(t *testing.T) {
	if got := configuredPath(&config.Config{}, "git"); got != "" {
		t.Errorf("configuredPath(empty) = %q, want empty", got)
	}
	cfg := &config.Config{Import: config.ImportConfig{Tools: map[string]config.ImportToolConfig{"git": {Path: "/repo"}}}}
	if got := configuredPath(cfg, "git"); got != "/repo" {
		t.Errorf("configuredPath = %q, want /repo", got)
	}
}

func TestDiscoverConfiguredPaths(t *testing.T) {
	if got := configuredPaths(&config.Config{}, "git"); got != nil {
		t.Errorf("configuredPaths(empty) = %v, want nil", got)
	}
	cfg := &config.Config{Import: config.ImportConfig{Tools: map[string]config.ImportToolConfig{"git": {Path: "/repo"}}}}
	got := configuredPaths(cfg, "git")
	if len(got) != 1 || got[0] != "/repo" {
		t.Errorf("configuredPaths = %v, want [/repo]", got)
	}
}

func TestDiscoverConfiguredOrDefaultPath(t *testing.T) {
	cfg := &config.Config{Import: config.ImportConfig{Tools: map[string]config.ImportToolConfig{"git": {Path: "/repo"}}}}
	if got := configuredOrDefaultPath(cfg, "git", "/fallback"); got != "/repo" {
		t.Errorf("configuredOrDefaultPath = %q, want /repo", got)
	}
	if got := configuredOrDefaultPath(cfg, "missing", "/fallback"); got != "/fallback" {
		t.Errorf("configuredOrDefaultPath = %q, want /fallback", got)
	}
}

// --------------------------------------------------------------------------
// displayLocation / privacyHint / intOption / gitImporter
// --------------------------------------------------------------------------

func TestDiscoverDisplayLocation(t *testing.T) {
	if got := displayLocation(""); got != "" {
		t.Errorf("displayLocation(\"\") = %q, want empty", got)
	}
	if got := displayLocation("/etc/symmemory"); got != "/etc/symmemory" {
		t.Errorf("displayLocation outside home = %q, want /etc/symmemory", got)
	}

	tempDir := t.TempDir()
	setTestHome(t, tempDir)
	if got := displayLocation(filepath.Join(tempDir, ".claude", "projects")); got != "~/.claude/projects" {
		t.Errorf("displayLocation inside home = %q, want ~/.claude/projects", got)
	}
}

func TestDiscoverPrivacyHint(t *testing.T) {
	cases := []struct {
		name string
		imp  importer.SessionImporter
		want string
	}{
		{name: "public", imp: &awareStubImporter{level: importer.PrivacyPublic}, want: "none"},
		{name: "secret", imp: &awareStubImporter{level: importer.PrivacySecret}, want: "contains_credentials"},
		{name: "internal", imp: &awareStubImporter{level: importer.PrivacyInternal}, want: "may_contain_personal_data"},
		{name: "confidential", imp: &awareStubImporter{level: importer.PrivacyConfidential}, want: "may_contain_personal_data"},
		{name: "not-aware", imp: &stubImporter{name: "plain"}, want: "may_contain_personal_data"},
	}
	for _, tc := range cases {
		if got := privacyHint(tc.imp); got != tc.want {
			t.Errorf("privacyHint(%s) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestDiscoverIntOption(t *testing.T) {
	if got := intOption(map[string]string{"max_content": "500"}, "max_content"); got != 500 {
		t.Errorf("intOption = %d, want 500", got)
	}
	if got := intOption(map[string]string{"max_content": "not-a-number"}, "max_content"); got != 0 {
		t.Errorf("intOption(invalid) = %d, want 0", got)
	}
	if got := intOption(map[string]string{}, "missing"); got != 0 {
		t.Errorf("intOption(missing) = %d, want 0", got)
	}
}

func TestDiscoverGitImporter(t *testing.T) {
	cfg := &config.Config{Import: config.ImportConfig{Tools: map[string]config.ImportToolConfig{"git": {Path: "/repo", Options: map[string]string{"author": "alice"}}}}}
	imp := gitImporter(cfg)
	if imp.Name() != "git" {
		t.Errorf("gitImporter().Name() = %q, want git", imp.Name())
	}
	// Git commits are public metadata.
	if got := privacyHint(imp); got != "none" {
		t.Errorf("privacyHint(git importer) = %q, want none", got)
	}

	// Unconfigured git still constructs an importer (empty path/author).
	imp = gitImporter(&config.Config{})
	if imp.Name() != "git" {
		t.Errorf("gitImporter(empty).Name() = %q, want git", imp.Name())
	}
}
