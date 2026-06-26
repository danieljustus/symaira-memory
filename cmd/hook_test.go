package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --------------------------------------------------------------------------
// Hook command registration
// --------------------------------------------------------------------------

func TestHookCommandRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "hook" {
			found = true
			if cmd.Short == "" {
				t.Error("hook command has empty Short description")
			}
		}
	}
	if !found {
		t.Error("hook command not registered on rootCmd")
	}
}

func TestHookClaudeCodeSubcommandRegistered(t *testing.T) {
	hookCmdFound := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "hook" {
			hookCmdFound = true
			found := false
			for _, sub := range cmd.Commands() {
				if sub.Use == "claude-code" {
					found = true
					if sub.Short == "" {
						t.Error("hook claude-code has empty Short description")
					}
				}
			}
			if !found {
				t.Error("claude-code subcommand not registered under hook")
			}
		}
	}
	if !hookCmdFound {
		t.Error("hook command not found on rootCmd")
	}
}

func TestHookClaudeCodeFlagsRegistered(t *testing.T) {
	cmd := findSubcommand(rootCmd, "hook", "claude-code")
	if cmd == nil {
		t.Fatal("hook claude-code command not found")
	}

	mergeFlag := cmd.Flags().Lookup("merge")
	if mergeFlag == nil {
		t.Fatal("expected 'merge' flag on hook claude-code command")
	}
	if mergeFlag.DefValue != "false" {
		t.Errorf("expected default 'false' for merge flag, got %q", mergeFlag.DefValue)
	}

	settingsFlag := cmd.Flags().Lookup("settings-path")
	if settingsFlag == nil {
		t.Fatal("expected 'settings-path' flag on hook claude-code command")
	}
}

// --------------------------------------------------------------------------
// PersistentPreRunE bypass
// --------------------------------------------------------------------------

func TestHookCommandBypassesDatabase(t *testing.T) {
	// hook parent should bypass DB
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "hook" {
			err := rootCmd.PersistentPreRunE(cmd, nil)
			if err != nil {
				t.Errorf("hook command should bypass DB: %v", err)
			}
		}
	}
}

func TestHookClaudeCodeBypassesDatabase(t *testing.T) {
	cmd := findSubcommand(rootCmd, "hook", "claude-code")
	if cmd == nil {
		t.Fatal("hook claude-code command not found")
	}
	err := rootCmd.PersistentPreRunE(cmd, nil)
	if err != nil {
		t.Errorf("hook claude-code should bypass DB: %v", err)
	}
}

// --------------------------------------------------------------------------
// JSON output validity
// --------------------------------------------------------------------------

func TestHookClaudeCodeOutputIsValidJSON(t *testing.T) {
	hookSettingsPath = t.TempDir() + "/.claude/settings.json"
	hookMerge = false
	defer func() {
		hookMerge = false
		hookSettingsPath = ""
	}()

	output := captureCmdOutput(func() {
		hookClaudeCodeCmd.RunE(hookClaudeCodeCmd, nil)
	})

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\nOutput:\n%s", err, output)
	}
}

func TestHookClaudeCodeOutputStructure(t *testing.T) {
	hookSettingsPath = t.TempDir() + "/.claude/settings.json"
	hookMerge = false
	defer func() {
		hookMerge = false
		hookSettingsPath = ""
	}()

	output := captureCmdOutput(func() {
		hookClaudeCodeCmd.RunE(hookClaudeCodeCmd, nil)
	})

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	// Check hooks key exists
	hooks, ok := parsed["hooks"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'hooks' key in output")
	}

	// Check SessionStart key exists
	sessionStart, ok := hooks["SessionStart"].([]interface{})
	if !ok {
		t.Fatal("expected 'SessionStart' array in hooks")
	}

	if len(sessionStart) != 1 {
		t.Fatalf("expected 1 SessionStart entry, got %d", len(sessionStart))
	}

	entry, ok := sessionStart[0].(map[string]interface{})
	if !ok {
		t.Fatal("expected SessionStart[0] to be an object")
	}

	if entry["type"] != "command" {
		t.Errorf("expected type 'command', got %v", entry["type"])
	}

	cmd, ok := entry["command"].(string)
	if !ok {
		t.Fatal("expected 'command' to be a string")
	}
	if cmd != "symmemory context --format md" {
		t.Errorf("expected command 'symmemory context --format md', got %q", cmd)
	}
}

// --------------------------------------------------------------------------
// Merge idempotency
// --------------------------------------------------------------------------

func TestHookClaudeCodeMergeCreatesFile(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, ".claude", "settings.json")

	hookSettingsPath = settingsPath
	hookMerge = true
	defer func() {
		hookMerge = false
		hookSettingsPath = ""
	}()

	stderr := captureStderr(func() {
		hookClaudeCodeCmd.RunE(hookClaudeCodeCmd, nil)
	})

	// File should exist
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("settings file not created: %v", err)
	}

	// Should contain our hook
	if !strings.Contains(string(data), "symmemory context") {
		t.Error("settings file does not contain symmemory context hook")
	}

	// Should confirm merge on stderr
	if !strings.Contains(stderr, "merged") {
		t.Errorf("expected merge confirmation on stderr, got: %q", stderr)
	}
}

func TestHookClaudeCodeMergeIsIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, ".claude", "settings.json")

	hookSettingsPath = settingsPath
	hookMerge = true
	defer func() {
		hookMerge = false
		hookSettingsPath = ""
	}()

	// Run merge twice
	hookClaudeCodeCmd.RunE(hookClaudeCodeCmd, nil)
	hookClaudeCodeCmd.RunE(hookClaudeCodeCmd, nil)

	// Read file and count occurrences of our hook
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings file: %v", err)
	}

	// Count occurrences of "symmemory context --format md"
	count := strings.Count(string(data), "symmemory context --format md")
	if count != 1 {
		t.Errorf("expected hook to appear exactly once after two merges, got %d occurrences", count)
	}
}

func TestHookClaudeCodeMergePreservesExistingSettings(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, ".claude", "settings.json")

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0700); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}

	// Write pre-existing settings
	existing := map[string]interface{}{
		"permissions": map[string]interface{}{
			"allow": []string{"Bash(npm run *)"},
		},
	}
	out, _ := json.MarshalIndent(existing, "", "  ")
	if err := os.WriteFile(settingsPath, append(out, '\n'), 0600); err != nil {
		t.Fatalf("failed to write pre-existing settings: %v", err)
	}

	hookSettingsPath = settingsPath
	hookMerge = true
	defer func() {
		hookMerge = false
		hookSettingsPath = ""
	}()

	hookClaudeCodeCmd.RunE(hookClaudeCodeCmd, nil)

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings file: %v", err)
	}

	content := string(data)

	// Existing content should be preserved
	if !strings.Contains(content, "permissions") {
		t.Error("existing 'permissions' key was lost after merge")
	}
	if !strings.Contains(content, "npm run") {
		t.Error("existing permission entry was lost after merge")
	}

	// Our hook should also be present
	if !strings.Contains(content, "symmemory context") {
		t.Error("hook not found after merge")
	}
}

func TestHookClaudeCodeMergeWithCorruptFile(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, ".claude", "settings.json")

	// Write corrupt JSON
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0700); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte("{corrupt json"), 0600); err != nil {
		t.Fatalf("failed to write corrupt file: %v", err)
	}

	hookSettingsPath = settingsPath
	hookMerge = true
	defer func() {
		hookMerge = false
		hookSettingsPath = ""
	}()

	// Should print error to stderr but return nil (fail-safe)
	err := hookClaudeCodeCmd.RunE(hookClaudeCodeCmd, nil)
	if err != nil {
		t.Errorf("expected nil error for corrupt file, got: %v", err)
	}
}

// --------------------------------------------------------------------------
// buildClaudeHookBlock unit test
// --------------------------------------------------------------------------

func TestBuildClaudeHookBlock(t *testing.T) {
	block := buildClaudeHookBlock()

	b, err := json.MarshalIndent(block, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal hook block: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("hook block is not valid JSON: %v", err)
	}

	// Verify structure
	hooks, ok := parsed["hooks"].(map[string]interface{})
	if !ok {
		t.Fatal("missing 'hooks' key")
	}
	ss, ok := hooks["SessionStart"].([]interface{})
	if !ok {
		t.Fatal("missing 'SessionStart' array")
	}
	if len(ss) != 1 {
		t.Fatalf("expected 1 entry in SessionStart, got %d", len(ss))
	}
	entry := ss[0].(map[string]interface{})
	if entry["type"] != "command" {
		t.Errorf("expected type 'command', got %v", entry["type"])
	}
	if entry["command"] != "symmemory context --format md" {
		t.Errorf("unexpected command: %v", entry["command"])
	}
}

// --------------------------------------------------------------------------
// mergeClaudeHook unit tests
// --------------------------------------------------------------------------

func TestMergeClaudeHookCreatesDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "deep", "nested", ".claude", "settings.json")

	block := buildClaudeHookBlock()
	err := mergeClaudeHook(settingsPath, block)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		t.Error("expected settings file to be created")
	}
}

func TestMergeClaudeHookIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "settings.json")

	block := buildClaudeHookBlock()

	// Merge twice
	if err := mergeClaudeHook(settingsPath, block); err != nil {
		t.Fatalf("first merge failed: %v", err)
	}
	if err := mergeClaudeHook(settingsPath, block); err != nil {
		t.Fatalf("second merge failed: %v", err)
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings: %v", err)
	}

	count := strings.Count(string(data), "symmemory context --format md")
	if count != 1 {
		t.Errorf("expected hook once, found %d times", count)
	}
}
