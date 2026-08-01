package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/discovery"
	"github.com/danieljustus/symaira-memory/internal/importer"
	"github.com/danieljustus/symaira-memory/internal/importer/aider"
	"github.com/danieljustus/symaira-memory/internal/importer/claudecode"
	"github.com/danieljustus/symaira-memory/internal/importer/codex"
	"github.com/danieljustus/symaira-memory/internal/importer/curatedmemory"
	"github.com/danieljustus/symaira-memory/internal/importer/git"
	"github.com/danieljustus/symaira-memory/internal/importer/github"
	"github.com/danieljustus/symaira-memory/internal/importer/hermes"
	"github.com/danieljustus/symaira-memory/internal/importer/obsidian"
	"github.com/danieljustus/symaira-memory/internal/importer/opencode"
	"github.com/danieljustus/symaira-memory/internal/importer/paperless"
	"github.com/spf13/cobra"
)

var discoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Discover configured and local import sources",
	RunE:  runDiscoverSources,
}

var discoverSourcesCmd = &cobra.Command{
	Use:   "sources",
	Short: "Discover source metadata as a versioned JSON envelope",
	RunE:  runDiscoverSources,
}

func init() {
	discoverCmd.AddCommand(discoverSourcesCmd)
	rootCmd.AddCommand(discoverCmd)
}

func runDiscoverSources(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	providers, err := buildDiscoveryProviders(cfg)
	if err != nil {
		return err
	}

	response := discovery.Scan(providers, time.Now())
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(response)
}

func buildDiscoveryProviders(cfg *config.Config) ([]discovery.Provider, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve home directory: %w", err)
	}

	providers := []discovery.Provider{
		localProvider(
			"claude-code", "session-data", "Claude Code Sessions",
			configuredOrDefaultPath(cfg, "claude-code", filepath.Join(home, ".claude", "projects")),
			claudecode.NewClaudeCodeImporter(configuredPath(cfg, "claude-code")),
		),
		localProvider(
			"codex", "session-data", "Codex Sessions",
			configuredOrDefaultPath(cfg, "codex", filepath.Join(home, ".codex")),
			codex.NewCodexImporter(configuredPath(cfg, "codex")),
		),
		localProvider(
			"hermes", "session-data", "Hermes Sessions",
			configuredOrDefaultPath(cfg, "hermes", filepath.Join(home, ".hermes", "state.db")),
			hermes.NewHermesImporter(configuredPath(cfg, "hermes")),
		),
		localProvider(
			"aider", "session-data", "Aider Chat Histories",
			configuredOrDefaultPath(cfg, "aider", home),
			aider.NewAiderImporter(configuredPaths(cfg, "aider")),
		),
		localProvider(
			"opencode", "session-data", "OpenCode Sessions",
			configuredOrDefaultPath(cfg, "opencode", filepath.Join(home, ".local", "share", "opencode", "opencode.db")),
			opencode.NewOpenCodeImporter(configuredPath(cfg, "opencode")),
		),
	}

	curated := curatedmemory.NewCuratedMemoryImporter("")
	providers = append(providers,
		filteredProvider(
			"curated-memory:claude-code", "curated-memory", "Claude Code Curated Memory",
			filepath.Join(home, ".claude", "projects"), "may_contain_personal_data", curated,
			"claude-code",
		),
		filteredProvider(
			"curated-memory:hermes", "curated-memory", "Hermes Curated Memory",
			filepath.Join(home, ".hermes", "memories"), "sensitive", curated,
			"hermes",
		),
	)

	// Explicitly configured local data sources are opt-in. This avoids scanning
	// arbitrary repositories or vaults during a background Hub refresh.
	if path := configuredPath(cfg, "obsidian"); path != "" {
		providers = append(providers, localProvider(
			"obsidian", "document-repository", "Obsidian Vault", path,
			obsidian.NewObsidianImporter(path, "", nil, nil, nil),
		))
	}
	if path := configuredPath(cfg, "git"); path != "" {
		providers = append(providers, localProvider(
			"git", "repository", "Git Repository", path,
			// Git is only scanned when the user explicitly configured a path.
			// The author filter is optional and remains metadata-only.
			gitImporter(cfg),
		))
	}

	// Remote sources are strictly opt-in. Credentials are passed only to the
	// importer and never appear in the discovery response.
	if tool, ok := configuredTool(cfg, "paperless"); ok && tool.Path != "" && tool.Token != "" {
		providers = append(providers, localProvider(
			"paperless", "document-repository", "Paperless Documents", tool.Path,
			paperless.NewPaperlessImporter(
				tool.Path,
				tool.Token,
				tool.Options["tag"],
				tool.Options["correspondent"],
				intOption(tool.Options, "max_content"),
			),
		))
	}
	if tool, ok := configuredTool(cfg, "github"); ok && tool.Path != "" && tool.Token != "" {
		parts := strings.SplitN(tool.Path, "/", 2)
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			providers = append(providers, localProvider(
				"github", "repository", "GitHub Repository", "https://github.com/"+tool.Path,
				github.NewGitHubImporter(parts[0], parts[1], tool.Token),
			))
		}
	}

	return providers, nil
}

func localProvider(id, kind, displayName, location string, sessionImporter importer.SessionImporter) discovery.Provider {
	return discovery.Provider{
		ID:          id,
		Tool:        "symmemory",
		Kind:        kind,
		DisplayName: displayName,
		Location:    displayLocation(location),
		PrivacyHint: privacyHint(sessionImporter),
		Discover: func() ([]importer.SessionRef, error) {
			return sessionImporter.DiscoverSessions(time.Time{})
		},
	}
}

func filteredProvider(id, kind, displayName, location, privacy string, sessionImporter importer.SessionImporter, sourceTool string) discovery.Provider {
	provider := localProvider(id, kind, displayName, location, sessionImporter)
	provider.PrivacyHint = privacy
	provider.Discover = func() ([]importer.SessionRef, error) {
		refs, err := sessionImporter.DiscoverSessions(time.Time{})
		if err != nil {
			return nil, err
		}
		filtered := make([]importer.SessionRef, 0, len(refs))
		for _, ref := range refs {
			if ref.Metadata["source_tool"] == sourceTool {
				filtered = append(filtered, ref)
			}
		}
		return filtered, nil
	}
	return provider
}

func configuredTool(cfg *config.Config, name string) (config.ImportToolConfig, bool) {
	if cfg == nil || cfg.Import.Tools == nil {
		return config.ImportToolConfig{}, false
	}
	tool, ok := cfg.Import.Tools[name]
	return tool, ok
}

func configuredPath(cfg *config.Config, name string) string {
	tool, ok := configuredTool(cfg, name)
	if !ok {
		return ""
	}
	return tool.Path
}

func configuredPaths(cfg *config.Config, name string) []string {
	path := configuredPath(cfg, name)
	if path == "" {
		return nil
	}
	return []string{path}
}

func configuredOrDefaultPath(cfg *config.Config, name, fallback string) string {
	if path := configuredPath(cfg, name); path != "" {
		return path
	}
	return fallback
}

func displayLocation(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || path == "" {
		return path
	}
	cleanPath, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	relative, err := filepath.Rel(home, cleanPath)
	if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "~/" + filepath.ToSlash(relative)
	}
	return filepath.ToSlash(path)
}

func privacyHint(sessionImporter importer.SessionImporter) string {
	if aware, ok := sessionImporter.(importer.PrivacyAware); ok {
		switch aware.PrivacyLevel() {
		case importer.PrivacyPublic:
			return "none"
		case importer.PrivacySecret:
			return "contains_credentials"
		default:
			return "may_contain_personal_data"
		}
	}
	return "may_contain_personal_data"
}

func intOption(options map[string]string, key string) int {
	value, _ := strconv.Atoi(options[key])
	return value
}

func gitImporter(cfg *config.Config) importer.SessionImporter {
	tool, _ := configuredTool(cfg, "git")
	return git.NewGitImporter(tool.Path, tool.Options["author"])
}
