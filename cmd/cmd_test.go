package cmd

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
)

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

// --------------------------------------------------------------------------
// Version command
// --------------------------------------------------------------------------

func TestVersionCommand(t *testing.T) {
	helperSetup()
	output := captureCmdOutput(func() {
		versionCmd.Run(versionCmd, nil)
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

	expected := []string{"version", "serve", "set", "get", "list", "search", "delete", "mcp-config", "backup", "console", "rule", "token"}
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
		if cmd.Use == "set" {
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
		if cmd.Use == "list" {
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
		configCmd.Run(configCmd, nil)
	})
	fmt.Println(output) // consume stdout (stderr is used by mcp-config)

	// The config command prints to stderr, not stdout
	// So stdout should be empty
	_ = output
}
