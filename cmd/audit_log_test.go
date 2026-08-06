package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/db"
)

// TestSetCommandWritesAuditEvent verifies the CLI set command records a "set"
// audit row with the correct scope and actor, and that memory content never
// reaches the audit detail.
func TestSetCommandWritesAuditEvent(t *testing.T) {
	database := helperTestDB(t)
	cfg := config.Defaults()
	cfg.Retention.AuditLogEnabled = true
	SetConfig(cfg)
	SetDB(database)
	t.Cleanup(func() { database.SetAuditLogEnabled(true) })

	if err := setCmd.Flags().Set("value", "audit repro fact"); err != nil {
		t.Fatal(err)
	}
	if err := setCmd.Flags().Set("scope", "global"); err != nil {
		t.Fatal(err)
	}
	if err := setCmd.Flags().Set("author", "audit-tester"); err != nil {
		t.Fatal(err)
	}
	if err := setCmd.RunE(setCmd, nil); err != nil {
		t.Fatalf("setCmd.RunE failed: %v", err)
	}

	events, err := database.GetAuditLogs("set", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("expected a set audit event from the CLI set command")
	}
	e := events[0]
	if e.Actor != "audit-tester" {
		t.Errorf("actor = %q, want %q", e.Actor, "audit-tester")
	}
	if e.Scope != "global" {
		t.Errorf("scope = %q, want %q", e.Scope, "global")
	}
	if e.MemoryID == "" {
		t.Error("expected a non-empty memory_id on the set audit event")
	}
	if strings.Contains(e.Detail, "audit repro fact") {
		t.Errorf("memory content leaked into audit detail: %q", e.Detail)
	}
}

// TestSetCommandAuditDisabled verifies that audit_log_enabled=false makes the
// CLI set command write no audit rows while the memory is still stored.
func TestSetCommandAuditDisabled(t *testing.T) {
	database := helperTestDB(t)
	cfg := config.Defaults()
	cfg.Retention.AuditLogEnabled = false
	SetConfig(cfg)
	SetDB(database)
	t.Cleanup(func() { database.SetAuditLogEnabled(true) })

	if err := setCmd.Flags().Set("value", "disabled audit fact"); err != nil {
		t.Fatal(err)
	}
	if err := setCmd.Flags().Set("scope", "global"); err != nil {
		t.Fatal(err)
	}
	if err := setCmd.RunE(setCmd, nil); err != nil {
		t.Fatalf("setCmd.RunE failed: %v", err)
	}

	events, err := database.GetAuditLogs("", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("expected no audit rows when audit_log_enabled=false, got %d", len(events))
	}
}

// TestDeleteCommandWritesAuditEvent verifies the CLI delete command records a
// "delete" audit row carrying the memory's scope, session and recorded
// creator.
func TestDeleteCommandWritesAuditEvent(t *testing.T) {
	database := helperTestDB(t)
	cfg := config.Defaults()
	SetConfig(cfg)
	SetDB(database)
	t.Cleanup(func() { database.SetAuditLogEnabled(true) })

	m := &db.Memory{
		ID:             "11111111-1111-1111-1111-111111111111",
		Content:        "doomed fact",
		Scope:          "global",
		Metadata:       map[string]string{},
		CreatedBy:      "cli:creator",
		CreatedSession: "sess-del",
	}
	if err := database.SaveMemory(m); err != nil {
		t.Fatal(err)
	}

	if err := deleteCmd.RunE(deleteCmd, []string{m.ID}); err != nil {
		t.Fatalf("deleteCmd.RunE failed: %v", err)
	}

	events, err := database.GetAuditLogs("delete", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 delete audit event, got %d", len(events))
	}
	e := events[0]
	if e.MemoryID != m.ID {
		t.Errorf("memory_id = %q, want %q", e.MemoryID, m.ID)
	}
	if e.Scope != "global" {
		t.Errorf("scope = %q, want %q", e.Scope, "global")
	}
	if e.Session != "sess-del" {
		t.Errorf("session = %q, want %q", e.Session, "sess-del")
	}
	if e.Actor != "cli:creator" {
		t.Errorf("actor = %q, want %q", e.Actor, "cli:creator")
	}
}

// TestPurgeCommandWritesAuditEventsAndPrunes verifies the CLI purge command
// records a "purge" audit event with the CLI operator as actor and applies
// the configured audit_retention window to stale audit rows.
func TestPurgeCommandWritesAuditEventsAndPrunes(t *testing.T) {
	database := helperTestDB(t)
	cfg := config.Defaults()
	cfg.Retention.AuditRetention = "1h"
	SetConfig(cfg)
	SetDB(database)
	t.Cleanup(func() { database.SetAuditLogEnabled(true) })

	m := &db.Memory{
		ID:        "22222222-2222-2222-2222-222222222222",
		Content:   "purge me",
		Scope:     "user",
		Metadata:  map[string]string{},
		CreatedBy: "cli:creator",
	}
	if err := database.SaveMemory(m); err != nil {
		t.Fatal(err)
	}
	// Backdate the resulting "set" audit row so the retention prune must
	// remove it.
	if _, err := database.Conn().Exec("UPDATE audit_log SET created_at = ?", time.Now().UTC().Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}

	if err := purgeCmd.Flags().Set("scope", "user"); err != nil {
		t.Fatal(err)
	}
	if err := purgeCmd.RunE(purgeCmd, nil); err != nil {
		t.Fatalf("purgeCmd.RunE failed: %v", err)
	}

	events, err := database.GetAuditLogs("purge", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 purge audit event, got %d", len(events))
	}
	e := events[0]
	if e.Scope != "user" {
		t.Errorf("scope = %q, want %q", e.Scope, "user")
	}
	if !strings.HasPrefix(e.Actor, "cli:") {
		t.Errorf("actor = %q, want a cli: prefixed identity", e.Actor)
	}
	if e.Detail != "purged=1" {
		t.Errorf("detail = %q, want %q", e.Detail, "purged=1")
	}

	// The backdated "set" row must be pruned by the retention window; the
	// fresh "purge" row survives.
	all, err := database.GetAuditLogs("", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("expected only the fresh purge row after retention pruning, got %d rows", len(all))
	}
	if all[0].Action != "purge" {
		t.Errorf("remaining row action = %q, want %q", all[0].Action, "purge")
	}
}
