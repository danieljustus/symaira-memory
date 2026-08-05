package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --------------------------------------------------------------------------
// Registration
// --------------------------------------------------------------------------

func TestHookMCPSubcommandsRegistered(t *testing.T) {
	expected := []string{"codex", "opencode", "cursor", "claude-desktop", "vscode"}
	for _, name := range expected {
		cmd := findSubcommand(rootCmd, "hook", name)
		if cmd == nil {
			t.Errorf("expected hook %s subcommand to be registered", name)
			continue
		}
		if cmd.Short == "" {
			t.Errorf("hook %s has empty Short description", name)
		}
		for _, flag := range []string{"merge", "config-path", "profile"} {
			if cmd.Flags().Lookup(flag) == nil {
				t.Errorf("hook %s missing --%s flag", name, flag)
			}
		}
	}
}

func TestHookListFlagRegistered(t *testing.T) {
	if hookCmd.Flags().Lookup("list") == nil {
		t.Fatal("expected 'list' flag on hook command")
	}
}

// --------------------------------------------------------------------------
// Generation: each supported client prints a valid config block to stdout
// --------------------------------------------------------------------------

func TestHookMCPGeneration(t *testing.T) {
	for i := range hookMCPClients {
		p := &hookMCPClients[i]
		t.Run(p.Name, func(t *testing.T) {
			hookMCPPath = filepath.Join(t.TempDir(), "config")
			hookMCPMerge = false
			hookMCPProfile = ""
			defer func() {
				hookMCPPath = ""
				hookMCPMerge = false
				hookMCPProfile = ""
			}()

			cmd := findSubcommand(rootCmd, "hook", p.Name)
			if cmd == nil {
				t.Fatal("subcommand not registered")
			}
			output := captureCmdOutput(func() {
				if err := cmd.RunE(cmd, nil); err != nil {
					t.Errorf("hook %s returned error: %v", p.Name, err)
				}
			})
			if strings.TrimSpace(output) == "" {
				t.Fatal("expected output on stdout")
			}

			if p.Format == "toml" {
				if !strings.Contains(output, "[mcp_servers."+mcpServerName+"]") {
					t.Errorf("codex output missing TOML section header:\n%s", output)
				}
				if !strings.Contains(output, `"serve"`) {
					t.Errorf("expected serve arg in codex output:\n%s", output)
				}
				return
			}

			var parsed map[string]interface{}
			if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &parsed); err != nil {
				t.Fatalf("hook %s output is not valid JSON: %v\n%s", p.Name, err, output)
			}
			root, ok := parsed[p.RootKey].(map[string]interface{})
			if !ok {
				t.Fatalf("expected root key %q in output:\n%s", p.RootKey, output)
			}
			entry, ok := root[mcpServerName].(map[string]interface{})
			if !ok {
				t.Fatalf("expected %q entry under %q:\n%s", mcpServerName, p.RootKey, output)
			}
			if p.ArrayCmd {
				cmdArr, ok := entry["command"].([]interface{})
				if !ok || !stringSliceContains(cmdArr, "serve") {
					t.Errorf("expected command array containing serve, got %v", entry["command"])
				}
				if entry["type"] != "local" || entry["enabled"] != true {
					t.Errorf("expected type local and enabled true, got %v", entry)
				}
			} else {
				if entry["command"] == nil || entry["args"] == nil {
					t.Errorf("expected command and args in entry, got %v", entry)
				}
				if !strings.Contains(output, `"serve"`) {
					t.Errorf("expected serve arg in output:\n%s", output)
				}
			}
		})
	}
}

func stringSliceContains(arr []interface{}, s string) bool {
	for _, v := range arr {
		if str, ok := v.(string); ok && str == s {
			return true
		}
	}
	return false
}

// --------------------------------------------------------------------------
// Merge: idempotent on repeat, never destroys unrelated keys
// --------------------------------------------------------------------------

func TestHookMCPMergeIdempotent(t *testing.T) {
	for _, name := range []string{"codex", "cursor"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config."+name)
			hookMCPPath = path
			hookMCPMerge = true
			hookMCPProfile = ""
			defer func() {
				hookMCPPath = ""
				hookMCPMerge = false
				hookMCPProfile = ""
			}()

			cmd := findSubcommand(rootCmd, "hook", name)
			if cmd == nil {
				t.Fatal("subcommand not registered")
			}

			// Merge twice; second run must be a no-op.
			captureCmdOutput(func() {
				if err := cmd.RunE(cmd, nil); err != nil {
					t.Fatalf("first merge failed: %v", err)
				}
			})
			captureCmdOutput(func() {
				if err := cmd.RunE(cmd, nil); err != nil {
					t.Fatalf("second merge failed: %v", err)
				}
			})

			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("config file not created: %v", err)
			}
			count := strings.Count(string(data), mcpServerName)
			if count != 1 {
				t.Errorf("expected %q exactly once after two merges, got %d:\n%s", mcpServerName, count, data)
			}
		})
	}
}

func TestHookMCPMergePreservesUnrelatedKeys(t *testing.T) {
	t.Run("cursor-json", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "mcp.json")
		existing := map[string]interface{}{
			"mcpServers": map[string]interface{}{
				"other-server": map[string]interface{}{
					"command": "some-other-tool",
					"args":    []string{"--flag"},
				},
			},
			"experimental": map[string]interface{}{"enabled": true},
		}
		out, _ := json.MarshalIndent(existing, "", "  ")
		if err := os.WriteFile(path, append(out, '\n'), 0600); err != nil {
			t.Fatal(err)
		}

		hookMCPPath = path
		hookMCPMerge = true
		hookMCPProfile = ""
		defer func() {
			hookMCPPath = ""
			hookMCPMerge = false
			hookMCPProfile = ""
		}()

		cmd := findSubcommand(rootCmd, "hook", "cursor")
		if cmd == nil {
			t.Fatal("cursor subcommand not registered")
		}
		captureCmdOutput(func() {
			if err := cmd.RunE(cmd, nil); err != nil {
				t.Fatalf("merge failed: %v", err)
			}
		})

		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read merged config: %v", err)
		}
		s := string(content)
		if !strings.Contains(s, "other-server") || !strings.Contains(s, "some-other-tool") {
			t.Error("unrelated mcpServers entry was lost after merge")
		}
		if !strings.Contains(s, "experimental") {
			t.Error("unrelated top-level key was lost after merge")
		}
		if !strings.Contains(s, mcpServerName) {
			t.Error("symaira-memory entry not present after merge")
		}
	})

	t.Run("codex-toml", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.toml")
		preexisting := "[model]\nmodel = \"gpt-5\"\n\n[experimental]\nenable = true\n"
		if err := os.WriteFile(path, []byte(preexisting), 0600); err != nil {
			t.Fatal(err)
		}

		hookMCPPath = path
		hookMCPMerge = true
		hookMCPProfile = ""
		defer func() {
			hookMCPPath = ""
			hookMCPMerge = false
			hookMCPProfile = ""
		}()

		cmd := findSubcommand(rootCmd, "hook", "codex")
		if cmd == nil {
			t.Fatal("codex subcommand not registered")
		}
		captureCmdOutput(func() {
			if err := cmd.RunE(cmd, nil); err != nil {
				t.Fatalf("merge failed: %v", err)
			}
		})

		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read merged config: %v", err)
		}
		s := string(content)
		if !strings.Contains(s, `model = "gpt-5"`) || !strings.Contains(s, "[experimental]") {
			t.Error("unrelated TOML content was lost after merge")
		}
		if strings.Count(s, "[mcp_servers."+mcpServerName+"]") != 1 {
			t.Errorf("expected mcp_servers section exactly once:\n%s", s)
		}
	})
}

// --------------------------------------------------------------------------
// Listing and unknown-client error
// --------------------------------------------------------------------------

func TestHookListOutput(t *testing.T) {
	hookList = true
	defer func() { hookList = false }()

	output := captureCmdOutput(func() {
		if err := hookCmd.RunE(hookCmd, nil); err != nil {
			t.Fatalf("hook --list returned error: %v", err)
		}
	})
	for i := range hookMCPClients {
		if !strings.Contains(output, hookMCPClients[i].Name) {
			t.Errorf("hook --list missing %q:\n%s", hookMCPClients[i].Name, output)
		}
	}
}

func TestHookUnknownClientError(t *testing.T) {
	err := hookCmd.RunE(hookCmd, []string{"bogus-client"})
	if err == nil {
		t.Fatal("expected error for unknown client")
	}
	if !strings.Contains(err.Error(), "bogus-client") || !strings.Contains(err.Error(), "supported clients") {
		t.Errorf("error should name the client and the supported set, got: %v", err)
	}
}
