package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestNoSubcommandRedefinesOutputFlags asserts no subcommand redefines the
// canonical output flags as local flags: --output/--format live on the root
// (persistent), and --json is only ever a hidden deprecated alias.
//
// Regression test for #532.
func TestNoSubcommandRedefinesOutputFlags(t *testing.T) {
	rootOutput := rootCmd.PersistentFlags().Lookup("output")
	rootFormat := rootCmd.PersistentFlags().Lookup("format")
	for _, c := range rootCmd.Commands() {
		// A local redefinition is a *different* flag object than the root's
		// persistent flag; an inherited flag is the very same pointer.
		if f := c.Flags().Lookup("output"); f != nil && f != rootOutput {
			t.Errorf("subcommand %q redefines %q as a local flag", c.Name(), "output")
		}
		if f := c.Flags().Lookup("format"); f != nil && f != rootFormat {
			t.Errorf("subcommand %q redefines %q as a local flag", c.Name(), "format")
		}
		if f := c.Flags().Lookup("json"); f != nil && !f.Hidden {
			t.Errorf("subcommand %q has a non-hidden --json flag; want a hidden deprecated alias", c.Name())
		}
	}
}

// TestGetOutputFormat_ResolvesFromCommand verifies GetOutputFormat reads the
// output format from the passed command's flag set, not only the package
// variable.
func TestGetOutputFormat_ResolvesFromCommand(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("output", "table", "output format")

	if got := GetOutputFormat(cmd); got != "table" {
		t.Fatalf("default = %q, want %q", got, "table")
	}
	if err := cmd.Flags().Set("output", "json"); err != nil {
		t.Fatalf("set --output: %v", err)
	}
	if got := GetOutputFormat(cmd); got != "json" {
		t.Fatalf("after set = %q, want %q", got, "json")
	}
}
