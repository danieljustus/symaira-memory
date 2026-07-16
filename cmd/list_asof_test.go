package cmd

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/db"
)

func TestListCmd_AsOf_ReturnsHistoricalVersion(t *testing.T) {
	database := helperTestDB(t)
	SetConfig(config.Defaults())
	SetDB(database)

	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(1 * time.Hour)

	old := &db.Memory{ID: "list-asof-old", Content: "old fact", Scope: "global", Metadata: map[string]string{}, ValidFrom: &t0, ValidTo: &t1, SupersededBy: "list-asof-new"}
	newer := &db.Memory{ID: "list-asof-new", Content: "new fact", Scope: "global", Metadata: map[string]string{}, ValidFrom: &t1}
	for _, m := range []*db.Memory{old, newer} {
		if err := database.SaveMemory(m); err != nil {
			t.Fatalf("save %s: %v", m.ID, err)
		}
	}

	if err := listCmd.Flags().Set("as-of", t0.Add(30*time.Minute).Format(time.RFC3339)); err != nil {
		t.Fatalf("failed to set as-of flag: %v", err)
	}
	oldOutput := outputFormat
	outputFormat = "json"
	defer func() {
		outputFormat = oldOutput
		listCmd.Flags().Set("as-of", "")
	}()

	output := captureCmdOutput(func() {
		if err := listCmd.RunE(listCmd, nil); err != nil {
			t.Fatalf("listCmd.RunE returned error: %v", err)
		}
	})

	var mems []*db.Memory
	if err := json.Unmarshal([]byte(output), &mems); err != nil {
		t.Fatalf("list --as-of output is not valid JSON: %v\noutput: %s", err, output)
	}
	if len(mems) != 1 || mems[0].ID != "list-asof-old" {
		t.Fatalf("expected the historical version 'list-asof-old', got %+v", mems)
	}
}

func TestListCmd_AsOf_RejectsEntityCombination(t *testing.T) {
	database := helperTestDB(t)
	SetConfig(config.Defaults())
	SetDB(database)

	if err := listCmd.Flags().Set("as-of", time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("failed to set as-of flag: %v", err)
	}
	if err := listCmd.Flags().Set("entity", "Someone"); err != nil {
		t.Fatalf("failed to set entity flag: %v", err)
	}
	defer func() {
		listCmd.Flags().Set("as-of", "")
		listCmd.Flags().Set("entity", "")
	}()

	err := listCmd.RunE(listCmd, nil)
	if err == nil {
		t.Fatal("expected an error when combining --as-of and --entity")
	}
}
