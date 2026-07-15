package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/db"
)

func resetEntityRelateFlags() {
	entityRelateFromID = ""
	entityRelateToID = ""
	entityRelateRelationFlag = ""
	entityRelateSource = ""
	entityRelateSourceRef = ""
	entityRelateVerification = ""
	entityRelateEvidenceJSON = ""
	entityUnrelateRelationID = ""
	outputFormat = "table"
}

func TestEntityRelateCmd_IDBasedWithProvenance_JSONOutput(t *testing.T) {
	database := helperTestDB(t)
	SetConfig(config.Defaults())
	SetDB(database)
	defer resetEntityRelateFlags()

	alice := &db.Entity{ID: "rel-alice", Name: "Alice", Type: "person"}
	meeting := &db.Entity{ID: "rel-meeting", Name: "Standup", Type: "event"}
	if err := database.SaveEntity(alice); err != nil {
		t.Fatalf("save Alice: %v", err)
	}
	if err := database.SaveEntity(meeting); err != nil {
		t.Fatalf("save meeting: %v", err)
	}

	entityRelateFromID = "rel-alice"
	entityRelateToID = "rel-meeting"
	entityRelateRelationFlag = "attended"
	entityRelateSource = "symdesk"
	entityRelateSourceRef = "meeting-123"
	entityRelateVerification = "verified"
	entityRelateEvidenceJSON = `{"source_doc_id":"doc-1","char_start":0,"char_end":10}`
	outputFormat = "json"

	output := captureCmdOutput(func() {
		if err := entityRelateCmd.RunE(entityRelateCmd, []string{}); err != nil {
			t.Fatalf("entityRelateCmd.RunE returned error: %v", err)
		}
	})

	var saved db.EntityRelation
	if err := json.Unmarshal([]byte(output), &saved); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}
	if saved.ID == "" || saved.Source != "symdesk" || saved.SourceRef != "meeting-123" || saved.Verification != "verified" {
		t.Fatalf("unexpected saved relation: %+v", saved)
	}

	fetched, err := database.GetEntityRelationByID(saved.ID)
	if err != nil {
		t.Fatalf("GetEntityRelationByID: %v", err)
	}
	if fetched == nil {
		t.Fatal("expected relation to be persisted")
	}
}

func TestEntityRelateCmd_RetryIsIdempotent(t *testing.T) {
	database := helperTestDB(t)
	SetConfig(config.Defaults())
	SetDB(database)
	defer resetEntityRelateFlags()

	if err := database.SaveEntity(&db.Entity{ID: "rel-a", Name: "A", Type: "person"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := database.SaveEntity(&db.Entity{ID: "rel-b", Name: "B", Type: "event"}); err != nil {
		t.Fatalf("save: %v", err)
	}

	entityRelateFromID = "rel-a"
	entityRelateToID = "rel-b"
	entityRelateRelationFlag = "attended"
	entityRelateSource = "symdesk"
	entityRelateSourceRef = "meeting-1"
	outputFormat = "json"

	run := func() db.EntityRelation {
		output := captureCmdOutput(func() {
			if err := entityRelateCmd.RunE(entityRelateCmd, []string{}); err != nil {
				t.Fatalf("entityRelateCmd.RunE returned error: %v", err)
			}
		})
		var saved db.EntityRelation
		if err := json.Unmarshal([]byte(output), &saved); err != nil {
			t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
		}
		return saved
	}

	first := run()
	second := run()
	if first.ID != second.ID {
		t.Fatalf("expected idempotent retry to return the same relation, got %s vs %s", first.ID, second.ID)
	}

	out, err := database.OutgoingRelations("rel-a")
	if err != nil {
		t.Fatalf("OutgoingRelations: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected exactly 1 relation after idempotent retry, got %d", len(out))
	}
}

func TestEntityRelateCmd_MixedIDAndPositionalArgsRejected(t *testing.T) {
	database := helperTestDB(t)
	SetConfig(config.Defaults())
	SetDB(database)
	defer resetEntityRelateFlags()

	entityRelateFromID = "some-id"
	entityRelateToID = "other-id"

	err := entityRelateCmd.RunE(entityRelateCmd, []string{"Alice", "works-with", "Bob"})
	if err == nil {
		t.Fatal("expected an error when mixing --from-id with positional args")
	}
}

func TestEntityUnrelateCmd_ByRelationID(t *testing.T) {
	database := helperTestDB(t)
	SetConfig(config.Defaults())
	SetDB(database)
	defer resetEntityRelateFlags()

	rel := &db.EntityRelation{FromEntityID: "u-a", ToEntityID: "u-b", RelationType: "works-with"}
	if err := database.SaveEntity(&db.Entity{ID: "u-a", Name: "UA", Type: "person"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := database.SaveEntity(&db.Entity{ID: "u-b", Name: "UB", Type: "person"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := database.SaveEntityRelation(rel); err != nil {
		t.Fatalf("save relation: %v", err)
	}

	entityUnrelateRelationID = rel.ID
	outputFormat = "json"

	output := captureCmdOutput(func() {
		if err := entityUnrelateCmd.RunE(entityUnrelateCmd, []string{}); err != nil {
			t.Fatalf("entityUnrelateCmd.RunE returned error: %v", err)
		}
	})
	if !strings.Contains(output, rel.ID) {
		t.Errorf("expected output to mention the removed relation ID, got %q", output)
	}

	fetched, err := database.GetEntityRelationByID(rel.ID)
	if err != nil {
		t.Fatalf("GetEntityRelationByID: %v", err)
	}
	if fetched != nil {
		t.Fatal("expected relation to be deleted")
	}
}

func TestEntityRelateCmd_VerifiedConflictReturnsError(t *testing.T) {
	database := helperTestDB(t)
	SetConfig(config.Defaults())
	SetDB(database)
	defer resetEntityRelateFlags()

	if err := database.SaveEntity(&db.Entity{ID: "vc-a", Name: "VA", Type: "person"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := database.SaveEntity(&db.Entity{ID: "vc-b", Name: "VB", Type: "event"}); err != nil {
		t.Fatalf("save: %v", err)
	}

	entityRelateFromID = "vc-a"
	entityRelateToID = "vc-b"
	entityRelateRelationFlag = "attended"
	entityRelateSource = "symdesk"
	entityRelateSourceRef = "meeting-1"
	entityRelateVerification = "verified"
	outputFormat = "json"

	if err := entityRelateCmd.RunE(entityRelateCmd, []string{}); err != nil {
		t.Fatalf("initial verified relate failed: %v", err)
	}

	entityRelateSource = "other-tool"
	entityRelateSourceRef = "different-ref"
	entityRelateVerification = "unverified"

	if err := entityRelateCmd.RunE(entityRelateCmd, []string{}); err == nil {
		t.Fatal("expected an error when a conflicting provenance write would overwrite a verified relation")
	}
}

func TestEntityRelateCmd_LegacyPositionalPathUnaffected(t *testing.T) {
	database := helperTestDB(t)
	SetConfig(config.Defaults())
	SetDB(database)
	defer resetEntityRelateFlags()

	if err := database.SaveEntity(&db.Entity{ID: "legacy-a", Name: "Alice", Type: "person"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := database.SaveEntity(&db.Entity{ID: "legacy-b", Name: "Bob", Type: "person"}); err != nil {
		t.Fatalf("save: %v", err)
	}

	output := captureCmdOutput(func() {
		if err := entityRelateCmd.RunE(entityRelateCmd, []string{"Alice", "works-with", "Bob"}); err != nil {
			t.Fatalf("entityRelateCmd.RunE returned error: %v", err)
		}
	})
	if !strings.Contains(output, "Related: Alice --works-with--> Bob") {
		t.Errorf("expected legacy text output unchanged, got %q", output)
	}
}
