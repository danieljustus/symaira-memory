package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// findSubcommand walks the command tree and returns the command at the given path.
// Example: findSubcommand(rootCmd, "token", "generate")
func findSubcommand(root *cobra.Command, path ...string) *cobra.Command {
	cur := root
	for _, name := range path {
		found := false
		for _, sub := range cur.Commands() {
			if sub.Use == name {
				cur = sub
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	return cur
}

// helperSetup sets Version info and captures stdout.
func helperSetup() {
	SetVersionInfo("0.1.0", "abc1234", "2026-06-03")
}

func captureCmdOutput(fn func()) string {
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	return buf.String()
}

func captureStderr(fn func()) string {
	r, w, _ := os.Pipe()
	old := os.Stderr
	os.Stderr = w

	fn()

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	return buf.String()
}

// --------------------------------------------------------------------------
// Version command
// --------------------------------------------------------------------------

func TestVersionCommand(t *testing.T) {
	helperSetup()
	output := captureCmdOutput(func() {
		versionCmd.RunE(versionCmd, nil)
	})

	if !strings.Contains(output, "symmemory version 0.1.0") {
		t.Errorf("expected version info, got %q", output)
	}
	if !strings.Contains(output, "abc1234") {
		t.Errorf("expected commit hash, got %q", output)
	}
	if !strings.Contains(output, "2026-06-03") {
		t.Errorf("expected date, got %q", output)
	}
}

// --------------------------------------------------------------------------
// Command structure tests
// --------------------------------------------------------------------------

func TestRootCommandStructure(t *testing.T) {
	if rootCmd.Use != "symmemory" {
		t.Errorf("expected Use 'symmemory', got %q", rootCmd.Use)
	}
	if rootCmd.Short == "" {
		t.Error("expected non-empty Short description")
	}
	if rootCmd.Long == "" {
		t.Error("expected non-empty Long description")
	}
}

func TestRootHasSubcommands(t *testing.T) {
	cmds := rootCmd.Commands()
	t.Logf("Found %d direct subcommands on rootCmd", len(cmds))
	for _, cmd := range cmds {
		t.Logf("  subcommand: %q", cmd.Use)
	}

	expected := []string{"version", "serve", "set", "get", "list", "search", "delete", "mcp-config", "backup", "console", "rule", "token", "sync"}
	for _, name := range expected {
		found := false
		for _, cmd := range cmds {
			if cmd.Use == name || strings.HasPrefix(cmd.Use, name+" ") || strings.HasPrefix(cmd.Use, name+" [") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected subcommand %q to be registered", name)
		}
	}
}

func TestCommandHasDescription(t *testing.T) {
	for _, cmd := range rootCmd.Commands() {
		if cmd.Short == "" {
			t.Errorf("subcommand %q has empty Short description", cmd.Use)
		}
	}
}

// --------------------------------------------------------------------------
// Flag registration tests
// --------------------------------------------------------------------------

func TestServeCommandFlags(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "serve" {
			found = true
			flag := cmd.Flags().Lookup("port")
			if flag == nil {
				t.Error("expected 'port' flag on serve command")
			}
		}
	}
	if !found {
		t.Error("serve command not found")
	}
}

func TestSearchCommandFlags(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "search [query]" {
			found = true
			for _, name := range []string{"scope", "limit"} {
				if cmd.Flags().Lookup(name) == nil {
					t.Errorf("expected %q flag on search command", name)
				}
			}
		}
	}
	if !found {
		t.Error("search command not found")
	}
}

func TestSetCommandFlags(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "set" {
			found = true
			for _, name := range []string{"value", "scope"} {
				if cmd.Flags().Lookup(name) == nil {
					t.Errorf("expected %q flag on set command", name)
				}
			}
		}
	}
	if !found {
		t.Error("set command not found")
	}
}

func TestListCommandFlags(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "list" {
			found = true
			if cmd.Flags().Lookup("scope") == nil {
				t.Error("expected 'scope' flag on list command")
			}
		}
	}
	if !found {
		t.Error("list command not found")
	}
}

// --------------------------------------------------------------------------
// Argument validation tests
// --------------------------------------------------------------------------

func TestSearchCommandRequiresArgs(t *testing.T) {
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "search [query]" {
			if cmd.Args == nil {
				t.Error("search command should require arguments")
			}
			// Test with no args
			err := cmd.Args(cmd, []string{})
			if err == nil {
				t.Error("expected error for search with no args")
			}
			// Test with exactly one arg
			err = cmd.Args(cmd, []string{"test query"})
			if err != nil {
				t.Errorf("expected no error for search with 1 arg, got: %v", err)
			}
		}
	}
}

func TestGetCommandRequiresArgs(t *testing.T) {
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "get [id]" {
			if cmd.Args == nil {
				t.Error("get command should require arguments")
			}
			err := cmd.Args(cmd, []string{})
			if err == nil {
				t.Error("expected error for get with no args")
			}
		}
	}
}

// --------------------------------------------------------------------------
// SetVersionInfo
// --------------------------------------------------------------------------

func TestSetVersionInfo(t *testing.T) {
	SetVersionInfo("9.9.9", "deadbeef", "2099-01-01")
	if Version != "9.9.9" {
		t.Errorf("expected Version '9.9.9', got %q", Version)
	}
	if Commit != "deadbeef" {
		t.Errorf("expected Commit 'deadbeef', got %q", Commit)
	}
	if Date != "2099-01-01" {
		t.Errorf("expected Date '2099-01-01', got %q", Date)
	}
}

// --------------------------------------------------------------------------
// RootDB bypass for helper commands
// --------------------------------------------------------------------------

func TestPersistentPreRunBypassesDatabase(t *testing.T) {
	// Commands that should bypass DB: version, mcp-config
	bypassCmds := []string{"version", "mcp-config"}
	for _, name := range bypassCmds {
		for _, cmd := range rootCmd.Commands() {
			if cmd.Use == name {
				// PersistentPreRunE should bypass DB opening
				err := rootCmd.PersistentPreRunE(cmd, nil)
				if err != nil {
					t.Errorf("command %q should not fail in PreRun: %v", name, err)
				}
			}
		}
	}
}

// --------------------------------------------------------------------------
// MCP Config command output
// --------------------------------------------------------------------------

func TestMcpConfigCommandOutput(t *testing.T) {
	output := captureCmdOutput(func() {
		configCmd.RunE(configCmd, nil)
	})
	fmt.Println(output) // consume stdout (stderr is used by mcp-config)

	// The config command prints to stderr, not stdout
	// So stdout should be empty
	_ = output
}

func TestMcpConfigProfileFlagRegistered(t *testing.T) {
	flag := configCmd.Flags().Lookup("profile")
	if flag == nil {
		t.Fatal("expected 'profile' flag on mcp-config command")
	}
	if flag.DefValue != "" {
		t.Errorf("expected empty default for profile flag, got %q", flag.DefValue)
	}
}

func TestMcpConfigDefaultArgsNoProfile(t *testing.T) {
	configProfile = ""

	output := captureStderr(func() {
		configCmd.RunE(configCmd, nil)
	})

	if !strings.Contains(output, `"serve"`) {
		t.Errorf("expected output to contain 'serve', got %q", output)
	}
	if strings.Contains(output, "--profile") {
		t.Errorf("expected output to NOT contain --profile when no profile set, got %q", output)
	}
}

func TestMcpConfigWithProfile(t *testing.T) {
	configProfile = "claude-code"
	defer func() { configProfile = "" }()

	output := captureStderr(func() {
		configCmd.RunE(configCmd, nil)
	})

	if !strings.Contains(output, "--profile") {
		t.Errorf("expected output to contain --profile, got %q", output)
	}
	if !strings.Contains(output, "claude-code") {
		t.Errorf("expected output to contain 'claude-code', got %q", output)
	}
	if !strings.Contains(output, "Active profile: claude-code") {
		t.Errorf("expected output to contain 'Active profile: claude-code', got %q", output)
	}
}

// --------------------------------------------------------------------------
// MCP Config --tool flag
// --------------------------------------------------------------------------

func TestMcpConfigToolFlagRegistered(t *testing.T) {
	flag := configCmd.Flags().Lookup("tool")
	if flag == nil {
		t.Fatal("expected 'tool' flag on mcp-config command")
	}
	if flag.DefValue != "" {
		t.Errorf("expected empty default for tool flag, got %q", flag.DefValue)
	}
}

func TestMcpConfigAllPresetsRegistered(t *testing.T) {
	expected := []string{"claude-code", "opencode", "codex", "kimi", "copilot"}
	for _, name := range expected {
		if _, ok := toolPresets[name]; !ok {
			t.Errorf("expected tool preset %q to be registered", name)
		}
	}
	if len(toolPresets) != len(expected) {
		t.Errorf("expected %d tool presets, got %d", len(expected), len(toolPresets))
	}
}

func TestMcpConfigDefaultToolIsClaudeCode(t *testing.T) {
	configTool = ""
	configProfile = ""

	output := captureStderr(func() {
		configCmd.RunE(configCmd, nil)
	})

	// Default should produce claude-code format (mcpServers with command/args)
	if !strings.Contains(output, `"mcpServers"`) {
		t.Errorf("expected default output to contain mcpServers, got %q", output)
	}
	if !strings.Contains(output, `"command"`) {
		t.Errorf("expected default output to contain command, got %q", output)
	}
	if !strings.Contains(output, `"args"`) {
		t.Errorf("expected default output to contain args, got %q", output)
	}
}

func TestMcpConfigClaudeCodePreset(t *testing.T) {
	configTool = "claude-code"
	configProfile = ""
	defer func() { configTool = ""; configProfile = "" }()

	output := captureStderr(func() {
		configCmd.RunE(configCmd, nil)
	})

	if !strings.Contains(output, `"mcpServers"`) {
		t.Errorf("expected claude-code output to contain mcpServers, got %q", output)
	}
	if !strings.Contains(output, `"serve"`) {
		t.Errorf("expected claude-code output to contain serve, got %q", output)
	}
}

func TestMcpConfigOpenCodePreset(t *testing.T) {
	configTool = "opencode"
	configProfile = ""
	defer func() { configTool = ""; configProfile = "" }()

	output := captureStderr(func() {
		configCmd.RunE(configCmd, nil)
	})

	// OpenCode uses "mcp" root key and "type": "local"
	if !strings.Contains(output, `"mcp"`) {
		t.Errorf("expected opencode output to contain mcp root key, got %q", output)
	}
	if !strings.Contains(output, `"type": "local"`) {
		t.Errorf("expected opencode output to contain type: local, got %q", output)
	}
	if !strings.Contains(output, `"enabled": true`) {
		t.Errorf("expected opencode output to contain enabled: true, got %q", output)
	}
	// OpenCode command should be an array containing both exec path and "serve"
	if !strings.Contains(output, `"serve"`) {
		t.Errorf("expected opencode output to contain serve in command array, got %q", output)
	}
}

func TestMcpConfigCodexPreset(t *testing.T) {
	configTool = "codex"
	configProfile = ""
	defer func() { configTool = ""; configProfile = "" }()

	output := captureStderr(func() {
		configCmd.RunE(configCmd, nil)
	})

	// Codex produces TOML output
	if !strings.Contains(output, "[mcp_servers.symaira-memory]") {
		t.Errorf("expected codex output to contain TOML section header, got %q", output)
	}
	if !strings.Contains(output, `command =`) {
		t.Errorf("expected codex output to contain command =, got %q", output)
	}
	if !strings.Contains(output, `args =`) {
		t.Errorf("expected codex output to contain args =, got %q", output)
	}
	if !strings.Contains(output, `"serve"`) {
		t.Errorf("expected codex output to contain serve arg, got %q", output)
	}
}

func TestMcpConfigKimiPreset(t *testing.T) {
	configTool = "kimi"
	configProfile = ""
	defer func() { configTool = ""; configProfile = "" }()

	output := captureStderr(func() {
		configCmd.RunE(configCmd, nil)
	})

	// Kimi uses same format as claude-code (mcpServers with command/args)
	if !strings.Contains(output, `"mcpServers"`) {
		t.Errorf("expected kimi output to contain mcpServers, got %q", output)
	}
	if !strings.Contains(output, `"command"`) {
		t.Errorf("expected kimi output to contain command, got %q", output)
	}
	if !strings.Contains(output, `"args"`) {
		t.Errorf("expected kimi output to contain args, got %q", output)
	}
}

func TestMcpConfigCopilotPreset(t *testing.T) {
	configTool = "copilot"
	configProfile = ""
	defer func() { configTool = ""; configProfile = "" }()

	output := captureStderr(func() {
		configCmd.RunE(configCmd, nil)
	})

	// Copilot requires type, env, and tools fields
	if !strings.Contains(output, `"mcpServers"`) {
		t.Errorf("expected copilot output to contain mcpServers, got %q", output)
	}
	if !strings.Contains(output, `"type": "local"`) {
		t.Errorf("expected copilot output to contain type: local, got %q", output)
	}
	if !strings.Contains(output, `"tools"`) {
		t.Errorf("expected copilot output to contain tools field, got %q", output)
	}
	if !strings.Contains(output, `"*"`) {
		t.Errorf("expected copilot output to contain tools: [*], got %q", output)
	}
	if !strings.Contains(output, `"env"`) {
		t.Errorf("expected copilot output to contain env field, got %q", output)
	}
}

func TestMcpConfigToolWithProfile(t *testing.T) {
	configTool = "copilot"
	configProfile = "my-project"
	defer func() { configTool = ""; configProfile = "" }()

	output := captureStderr(func() {
		configCmd.RunE(configCmd, nil)
	})

	if !strings.Contains(output, "--profile") {
		t.Errorf("expected output to contain --profile flag in args, got %q", output)
	}
	if !strings.Contains(output, "my-project") {
		t.Errorf("expected output to contain profile name 'my-project', got %q", output)
	}
	if !strings.Contains(output, "Active profile: my-project") {
		t.Errorf("expected stderr to show active profile, got %q", output)
	}
}

func TestMcpConfigInvalidToolExits(t *testing.T) {
	configTool = "nonexistent"
	defer func() { configTool = "" }()

	// Invalid tool should cause os.Exit(1), which we can't easily test.
	// Instead, verify the tool name validation logic via toolPresets map.
	if _, ok := toolPresets["nonexistent"]; ok {
		t.Error("expected 'nonexistent' to not be a valid tool preset")
	}
}

func TestMcpConfigOutputIsValidJSON(t *testing.T) {
	jsonTools := []string{"claude-code", "opencode", "copilot", "kimi"}
	for _, tool := range jsonTools {
		t.Run(tool, func(t *testing.T) {
			configTool = tool
			configProfile = ""

			stderr := captureStderr(func() {
				configCmd.RunE(configCmd, nil)
			})

			// Extract the JSON block from stderr between the separator lines
			configBlock := extractConfigBlock(stderr)
			if configBlock == "" {
				t.Fatalf("no config block found in stderr output for tool %q", tool)
			}

			// Validate it's valid JSON
			var parsed interface{}
			if err := json.Unmarshal([]byte(configBlock), &parsed); err != nil {
				t.Errorf("tool %q produced invalid JSON: %v\nBlock:\n%s", tool, err, configBlock)
			}
		})
	}
	configTool = ""
}

func extractConfigBlock(output string) string {
	startMarker := "========================= CONFIGURATION BLOCK ========================="
	endMarker := "======================================================================="

	startIdx := strings.Index(output, startMarker)
	if startIdx == -1 {
		return ""
	}
	startIdx += len(startMarker) + 1 // skip marker + newline

	endIdx := strings.Index(output[startIdx:], endMarker)
	if endIdx == -1 {
		return output[startIdx:]
	}
	return strings.TrimSpace(output[startIdx : startIdx+endIdx])
}

// --------------------------------------------------------------------------
// Token command structure and flags
// --------------------------------------------------------------------------

func TestTokenCommandHasSubcommands(t *testing.T) {
	var tokenCmd = findSubcommand(rootCmd, "token")
	if tokenCmd == nil {
		t.Fatal("token command not found")
	}

	subs := tokenCmd.Commands()
	if len(subs) < 2 {
		t.Errorf("expected at least 2 subcommands under token, got %d", len(subs))
	}

	found := map[string]bool{"generate": false, "verify": false}
	for _, sub := range subs {
		if sub.Use == "generate" {
			found["generate"] = true
		}
		if sub.Use == "verify [token]" {
			found["verify"] = true
		}
	}
	for name, ok := range found {
		if !ok {
			t.Errorf("expected token subcommand %q", name)
		}
	}
}

func TestTokenGenerateCommandFlags(t *testing.T) {
	genCmd := findSubcommand(rootCmd, "token", "generate")
	if genCmd == nil {
		t.Fatal("token generate command not found")
	}

	for _, name := range []string{"subject", "duration"} {
		if genCmd.Flags().Lookup(name) == nil {
			t.Errorf("expected %q flag on token generate command", name)
		}
	}

	// Default subject should be "extension"
	subjFlag := genCmd.Flags().Lookup("subject")
	if subjFlag.DefValue != "extension" {
		t.Errorf("expected default subject 'extension', got %q", subjFlag.DefValue)
	}
}

func TestTokenVerifyCommandRequiresArgs(t *testing.T) {
	verifyCmd := findSubcommand(rootCmd, "token", "verify [token]")
	if verifyCmd == nil {
		t.Fatal("token verify command not found")
	}

	err := verifyCmd.Args(verifyCmd, []string{})
	if err == nil {
		t.Error("expected error for verify with no args")
	}

	err = verifyCmd.Args(verifyCmd, []string{"some-token"})
	if err != nil {
		t.Errorf("expected no error for verify with 1 arg, got: %v", err)
	}
}

// --------------------------------------------------------------------------
// Backup command structure
// --------------------------------------------------------------------------

func TestBackupCommandHasSubcommands(t *testing.T) {
	bkCmd := findSubcommand(rootCmd, "backup")
	if bkCmd == nil {
		t.Fatal("backup command not found")
	}

	subs := bkCmd.Commands()
	found := map[string]bool{"export": false, "restore": false}
	for _, sub := range subs {
		if sub.Use == "export [destination.tar.gz]" {
			found["export"] = true
		}
		if sub.Use == "restore [source.tar.gz]" {
			found["restore"] = true
		}
	}
	for name, ok := range found {
		if !ok {
			t.Errorf("expected backup subcommand %q", name)
		}
	}

	if bkCmd.PersistentFlags().Lookup("password") == nil {
		t.Error("expected 'password' persistent flag on backup command")
	}
}

func TestBackupExportRequiresArgs(t *testing.T) {
	exportCmd := findSubcommand(rootCmd, "backup", "export [destination.tar.gz]")
	if exportCmd == nil {
		t.Fatal("backup export command not found")
	}

	err := exportCmd.Args(exportCmd, []string{})
	if err == nil {
		t.Error("expected error for export with no args")
	}
}

// --------------------------------------------------------------------------
// Console and Rule command structure
// --------------------------------------------------------------------------

func TestConsoleCommandRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "console" {
			found = true
			if cmd.Short == "" {
				t.Error("console command has empty Short")
			}
		}
	}
	if !found {
		t.Error("console command not found")
	}
}

func TestRuleCommandHasSubcommands(t *testing.T) {
	ruleCmd := findSubcommand(rootCmd, "rule")
	if ruleCmd == nil {
		t.Fatal("rule command not found")
	}

	subs := ruleCmd.Commands()
	expected := map[string]bool{"add [instruction]": false, "list": false, "delete [id]": false}
	for _, sub := range subs {
		if _, ok := expected[sub.Use]; ok {
			expected[sub.Use] = true
		}
	}
	for name, ok := range expected {
		if !ok {
			t.Errorf("expected rule subcommand %q", name)
		}
	}
}

// --------------------------------------------------------------------------
// DB getter/setter
// --------------------------------------------------------------------------

func TestGetDBReturnsNilWithoutInit(t *testing.T) {
	SetDB(nil)

	if db := GetDB(); db != nil {
		t.Error("expected nil from GetDB when not initialized")
	}
}

// --------------------------------------------------------------------------
// Delete command args
// --------------------------------------------------------------------------

func TestDeleteCommandRequiresArgs(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "delete [id]" {
			found = true
			err := cmd.Args(cmd, []string{})
			if err == nil {
				t.Error("expected error for delete with no args")
			}
		}
	}
	if !found {
		t.Error("delete command not found")
	}
}
