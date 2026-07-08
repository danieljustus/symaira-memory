package cmd

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/db"
)

func TestEntityNeighborsCmd_JSONMatchesMCPShape(t *testing.T) {
	database := helperTestDB(t)
	SetConfig(config.Defaults())
	SetDB(database)

	alice := &db.Entity{ID: "cli-alice", Name: "Alice", Type: "person", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	bob := &db.Entity{ID: "cli-bob", Name: "Bob", Type: "person", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := database.SaveEntity(alice); err != nil {
		t.Fatalf("save Alice: %v", err)
	}
	if err := database.SaveEntity(bob); err != nil {
		t.Fatalf("save Bob: %v", err)
	}
	if err := database.SaveEntityRelation(&db.EntityRelation{FromEntityID: alice.ID, ToEntityID: bob.ID, RelationType: "works-with"}); err != nil {
		t.Fatalf("save relation: %v", err)
	}

	if err := entityNeighborsCmd.Flags().Set("depth", "1"); err != nil {
		t.Fatalf("failed to set depth flag: %v", err)
	}
	outputFormat = "json"
	defer func() {
		entityNeighborsCmd.Flags().Set("depth", "1")
		outputFormat = "table"
	}()

	output := captureCmdOutput(func() {
		if err := entityNeighborsCmd.RunE(entityNeighborsCmd, []string{"Alice"}); err != nil {
			t.Fatalf("entityNeighborsCmd.RunE returned error: %v", err)
		}
	})

	var parsed entityNeighborsResult
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("entity neighbors --output json output is not valid JSON: %v\noutput: %s", err, output)
	}
	if len(parsed.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(parsed.Nodes))
	}
	if len(parsed.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(parsed.Edges))
	}
	if !strings.Contains(output, "works-with") {
		t.Errorf("expected relation type in output, got %s", output)
	}
}
