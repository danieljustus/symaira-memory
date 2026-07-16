package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestContextCommandRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "context" {
			found = true
			if cmd.Short == "" {
				t.Error("context command has empty Short description")
			}
			break
		}
	}
	if !found {
		t.Error("context command not registered on rootCmd")
	}
}

func TestContextCommandFlags(t *testing.T) {
	var ctxCmd *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "context" {
			ctxCmd = cmd
			break
		}
	}
	if ctxCmd == nil {
		t.Fatal("context command not found")
	}

	tests := []struct {
		name         string
		flagName     string
		defaultValue string
	}{
		{"query flag", "query", ""},
		{"budget flag", "budget", "0"},
		{"output flag inherited", "output", "table"},
		{"scope flag", "scope", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var flag *pflag.Flag
			if tt.flagName == "output" {
				flag = ctxCmd.InheritedFlags().Lookup(tt.flagName)
			} else {
				flag = ctxCmd.Flags().Lookup(tt.flagName)
			}
			if flag == nil {
				t.Errorf("expected %q flag on context command", tt.flagName)
				return
			}
			if flag.DefValue != tt.defaultValue {
				t.Errorf("expected default %q for flag %q, got %q", tt.defaultValue, tt.flagName, flag.DefValue)
			}
		})
	}
}

func TestContextOutputFormatDefaultIsMarkdown(t *testing.T) {
	if got := contextOutputFormat(contextCmd); got != "md" {
		t.Errorf("expected default context output format 'md', got %q", got)
	}
}

func TestContextBypassesPersistentPreRunE(t *testing.T) {
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "context" {
			err := rootCmd.PersistentPreRunE(cmd, nil)
			if err != nil {
				t.Errorf("context command should bypass PersistentPreRunE, got error: %v", err)
			}
			return
		}
	}
	t.Error("context command not found")
}

func TestEmitContextEmptyJSON(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	emitContextEmpty("json")

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := strings.TrimSpace(buf.String())

	// Should be valid JSON.
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("emitContextEmpty json produced invalid JSON: %v\nOutput: %s", err, output)
	}

	if parsed["query"] != "" {
		t.Errorf("expected empty query, got %v", parsed["query"])
	}
	if parsed["budget"] != float64(0) {
		t.Errorf("expected budget 0, got %v", parsed["budget"])
	}
	if parsed["used_tokens"] != float64(0) {
		t.Errorf("expected used_tokens 0, got %v", parsed["used_tokens"])
	}
	pieces, ok := parsed["pieces"].([]interface{})
	if !ok {
		t.Fatalf("expected pieces array, got %T", parsed["pieces"])
	}
	if len(pieces) != 0 {
		t.Errorf("expected empty pieces array, got %d items", len(pieces))
	}
}

func TestEmitContextEmptyMD(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	emitContextEmpty("md")

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := strings.TrimSpace(buf.String())

	if output != "" {
		t.Errorf("expected empty output for md format, got %q", output)
	}
}

func TestEstimateContextTokens(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"hello", 1},         // 1 word * 4 / 3 = 1
		{"one two three", 4}, // 3 words * 4 / 3 = 4
		{"a b c d e f", 8},   // 6 words * 4 / 3 = 8
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := estimateContextTokens(tt.input)
			if got != tt.expected {
				t.Errorf("estimateContextTokens(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}
