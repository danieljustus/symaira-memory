package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/db"
)

func TestEntityResolveCmd_JSONOutput(t *testing.T) {
	database := helperTestDB(t)
	SetConfig(config.Defaults())
	SetDB(database)

	alice := &db.Entity{ID: "cli-resolve-alice", Name: "Alice", Type: "person", Aliases: []string{"Ali"}}
	if err := database.SaveEntity(alice); err != nil {
		t.Fatalf("save Alice: %v", err)
	}

	entityResolveType = ""
	entityResolveAliases = ""
	entityResolveLimit = 10
	outputFormat = "json"
	defer func() {
		entityResolveType = ""
		entityResolveAliases = ""
		entityResolveLimit = 10
		outputFormat = "table"
	}()

	output := captureCmdOutput(func() {
		if err := entityResolveCmd.RunE(entityResolveCmd, []string{"Alice"}); err != nil {
			t.Fatalf("entityResolveCmd.RunE returned error: %v", err)
		}
	})

	var parsed []db.EntityCandidate
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("entity resolve --output json output is not valid JSON: %v\noutput: %s", err, output)
	}
	if len(parsed) != 1 || parsed[0].EntityID != "cli-resolve-alice" {
		t.Fatalf("expected exactly cli-resolve-alice, got %+v", parsed)
	}
	if parsed[0].MatchKind != "exact_name" {
		t.Errorf("expected exact_name match kind, got %s", parsed[0].MatchKind)
	}
}

func TestEntityResolveCmd_TypeFlagFiltersStrictly(t *testing.T) {
	database := helperTestDB(t)
	SetConfig(config.Defaults())
	SetDB(database)

	if err := database.SaveEntity(&db.Entity{ID: "cli-resolve-person", Name: "SamPerson", Aliases: []string{"Sam"}, Type: "person"}); err != nil {
		t.Fatalf("save entity: %v", err)
	}
	if err := database.SaveEntity(&db.Entity{ID: "cli-resolve-project", Name: "SamProject", Aliases: []string{"Sam"}, Type: "project"}); err != nil {
		t.Fatalf("save entity: %v", err)
	}

	entityResolveType = "project"
	entityResolveAliases = ""
	entityResolveLimit = 10
	outputFormat = "json"
	defer func() {
		entityResolveType = ""
		outputFormat = "table"
	}()

	output := captureCmdOutput(func() {
		if err := entityResolveCmd.RunE(entityResolveCmd, []string{"Sam"}); err != nil {
			t.Fatalf("entityResolveCmd.RunE returned error: %v", err)
		}
	})

	var parsed []db.EntityCandidate
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}
	if len(parsed) != 1 || parsed[0].EntityID != "cli-resolve-project" {
		t.Fatalf("expected only the project-typed entity, got %+v", parsed)
	}
}

func TestEntityResolveCmd_AliasesFlagIsParsedAndHinted(t *testing.T) {
	database := helperTestDB(t)
	SetConfig(config.Defaults())
	SetDB(database)

	if err := database.SaveEntity(&db.Entity{ID: "cli-resolve-hint", Name: "Guillaume Charhon", Aliases: []string{"GC"}, Type: "person"}); err != nil {
		t.Fatalf("save entity: %v", err)
	}

	entityResolveType = ""
	entityResolveAliases = "GC, someone@example.com"
	entityResolveLimit = 10
	outputFormat = "json"
	defer func() {
		entityResolveAliases = ""
		outputFormat = "table"
	}()

	output := captureCmdOutput(func() {
		if err := entityResolveCmd.RunE(entityResolveCmd, []string{"nobody matches this text"}); err != nil {
			t.Fatalf("entityResolveCmd.RunE returned error: %v", err)
		}
	})

	var parsed []db.EntityCandidate
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}
	if len(parsed) != 1 || parsed[0].EntityID != "cli-resolve-hint" || parsed[0].MatchKind != "exact_alias" {
		t.Fatalf("expected exact_alias match via --aliases hint, got %+v", parsed)
	}
}

func TestEntityResolveCmd_TextOutputNoMatches(t *testing.T) {
	database := helperTestDB(t)
	SetConfig(config.Defaults())
	SetDB(database)

	entityResolveType = ""
	entityResolveAliases = ""
	entityResolveLimit = 10
	outputFormat = "table"

	output := captureCmdOutput(func() {
		if err := entityResolveCmd.RunE(entityResolveCmd, []string{"nothing here"}); err != nil {
			t.Fatalf("entityResolveCmd.RunE returned error: %v", err)
		}
	})

	if !strings.Contains(output, "No matching entities found.") {
		t.Errorf("expected no-match message, got %q", output)
	}
}
