package db

import (
	"testing"
)

func seedRelationEntities(t *testing.T, database *DB, names ...string) map[string]string {
	t.Helper()
	ids := make(map[string]string)
	for _, name := range names {
		e := &Entity{ID: "ent-" + name, Name: name, Type: "person"}
		if err := database.SaveEntity(e); err != nil {
			t.Fatalf("seed entity %s: %v", name, err)
		}
		ids[name] = e.ID
	}
	return ids
}

func TestSaveEntityRelation_RoundTrip(t *testing.T) {
	database := newTestDB(t)
	ids := seedRelationEntities(t, database, "Alice", "Bob")

	rel := &EntityRelation{FromEntityID: ids["Alice"], ToEntityID: ids["Bob"], RelationType: "works-with", CreatedBy: "tester"}
	if err := database.SaveEntityRelation(rel); err != nil {
		t.Fatalf("SaveEntityRelation: %v", err)
	}

	out, err := database.OutgoingRelations(ids["Alice"])
	if err != nil {
		t.Fatalf("OutgoingRelations: %v", err)
	}
	if len(out) != 1 || out[0].ToEntityID != ids["Bob"] || out[0].RelationType != "works-with" {
		t.Fatalf("unexpected outgoing relations: %+v", out)
	}

	in, err := database.IncomingRelations(ids["Bob"])
	if err != nil {
		t.Fatalf("IncomingRelations: %v", err)
	}
	if len(in) != 1 || in[0].FromEntityID != ids["Alice"] {
		t.Fatalf("unexpected incoming relations: %+v", in)
	}

	// Directed: Bob has no outgoing relation to Alice.
	bobOut, err := database.OutgoingRelations(ids["Bob"])
	if err != nil {
		t.Fatalf("OutgoingRelations(Bob): %v", err)
	}
	if len(bobOut) != 0 {
		t.Fatalf("expected 0 outgoing relations for Bob, got %d", len(bobOut))
	}
}

func TestSaveEntityRelation_IdempotentOnDuplicate(t *testing.T) {
	database := newTestDB(t)
	ids := seedRelationEntities(t, database, "Alice", "Bob")

	rel := &EntityRelation{FromEntityID: ids["Alice"], ToEntityID: ids["Bob"], RelationType: "works-with"}
	if err := database.SaveEntityRelation(rel); err != nil {
		t.Fatalf("first save: %v", err)
	}
	if err := database.SaveEntityRelation(rel); err != nil {
		t.Fatalf("second save: %v", err)
	}

	out, err := database.OutgoingRelations(ids["Alice"])
	if err != nil {
		t.Fatalf("OutgoingRelations: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected exactly 1 relation after duplicate save, got %d", len(out))
	}
}

func TestSaveEntityRelation_DistinctRelationTypesCoexist(t *testing.T) {
	database := newTestDB(t)
	ids := seedRelationEntities(t, database, "Alice", "Bob")

	if err := database.SaveEntityRelation(&EntityRelation{FromEntityID: ids["Alice"], ToEntityID: ids["Bob"], RelationType: "works-with"}); err != nil {
		t.Fatalf("save works-with: %v", err)
	}
	if err := database.SaveEntityRelation(&EntityRelation{FromEntityID: ids["Alice"], ToEntityID: ids["Bob"], RelationType: "manages"}); err != nil {
		t.Fatalf("save manages: %v", err)
	}

	out, err := database.OutgoingRelations(ids["Alice"])
	if err != nil {
		t.Fatalf("OutgoingRelations: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 distinct relation types, got %d", len(out))
	}
}

func TestDeleteEntityRelation(t *testing.T) {
	database := newTestDB(t)
	ids := seedRelationEntities(t, database, "Alice", "Bob")

	rel := &EntityRelation{FromEntityID: ids["Alice"], ToEntityID: ids["Bob"], RelationType: "works-with"}
	if err := database.SaveEntityRelation(rel); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := database.DeleteEntityRelation(ids["Alice"], ids["Bob"], "works-with"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	out, err := database.OutgoingRelations(ids["Alice"])
	if err != nil {
		t.Fatalf("OutgoingRelations: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected 0 relations after delete, got %d", len(out))
	}
}

func TestRelationsForEntity_CombinesBothDirections(t *testing.T) {
	database := newTestDB(t)
	ids := seedRelationEntities(t, database, "Alice", "Bob", "Carol")

	if err := database.SaveEntityRelation(&EntityRelation{FromEntityID: ids["Alice"], ToEntityID: ids["Bob"], RelationType: "works-with"}); err != nil {
		t.Fatalf("save Alice->Bob: %v", err)
	}
	if err := database.SaveEntityRelation(&EntityRelation{FromEntityID: ids["Carol"], ToEntityID: ids["Bob"], RelationType: "reports-to"}); err != nil {
		t.Fatalf("save Carol->Bob: %v", err)
	}

	all, err := database.RelationsForEntity(ids["Bob"])
	if err != nil {
		t.Fatalf("RelationsForEntity: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 relations touching Bob, got %d", len(all))
	}
}

func TestEntityRelation_CascadesOnEntityDelete(t *testing.T) {
	database := newTestDB(t)
	ids := seedRelationEntities(t, database, "Alice", "Bob")

	if err := database.SaveEntityRelation(&EntityRelation{FromEntityID: ids["Alice"], ToEntityID: ids["Bob"], RelationType: "works-with"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := database.DeleteEntity(ids["Alice"]); err != nil {
		t.Fatalf("delete entity: %v", err)
	}

	in, err := database.IncomingRelations(ids["Bob"])
	if err != nil {
		t.Fatalf("IncomingRelations: %v", err)
	}
	if len(in) != 0 {
		t.Fatalf("expected relation to be cascade-deleted with its entity, got %d remaining", len(in))
	}
}
