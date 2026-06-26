package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/danieljustus/symaira-memory/internal/config"
)

func TestSetCommandOutputsJSON(t *testing.T) {
	oldHome := os.Getenv("HOME")
	tempDir, err := os.MkdirTemp("", "symmemory-home-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	database := helperTestDB(t)
	cfg := config.Defaults()
	SetConfig(cfg)
	SetDB(database)

	if err := setCmd.Flags().Set("value", "JSON_OUTPUT_REPRO"); err != nil {
		t.Fatalf("failed to set value flag: %v", err)
	}
	if err := setCmd.Flags().Set("scope", "global"); err != nil {
		t.Fatalf("failed to set scope flag: %v", err)
	}
	if err := setCmd.Flags().Set("author", "repro"); err != nil {
		t.Fatalf("failed to set author flag: %v", err)
	}
	oldOutput := outputFormat
	outputFormat = "json"
	defer func() { outputFormat = oldOutput }()

	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	if err := setCmd.RunE(setCmd, nil); err != nil {
		t.Fatalf("setCmd.RunE returned error: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout
	buf.ReadFrom(r)

	first := buf.Bytes()[0]
	if first != '{' {
		t.Fatalf("expected JSON object to start with '{', got %q", string(first))
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("set output is not valid JSON: %v\noutput: %s", err, buf.String())
	}
	if parsed["content"] != "JSON_OUTPUT_REPRO" {
		t.Errorf("expected content JSON_OUTPUT_REPRO, got %v", parsed["content"])
	}
	if parsed["scope"] != "global" {
		t.Errorf("expected scope global, got %v", parsed["scope"])
	}
	if parsed["author"] != "repro" {
		t.Errorf("expected author repro, got %v", parsed["author"])
	}
}

func TestSetCommandOutputsHumanTextByDefault(t *testing.T) {
	oldHome := os.Getenv("HOME")
	tempDir, err := os.MkdirTemp("", "symmemory-home-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	database := helperTestDB(t)
	cfg := config.Defaults()
	SetConfig(cfg)
	SetDB(database)

	if err := setCmd.Flags().Set("value", "HUMAN_OUTPUT_REPRO"); err != nil {
		t.Fatalf("failed to set value flag: %v", err)
	}
	if err := setCmd.Flags().Set("scope", "global"); err != nil {
		t.Fatalf("failed to set scope flag: %v", err)
	}
	oldOutput := outputFormat
	outputFormat = "table"
	defer func() { outputFormat = oldOutput }()

	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	if err := setCmd.RunE(setCmd, nil); err != nil {
		t.Fatalf("setCmd.RunE returned error: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout
	buf.ReadFrom(r)

	if !strings.Contains(buf.String(), "Memory saved successfully") {
		t.Errorf("expected human success message, got %q", buf.String())
	}
}
