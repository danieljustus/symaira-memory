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
	hookMerge        bool
	hookSettingsPath string
)

// hookCmd is the parent command for hook-related operations.
var hookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Generate agent integration hooks",
	Long: `Generate and optionally install agent hook configurations for tools like
Claude Code, and one-command MCP server wiring for MCP host clients (Codex,
OpenCode, Cursor, Claude Desktop, VS Code). The generated block is always
printed to stdout; use --merge to write it into the client's config file
idempotently. Run 'symmemory hook --list' to see supported clients.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if hookList {
			printSupportedClients()
			return nil
		}
		if len(args) > 0 {
			return exitcodes.Wrapf(nil, exitcodes.ExitNoInput, exitcodes.KindValidation,
				"unknown client %q; supported clients: %s", args[0], strings.Join(hookMCPClientNames(), ", "))
		}
		return cmd.Help()
	},
}

// hookClaudeCodeCmd generates a Claude Code SessionStart hook.
var hookClaudeCodeCmd = &cobra.Command{
	Use:   "claude-code",
	Short: "Print a Claude Code SessionStart hook JSON block",
	Long: `Prints a SessionStart hook configuration for Claude Code that invokes
symmemory context on every session start. The hook block is always printed
to stdout. With --merge it is also written into ~/.claude/settings.json
idempotently.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		hookBlock := buildClaudeHookBlock()

		// Always print the hook JSON to stdout
		b, err := json.MarshalIndent(hookBlock, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to encode hook JSON: %v\n", err)
			return nil // fail-safe: never exit non-zero
		}
		fmt.Println(string(b))

		// Optionally merge into settings file
		if hookMerge {
			if err := mergeClaudeHook(hookSettingsPath, hookBlock); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: merge failed: %v\n", err)
			}
		}

		return nil
	},
}

// buildClaudeHookBlock returns the structured hook JSON block for Claude Code.
// It includes all four lifecycle hooks: SessionStart, PostToolUseFailure,
// SessionEnd, and PreCompact. Each invokes symmemory via the relevant
// subcommand — context (read) or observe (write-path).
// Hook scripts never exit non-zero and never write to stdout.
func buildClaudeHookBlock() map[string]interface{} {
	return map[string]interface{}{
		"hooks": map[string]interface{}{
			"SessionStart": []interface{}{
				map[string]interface{}{
					"type":    "command",
					"command": "symmemory context --output md",
				},
			},
			"PostToolUseFailure": []interface{}{
				map[string]interface{}{
					"type":    "command",
					"command": "symmemory observe tool-failure",
				},
			},
			"SessionEnd": []interface{}{
				map[string]interface{}{
					"type":    "command",
					"command": "symmemory observe session-end",
				},
			},
			"PreCompact": []interface{}{
				map[string]interface{}{
					"type":    "command",
					"command": "symmemory observe pre-compact",
				},
			},
		},
	}
}

// mergeClaudeHook idempotently merges the symmemory lifecycle hooks into
// the settings file at settingsPath. It creates the file if missing, parses
// existing JSON, iterates over each hook type in the block, checks for the
// command by exact match, appends if absent, and writes back.
func mergeClaudeHook(settingsPath string, hookBlock map[string]interface{}) error {
	// Ensure parent directory exists
	dir := filepath.Dir(settingsPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating settings directory: %w", err)
	}

	// Read existing file or start with empty object
	var existing map[string]interface{}
	data, err := os.ReadFile(settingsPath)
	if err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, &existing); err != nil {
			return fmt.Errorf("parsing existing settings: %w", err)
		}
	}
	if existing == nil {
		existing = make(map[string]interface{})
	}

	// Ensure "hooks" key exists
	hooksRaw, _ := existing["hooks"].(map[string]interface{})
	if hooksRaw == nil {
		hooksRaw = make(map[string]interface{})
		existing["hooks"] = hooksRaw
	}

	// Extract desired hooks from the block
	desiredHooks, _ := hookBlock["hooks"].(map[string]interface{})

	// Iterate over each hook type in the desired block
	changed := false
	for hookType, entries := range desiredHooks {
		desiredArray, _ := entries.([]interface{})
		if len(desiredArray) == 0 {
			continue
		}

		// Get existing array for this hook type
		existingArray, _ := hooksRaw[hookType].([]interface{})
		if existingArray == nil {
			existingArray = make([]interface{}, 0)
		}

		// For each desired entry, check if it already exists (by exact command match)
		for _, desired := range desiredArray {
			desiredEntry, _ := desired.(map[string]interface{})
			desiredCmd, _ := desiredEntry["command"].(string)
			if desiredCmd == "" {
				continue
			}

			alreadyPresent := false
			for _, existingEntry := range existingArray {
				if ex, ok := existingEntry.(map[string]interface{}); ok {
					if exCmd, ok := ex["command"].(string); ok {
						if exCmd == desiredCmd {
							alreadyPresent = true
							break
						}
					}
				}
			}

			if !alreadyPresent {
				existingArray = append(existingArray, desired)
				changed = true
			}
		}

		hooksRaw[hookType] = existingArray
	}

	if !changed {
		fmt.Fprintf(os.Stderr, "All hooks already present in %s — skipping.\n", settingsPath)
		return nil
	}

	// Write back
	out, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding settings: %w", err)
	}
	if err := os.WriteFile(settingsPath, append(out, '\n'), 0600); err != nil {
		return fmt.Errorf("writing settings: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Hooks merged into %s\n", settingsPath)
	return nil
}

func init() {
	hookClaudeCodeCmd.Flags().BoolVar(&hookMerge, "merge", false, "Merge hook into ~/.claude/settings.json (idempotent)")
	hookClaudeCodeCmd.Flags().StringVar(&hookSettingsPath, "settings-path", defaultClaudeSettingsPath(), "Path to Claude Code settings file")

	hookCmd.AddCommand(hookClaudeCodeCmd)
	rootCmd.AddCommand(hookCmd)
}

// defaultClaudeSettingsPath returns ~/.claude/settings.json.
func defaultClaudeSettingsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".claude/settings.json"
	}
	return filepath.Join(home, ".claude", "settings.json")
}
