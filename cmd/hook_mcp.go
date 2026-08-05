package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// mcpServerName is the registration key used for the symmemory MCP server in
// every host config.
const mcpServerName = "symaira-memory"

// mcpClientPreset describes how a host client stores MCP server registrations.
// Every per-client difference lives in this one table, so a format change is
// a single-line edit.
type mcpClientPreset struct {
	Name        string // CLI name used as `symmemory hook <name>`
	Description string
	Format      string // "json" or "toml"
	RootKey     string // JSON root key ("mcpServers" or "mcp"); empty for TOML
	ArrayCmd    bool   // JSON: command as [exec, args...] array vs command+args split
	DefaultPath func() string
}

// hookMCPClients is the single source of truth for the clients the hook
// family can wire up. Supported formats were verified against the project
// docs (docs/agent-integration.md) and the existing mcp-config generator.
var hookMCPClients = []mcpClientPreset{
	{
		Name:        "codex",
		Description: "OpenAI Codex CLI (~/.codex/config.toml, [mcp_servers] section)",
		Format:      "toml",
		DefaultPath: func() string { return homeRelativePath(".codex", "config.toml") },
	},
	{
		Name:        "opencode",
		Description: "OpenCode (project opencode.json, mcp section)",
		Format:      "json",
		RootKey:     "mcp",
		ArrayCmd:    true,
		DefaultPath: func() string { return cwdRelativePath("opencode.json") },
	},
	{
		Name:        "cursor",
		Description: "Cursor (~/.cursor/mcp.json, mcpServers)",
		Format:      "json",
		RootKey:     "mcpServers",
		DefaultPath: func() string { return homeRelativePath(".cursor", "mcp.json") },
	},
	{
		Name:        "claude-desktop",
		Description: "Claude Desktop (claude_desktop_config.json, mcpServers)",
		Format:      "json",
		RootKey:     "mcpServers",
		DefaultPath: func() string {
			return homeRelativePath("Library", "Application Support", "Claude", "claude_desktop_config.json")
		},
	},
	{
		Name:        "vscode",
		Description: "VS Code (project .vscode/mcp.json, mcpServers)",
		Format:      "json",
		RootKey:     "mcpServers",
		DefaultPath: func() string { return cwdRelativePath(".vscode", "mcp.json") },
	},
}

var (
	hookMCPMerge   bool
	hookMCPPath    string
	hookMCPProfile string
	hookList       bool
)

func init() {
	hookCmd.Flags().BoolVar(&hookList, "list", false, "List supported MCP client hooks")

	for i := range hookMCPClients {
		client := &hookMCPClients[i]
		cmd := &cobra.Command{
			Use:   client.Name,
			Short: client.Description,
			Long: fmt.Sprintf(`Prints the MCP server registration that wires symmemory into %s.
The config block is always printed to stdout. With --merge it is also written
into the client's config file idempotently — unrelated keys are never touched.`,
				client.Description),
			RunE: func(c *cobra.Command, args []string) error {
				return runMCPHook(client)
			},
		}
		cmd.Flags().BoolVar(&hookMCPMerge, "merge", false, "Merge the MCP server registration into the client config file (idempotent)")
		cmd.Flags().StringVar(&hookMCPPath, "config-path", "", "Config file to write with --merge (default: the client's standard location)")
		cmd.Flags().StringVar(&hookMCPProfile, "profile", "", "Agent profile name to pass to symmemory serve")
		hookCmd.AddCommand(cmd)
	}
}

// hookMCPClientNames returns the sorted list of supported client names.
func hookMCPClientNames() []string {
	names := make([]string, 0, len(hookMCPClients))
	for i := range hookMCPClients {
		names = append(names, hookMCPClients[i].Name)
	}
	sort.Strings(names)
	return names
}

// printSupportedClients lists every supported client with its config target.
func printSupportedClients() {
	fmt.Println("Supported MCP client hooks:")
	for i := range hookMCPClients {
		p := &hookMCPClients[i]
		fmt.Printf("  %-15s %s\n", p.Name, p.Description)
	}
}

// runMCPHook prints the client's MCP server registration block to stdout and,
// with --merge, writes it into the client's config file idempotently.
func runMCPHook(p *mcpClientPreset) error {
	execPath, err := os.Executable()
	if err != nil {
		execPath = "symmemory"
	}

	// Every client reuses the same server invocation: <exec> serve [--profile <name>].
	serveArgs := []string{"serve"}
	if hookMCPProfile != "" {
		serveArgs = append(serveArgs, "--profile", hookMCPProfile)
	}

	block := buildMCPHookBlock(p, execPath, serveArgs)

	// Always print the config block to stdout
	fmt.Println(block)

	// Optionally merge into the host config file
	if hookMCPMerge {
		path := hookMCPPath
		if path == "" {
			path = p.DefaultPath()
		}
		if err := mergeMCPHook(p, path, execPath, serveArgs); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: merge failed: %v\n", err)
		}
	}

	return nil
}

// buildMCPHookBlock returns the full config document registering the
// symmemory MCP server for the client's config format.
func buildMCPHookBlock(p *mcpClientPreset, execPath string, serveArgs []string) string {
	if p.Format == "toml" {
		return buildCodexToml(execPath, serveArgs)
	}

	config := map[string]interface{}{
		p.RootKey: map[string]interface{}{
			mcpServerName: mcpServerEntry(p, execPath, serveArgs),
		},
	}
	if p.Name == "opencode" {
		config["$schema"] = "https://opencode.ai/config.json"
	}

	b, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		// This should never happen with well-formed config maps
		return fmt.Sprintf(`{"error": %q}`, err.Error())
	}
	return string(b)
}

// mcpServerEntry returns the per-host registration object for the symmemory
// MCP stdio server.
func mcpServerEntry(p *mcpClientPreset, execPath string, serveArgs []string) map[string]interface{} {
	if p.ArrayCmd {
		fullCmd := append([]string{execPath}, serveArgs...)
		return map[string]interface{}{
			"type":    "local",
			"command": fullCmd,
			"enabled": true,
		}
	}
	return map[string]interface{}{
		"command": execPath,
		"args":    serveArgs,
	}
}

// mergeMCPHook idempotently merges the symmemory MCP server registration into
// the host config file at path. Existing keys are never destroyed: JSON hosts
// keep every unrelated key, TOML hosts keep every unrelated line.
func mergeMCPHook(p *mcpClientPreset, path, execPath string, serveArgs []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading existing config: %w", err)
	}

	if p.Format == "toml" {
		return mergeMCPHookTOML(path, data, execPath, serveArgs)
	}
	return mergeMCPHookJSON(p, path, data, execPath, serveArgs)
}

// mergeMCPHookJSON merges the server entry under the client's root key,
// preserving every other key. An identical entry is left untouched.
func mergeMCPHookJSON(p *mcpClientPreset, path string, data []byte, execPath string, serveArgs []string) error {
	var existing map[string]interface{}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &existing); err != nil {
			return fmt.Errorf("parsing existing config: %w", err)
		}
	}
	if existing == nil {
		existing = make(map[string]interface{})
	}

	rootRaw, _ := existing[p.RootKey].(map[string]interface{})
	if rootRaw == nil {
		rootRaw = make(map[string]interface{})
		existing[p.RootKey] = rootRaw
	}

	desired := mcpServerEntry(p, execPath, serveArgs)
	if cur, ok := rootRaw[mcpServerName]; ok && jsonEqual(cur, desired) {
		fmt.Fprintf(os.Stderr, "%s already present in %s — skipping.\n", mcpServerName, path)
		return nil
	}
	rootRaw[mcpServerName] = desired

	out, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	if err := os.WriteFile(path, append(out, '\n'), 0600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	fmt.Fprintf(os.Stderr, "MCP server config merged into %s\n", path)
	return nil
}

// jsonEqual reports whether a and b encode to the same JSON value. Values are
// compared after canonical encoding so that in-memory Go types ([]string) and
// their JSON round-trip forms ([]interface{}) compare equal.
func jsonEqual(a, b interface{}) bool {
	ab, err := json.Marshal(a)
	if err != nil {
		return false
	}
	bb, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return string(ab) == string(bb)
}

// mergeMCPHookTOML appends the [mcp_servers.symaira-memory] block unless it is
// already present, preserving every existing line byte-for-byte.
func mergeMCPHookTOML(path string, data []byte, execPath string, serveArgs []string) error {
	section := "[mcp_servers." + mcpServerName + "]"
	if len(data) > 0 && strings.Contains(string(data), section) {
		fmt.Fprintf(os.Stderr, "%s already present in %s — skipping.\n", mcpServerName, path)
		return nil
	}

	block := buildCodexToml(execPath, serveArgs)
	out := append(data, []byte(block)...)
	if len(out) > 0 && out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	if err := os.WriteFile(path, out, 0600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	fmt.Fprintf(os.Stderr, "MCP server config merged into %s\n", path)
	return nil
}

// homeRelativePath joins path parts under the user's home directory.
func homeRelativePath(parts ...string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(parts...)
	}
	return filepath.Join(append([]string{home}, parts...)...)
}

// cwdRelativePath joins path parts under the current working directory.
func cwdRelativePath(parts ...string) string {
	wd, err := os.Getwd()
	if err != nil {
		return filepath.Join(parts...)
	}
	return filepath.Join(append([]string{wd}, parts...)...)
}
