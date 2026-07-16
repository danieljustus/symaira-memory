package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
Claude Code. The hook JSON block is always printed to stdout; use --merge
to write it into the settings file idempotently.`,
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
func buildClaudeHookBlock() map[string]interface{} {
	return map[string]interface{}{
		"hooks": map[string]interface{}{
			"SessionStart": []interface{}{
				map[string]interface{}{
					"type":    "command",
					"command": "symmemory context --output md",
				},
			},
		},
	}
}

// mergeClaudeHook idempotently merges the symmemory SessionStart hook into
// the settings file at settingsPath. It creates the file if missing, parses
// existing JSON, checks for the hook by command marker, appends if absent,
// and writes back.
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

	// Ensure "SessionStart" key exists as array
	sessionStart, _ := hooksRaw["SessionStart"].([]interface{})
	if sessionStart == nil {
		sessionStart = make([]interface{}, 0)
	}

	// Check if our hook already exists (idempotency marker: "symmemory context" in command)
	for _, entry := range sessionStart {
		if m, ok := entry.(map[string]interface{}); ok {
			if cmd, ok := m["command"].(string); ok {
				if strings.Contains(cmd, "symmemory context") {
					fmt.Fprintf(os.Stderr, "Hook already present in %s — skipping.\n", settingsPath)
					return nil
				}
			}
		}
	}

	// Append our hook
	newHook := map[string]interface{}{
		"type":    "command",
		"command": "symmemory context --output md",
	}
	sessionStart = append(sessionStart, newHook)
	hooksRaw["SessionStart"] = sessionStart
	existing["hooks"] = hooksRaw

	// Write back
	out, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding settings: %w", err)
	}
	if err := os.WriteFile(settingsPath, append(out, '\n'), 0600); err != nil {
		return fmt.Errorf("writing settings: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Hook merged into %s\n", settingsPath)
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
