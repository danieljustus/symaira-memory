package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/db"
)

// reviewTestSetup builds a temp DB with one staged candidate and one live
// memory, mirroring the CLI review surface (#485).
func reviewTestSetup(t *testing.T) *db.DB {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "symmemory-review-test-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tempDir) })
	setTestHome(t, tempDir)

	database := helperTestDB(t)
	cfg := config.Defaults()
	SetConfig(cfg)
	SetDB(database)

	live := &db.Memory{
		ID:      "live-1",
		Content: "live memory",
		Scope:   "global",
		Kind:    db.KindUser,
	}
	staged := &db.Memory{
		ID:           "staged-1",
		Content:      "staged candidate",
		Scope:        "global",
		Kind:         db.KindProject,
		ReviewStatus: db.ReviewStaged,
	}
	for _, m := range []*db.Memory{live, staged} {
		if err := database.SaveMemory(m); err != nil {
			t.Fatalf("save: %v", err)
		}
	}
	return database
}

func TestReviewListsStagedCandidates(t *testing.T) {
	reviewTestSetup(t)
	out := captureStdoutOfReview(t)
	if !strings.Contains(out, "staged-1") || !strings.Contains(out, "staged candidate") {
		t.Fatalf("review output missing staged candidate:\n%s", out)
	}
	if strings.Contains(out, "live-1") {
		t.Fatalf("review output must not list live memories:\n%s", out)
	}
}

func TestReviewPromoteMakesRetrievable(t *testing.T) {
	database := reviewTestSetup(t)
	if err := reviewCmd.Flags().Set("promote", "staged-1"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reviewCmd.Flags().Set("promote", "") })

	out := captureStdoutOfReview(t)
	if !strings.Contains(out, "promoted") {
		t.Fatalf("promote output missing confirmation:\n%s", out)
	}

	m, err := database.GetMemory("staged-1")
	if err != nil {
		t.Fatal(err)
	}
	if m.ReviewStatus != db.ReviewApproved {
		t.Fatalf("review status after promote = %q, want approved", m.ReviewStatus)
	}
}

func TestReviewRejectRemovesCandidate(t *testing.T) {
	database := reviewTestSetup(t)
	if err := reviewCmd.Flags().Set("reject", "staged-1"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reviewCmd.Flags().Set("reject", "") })

	out := captureStdoutOfReview(t)
	if !strings.Contains(out, "rejected") {
		t.Fatalf("reject output missing confirmation:\n%s", out)
	}

	m, err := database.GetMemory("staged-1")
	if err != nil {
		t.Fatal(err)
	}
	if m != nil {
		t.Fatalf("rejected candidate still exists: %+v", m)
	}
}

func TestReviewRejectsLiveMemoryRefused(t *testing.T) {
	reviewTestSetup(t)
	if err := reviewCmd.Flags().Set("reject", "live-1"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reviewCmd.Flags().Set("reject", "") })

	err := reviewCmd.RunE(reviewCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "not a staged candidate") {
		t.Fatalf("rejecting a live memory must fail with a clear error, got %v", err)
	}
}

// captureStdoutOfReview runs the review command and returns its stdout.
func captureStdoutOfReview(t *testing.T) string {
	t.Helper()
	return captureCmdOutput(func() {
		if err := reviewCmd.RunE(reviewCmd, nil); err != nil {
			t.Errorf("review command error: %v", err)
		}
	})
}
