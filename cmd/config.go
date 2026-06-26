package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/danieljustus/symaira-corekit/exitcodes"
	"github.com/spf13/cobra"
)

var (
	configProfile string
	configTool    string
)

// toolPreset defines the MCP configuration format for a specific AI coding tool.
type toolPreset struct {
	Name        string
	Description string
	ConfigPath  string // human-readable config file path hint
	Format      string // "json" or "toml"
}

var toolPresets = map[string]toolPreset{
	"claude-code": {
		Name:        "claude-code",
		Description: "Claude Code / Claude Desktop (.mcp.json)",
		ConfigPath:  ".mcp.json",
		Format:      "json",
	},
	"opencode": {
		Name:        "opencode",
		Description: "OpenCode (opencode.json)",
		ConfigPath:  "opencode.json",
		Format:      "json",
	},
	"codex": {
		Name:        "codex",
		Description: "OpenAI Codex CLI (~/.codex/config.toml)",
		ConfigPath:  "~/.codex/config.toml",
		Format:      "toml",
	},
	"kimi": {
		Name:        "kimi",
		Description: "Kimi Code CLI (~/.kimi-code/mcp.json)",
		ConfigPath:  "~/.kimi-code/mcp.json",
		Format:      "json",
	},
	"copilot": {
		Name:        "copilot",
		Description: "GitHub Copilot CLI (~/.copilot/mcp-config.json)",
		ConfigPath:  "~/.copilot/mcp-config.json",
		Format:      "json",
	},
}

// toolNames returns a sorted list of valid tool preset names for error messages.
func toolNames() []string {
	names := make([]string, 0, len(toolPresets))
	for k := range toolPresets {
		names = append(names, k)
	}
	return names
}

func init() {
	configCmd.Flags().StringVar(&configProfile, "profile", "", "Agent profile name to include in generated config (e.g., claude-code, opencode)")
	configCmd.Flags().StringVar(&configTool, "tool", "", "Target tool preset: claude-code, opencode, codex, kimi, copilot (default: claude-code)")
	rootCmd.AddCommand(configCmd)
}

var configCmd = &cobra.Command{
	Use:   "mcp-config",
	Short: "Print ready-to-use Model Context Protocol (MCP) configuration template",
	Long: `Prints configuration blocks for integrating the symmemory stdio server into
major MCP clients. Use --tool to generate tool-specific config formats.

Supported tools: claude-code (default), opencode, codex, kimi, copilot.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Default to claude-code when no --tool flag provided
		tool := configTool
		if tool == "" {
			tool = "claude-code"
		}

		preset, ok := toolPresets[tool]
		if !ok {
			return exitcodes.Wrapf(nil, exitcodes.ExitNoInput, exitcodes.KindValidation, "unknown tool %q; valid tools: %s", tool, strings.Join(toolNames(), ", "))
		}

		// Find the path to the current executable or assume symmemory standard path
		execPath, err := os.Executable()
		if err != nil {
			execPath = "symmemory" // fallback
		}

		// Build args: ["serve"] or ["serve", "--profile", "<name>"]
		serveArgs := []string{"serve"}
		if configProfile != "" {
			serveArgs = append(serveArgs, "--profile", configProfile)
		}

		var output string
		switch preset.Format {
		case "toml":
			output = buildCodexToml(execPath, serveArgs)
		default:
			output = buildJSONConfig(tool, execPath, serveArgs)
		}

		// Print guidance to stderr (never stdout — zero stdio pollution)
		home, _ := os.UserHomeDir()

		fmt.Fprintf(os.Stderr, "⚡ Tool: %s\n", preset.Description)
		fmt.Fprintf(os.Stderr, "📂 Config file: %s\n", preset.ConfigPath)
		if configProfile != "" {
			fmt.Fprintf(os.Stderr, "\n🔐 Active profile: %s\n", configProfile)
		}

		switch tool {
		case "claude-code":
			fmt.Fprintf(os.Stderr, "\n💡 Claude Desktop path:\n  %s\n",
				filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json"))
			fmt.Fprintf(os.Stderr, "\n🛠️  Cursor settings:\n  Cursor -> Settings -> Features -> MCP -> Click '+ Add New MCP Server'\n  Name: symaira-memory\n  Type: stdio\n  Command: %s serve\n", execPath)
		case "opencode":
			fmt.Fprintf(os.Stderr, "\n💡 Place in your project root as opencode.json or merge into existing config.\n")
		case "codex":
			fmt.Fprintf(os.Stderr, "\n💡 Append this block to ~/.codex/config.toml (or project .codex/config.toml).\n")
		case "kimi":
			fmt.Fprintf(os.Stderr, "\n💡 Place at ~/.kimi-code/mcp.json (user-level) or .kimi-code/mcp.json (project-level).\n")
		case "copilot":
			fmt.Fprintf(os.Stderr, "\n💡 Place at ~/.copilot/mcp-config.json.\n")
		}

		fmt.Fprintln(os.Stderr, "\n========================= CONFIGURATION BLOCK =========================")
		fmt.Fprintln(os.Stderr, output)
		fmt.Fprintln(os.Stderr, "=======================================================================")
		return nil
	},
}

// buildJSONConfig generates JSON config for tools that use the mcpServers or mcp root key.
func buildJSONConfig(tool, execPath string, serveArgs []string) string {
	var config interface{}

	switch tool {
	case "opencode":
		// OpenCode uses "mcp" root key, command as array, type: "local"
		fullCmd := append([]string{execPath}, serveArgs...)
		config = map[string]interface{}{
			"$schema": "https://opencode.ai/config.json",
			"mcp": map[string]interface{}{
				"symaira-memory": map[string]interface{}{
					"type":    "local",
					"command": fullCmd,
					"enabled": true,
				},
			},
		}

	case "copilot":
		// Copilot uses mcpServers, requires type and tools fields
		config = map[string]interface{}{
			"mcpServers": map[string]interface{}{
				"symaira-memory": map[string]interface{}{
					"type":    "local",
					"command": execPath,
					"args":    serveArgs,
					"env":     map[string]interface{}{},
					"tools":   []string{"*"},
				},
			},
		}

	default:
		// claude-code and kimi: standard mcpServers with command + args
		config = map[string]interface{}{
			"mcpServers": map[string]interface{}{
				"symaira-memory": map[string]interface{}{
					"command": execPath,
					"args":    serveArgs,
				},
			},
		}
	}

	b, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		// This should never happen with well-formed config maps
		return fmt.Sprintf(`{"error": %q}`, err.Error())
	}
	return string(b)
}

// buildCodexToml generates TOML config for OpenAI Codex CLI.
func buildCodexToml(execPath string, serveArgs []string) string {
	var sb strings.Builder
	sb.WriteString("[mcp_servers.symaira-memory]\n")
	fmt.Fprintf(&sb, "command = %q\n", execPath)
	if len(serveArgs) > 0 {
		args := make([]string, len(serveArgs))
		for i, a := range serveArgs {
			args[i] = fmt.Sprintf("%q", a)
		}
		fmt.Fprintf(&sb, "args = [%s]\n", strings.Join(args, ", "))
	}
	return sb.String()
}
