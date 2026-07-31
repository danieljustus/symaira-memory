package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// --------------------------------------------------------------------------
// Observe command registration
// --------------------------------------------------------------------------

func TestObserveCommandRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "observe" {
			found = true
			if cmd.Short == "" {
				t.Error("observe command has empty Short description")
			}
		}
	}
	if !found {
		t.Error("observe command not registered on rootCmd")
	}
}

func TestObserveSubcommandsRegistered(t *testing.T) {
	obsCmd := findSubcommand(rootCmd, "observe")
	if obsCmd == nil {
		t.Fatal("observe command not found")
	}

	expected := []string{"tool-failure", "session-end", "pre-compact"}
	registered := make(map[string]bool)
	for _, sub := range obsCmd.Commands() {
		registered[sub.Use] = true
		if sub.Short == "" {
			t.Errorf("observe %s has empty Short description", sub.Use)
		}
	}

	for _, name := range expected {
		if !registered[name] {
			t.Errorf("observe %s subcommand not registered", name)
		}
	}
}

func TestObserveBypassesPersistentPreRunE(t *testing.T) {
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "observe" {
			err := rootCmd.PersistentPreRunE(cmd, nil)
			if err != nil {
				t.Errorf("observe command should bypass PersistentPreRunE, got error: %v", err)
			}
			return
		}
	}
	t.Error("observe command not found")
}

func TestObserveSubcommandsBypassPersistentPreRunE(t *testing.T) {
	obsCmd := findSubcommand(rootCmd, "observe")
	if obsCmd == nil {
		t.Fatal("observe command not found")
	}

	for _, sub := range obsCmd.Commands() {
		t.Run(sub.Use, func(t *testing.T) {
			err := rootCmd.PersistentPreRunE(sub, nil)
			if err != nil {
				t.Errorf("observe %s should bypass PersistentPreRunE, got error: %v", sub.Use, err)
			}
		})
	}
}

// --------------------------------------------------------------------------
// Fail-safe behavior: never write stdout, always return nil
// --------------------------------------------------------------------------

func TestObserveToolFailureNeverWritesStdout(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := observeToolFailureCmd.RunE(observeToolFailureCmd, nil)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	stdout := strings.TrimSpace(buf.String())

	if err != nil {
		t.Errorf("expected nil error (fail-safe), got: %v", err)
	}
	if stdout != "" {
		t.Errorf("expected no stdout output (fail-safe), got: %q", stdout)
	}
}

func TestObserveSessionEndNeverWritesStdout(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := observeSessionEndCmd.RunE(observeSessionEndCmd, nil)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	stdout := strings.TrimSpace(buf.String())

	if err != nil {
		t.Errorf("expected nil error (fail-safe), got: %v", err)
	}
	if stdout != "" {
		t.Errorf("expected no stdout output (fail-safe), got: %q", stdout)
	}
}

func TestObservePreCompactNeverWritesStdout(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := observePreCompactCmd.RunE(observePreCompactCmd, nil)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	stdout := strings.TrimSpace(buf.String())

	if err != nil {
		t.Errorf("expected nil error (fail-safe), got: %v", err)
	}
	if stdout != "" {
		t.Errorf("expected no stdout output (fail-safe), got: %q", stdout)
	}
}

func TestObserveToolFailureWithDataNeverWritesStdout(t *testing.T) {
	observeToolFailureCmd.Flags().Set("data", `{"tool":"bash","error":"exit code 1"}`)
	defer observeToolFailureCmd.Flags().Set("data", "")

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := observeToolFailureCmd.RunE(observeToolFailureCmd, nil)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	stdout := strings.TrimSpace(buf.String())

	if err != nil {
		t.Errorf("expected nil error (fail-safe), got: %v", err)
	}
	if stdout != "" {
		t.Errorf("expected no stdout output (fail-safe), got: %q", stdout)
	}
}

// --------------------------------------------------------------------------
// Data flag
// --------------------------------------------------------------------------

func TestObserveDataFlagRegistered(t *testing.T) {
	obsCmd := findSubcommand(rootCmd, "observe")
	if obsCmd == nil {
		t.Fatal("observe command not found")
	}

	for _, sub := range obsCmd.Commands() {
		t.Run(sub.Use, func(t *testing.T) {
			flag := sub.Flags().Lookup("data")
			if flag == nil {
				t.Errorf("expected 'data' flag on observe %s", sub.Use)
			}
		})
	}
}
