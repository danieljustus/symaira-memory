package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danieljustus/symaira-memory/internal/config"
)

func resetImportFlags(t *testing.T) {
	t.Helper()
	for _, name := range []string{"list", "dry-run", "all"} {
		_ = importSessionsCmd.Flags().Set(name, "false")
	}
	_ = importSessionsCmd.Flags().Set("tool", "")
}

func TestImportListOutputsJSON(t *testing.T) {
	defer resetImportFlags(t)
	oldHome := os.Getenv("HOME")
	tempDir, err := os.MkdirTemp("", "symmemory-import-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	SetConfig(config.Defaults())

	if err := importSessionsCmd.Flags().Set("list", "true"); err != nil {
		t.Fatalf("failed to set list flag: %v", err)
	}
	oldOutput := outputFormat
	outputFormat = "json"
	defer func() { outputFormat = oldOutput }()

	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	importSessionsCmd.RunE(importSessionsCmd, nil)

	w.Close()
	os.Stdout = oldStdout
	buf.ReadFrom(r)

	first := buf.Bytes()[0]
	if first != '[' {
		t.Fatalf("expected JSON array to start with '[', got %q", string(first))
	}

	var parsed []map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("import --list output is not valid JSON: %v\noutput: %s", err, buf.String())
	}
	if len(parsed) == 0 {
		t.Fatal("expected at least one tool status in JSON output")
	}
	if parsed[0]["tool"] == "" {
		t.Errorf("expected first entry to have a tool name, got %v", parsed[0])
	}
}

func TestImportListOutputsHumanTextByDefault(t *testing.T) {
	defer resetImportFlags(t)
	oldHome := os.Getenv("HOME")
	tempDir, err := os.MkdirTemp("", "symmemory-import-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	SetConfig(config.Defaults())

	if err := importSessionsCmd.Flags().Set("list", "true"); err != nil {
		t.Fatalf("failed to set list flag: %v", err)
	}
	oldOutput := outputFormat
	outputFormat = "table"
	defer func() { outputFormat = oldOutput }()

	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	importSessionsCmd.RunE(importSessionsCmd, nil)

	w.Close()
	os.Stdout = oldStdout
	buf.ReadFrom(r)

	if !strings.Contains(buf.String(), "not configured") {
		t.Errorf("expected human status text, got %q", buf.String())
	}
}

func TestImportDryRunOutputsJSON(t *testing.T) {
	defer resetImportFlags(t)
	oldHome := os.Getenv("HOME")
	tempDir, err := os.MkdirTemp("", "symmemory-import-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	exportPath := filepath.Join(tempDir, "openmemory-export.json")
	data := []byte(`{"memories":[{"id":"mem-1","content":"hello from openmemory","created_at":"2026-06-22T12:00:00Z"}]}`)
	if err := os.WriteFile(exportPath, data, 0644); err != nil {
		t.Fatalf("failed to write export: %v", err)
	}

	cfg := config.Defaults()
	cfg.Import.Tools = map[string]config.ImportToolConfig{}
	cfg.Import.Tools["openmemory"] = config.ImportToolConfig{Path: exportPath}
	SetConfig(cfg)
	SetDB(helperTestDB(t))

	if err := importSessionsCmd.Flags().Set("tool", "openmemory"); err != nil {
		t.Fatalf("failed to set tool flag: %v", err)
	}
	if err := importSessionsCmd.Flags().Set("dry-run", "true"); err != nil {
		t.Fatalf("failed to set dry-run flag: %v", err)
	}
	oldOutput := outputFormat
	outputFormat = "json"
	defer func() { outputFormat = oldOutput }()

	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	importSessionsCmd.RunE(importSessionsCmd, nil)

	w.Close()
	os.Stdout = oldStdout
	buf.ReadFrom(r)

	first := buf.Bytes()[0]
	if first != '[' {
		t.Fatalf("expected JSON array to start with '[', got %q", string(first))
	}

	var parsed []map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("import --dry-run output is not valid JSON: %v\noutput: %s", err, buf.String())
	}
	if len(parsed) != 1 {
		t.Fatalf("expected 1 result, got %d", len(parsed))
	}
	if parsed[0]["Tool"] != "openmemory" {
		t.Errorf("expected tool openmemory, got %v", parsed[0]["Tool"])
	}
}
