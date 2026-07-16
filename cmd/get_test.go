package cmd

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/danieljustus/symaira-corekit/evidencekit"
	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/db"
)

func TestGetCommandWithEvidenceJSON(t *testing.T) {
	database := helperTestDB(t)
	SetConfig(config.Defaults())
	SetDB(database)

	m := &db.Memory{
		ID:                  "mem-with-evidence",
		Content:             "User prefers dark mode",
		Scope:               "global",
		Metadata:            map[string]string{},
		CreatedAt:           time.Now().UTC(),
		ConsolidationStatus: "raw",
	}
	if err := database.SaveMemory(m); err != nil {
		t.Fatalf("failed to save memory: %v", err)
	}
	if err := database.SaveMemoryEvidence(m.ID, []evidencekit.Extraction{
		{
			EvidenceText:    "I always use dark mode",
			Span:            evidencekit.Span{Start: 0, End: 23},
			AlignmentStatus: evidencekit.AlignmentExact,
		},
	}); err != nil {
		t.Fatalf("failed to save evidence: %v", err)
	}

	oldOutput := outputFormat
	outputFormat = "json"
	if err := getCmd.Flags().Set("with-evidence", "true"); err != nil {
		t.Fatalf("failed to set with-evidence flag: %v", err)
	}
	defer func() {
		outputFormat = oldOutput
		getCmd.Flags().Set("with-evidence", "false")
	}()

	output := captureCmdOutput(func() {
		if err := getCmd.RunE(getCmd, []string{m.ID}); err != nil {
			t.Fatalf("getCmd.RunE returned error: %v", err)
		}
	})

	var parsed db.Memory
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("get --with-evidence output is not valid JSON: %v\noutput: %s", err, output)
	}
	if len(parsed.Evidence) != 1 {
		t.Fatalf("expected 1 evidence row in output, got %d", len(parsed.Evidence))
	}
	if parsed.Evidence[0].EvidenceText != "I always use dark mode" {
		t.Errorf("unexpected evidence text: %q", parsed.Evidence[0].EvidenceText)
	}
}

func TestGetCommandWithoutEvidenceFlagOmitsEvidence(t *testing.T) {
	database := helperTestDB(t)
	SetConfig(config.Defaults())
	SetDB(database)

	m := &db.Memory{
		ID:                  "mem-no-evidence-flag",
		Content:             "User prefers dark mode",
		Scope:               "global",
		Metadata:            map[string]string{},
		CreatedAt:           time.Now().UTC(),
		ConsolidationStatus: "raw",
	}
	if err := database.SaveMemory(m); err != nil {
		t.Fatalf("failed to save memory: %v", err)
	}
	if err := database.SaveMemoryEvidence(m.ID, []evidencekit.Extraction{
		{EvidenceText: "I always use dark mode", Span: evidencekit.Span{Start: 0, End: 23}, AlignmentStatus: evidencekit.AlignmentExact},
	}); err != nil {
		t.Fatalf("failed to save evidence: %v", err)
	}

	oldOutput := outputFormat
	outputFormat = "json"
	defer func() { outputFormat = oldOutput }()

	output := captureCmdOutput(func() {
		if err := getCmd.RunE(getCmd, []string{m.ID}); err != nil {
			t.Fatalf("getCmd.RunE returned error: %v", err)
		}
	})

	if strings.Contains(output, "\"evidence\"") {
		t.Errorf("expected no evidence field without --with-evidence, got: %s", output)
	}
}
