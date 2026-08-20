package mcp

import (
	"strings"
	"testing"
	"time"

	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/db"
)

// ---------------------------------------------------------------------------
// Write-path governance (#485 staging, #486 semantic kind)
// ---------------------------------------------------------------------------

func TestMemorySetRequiresKind(t *testing.T) {
	s := helperServer(t)

	// Missing kind → refusal that names the buckets.
	res := callTool(s, "memory_set", map[string]interface{}{"content": "x"})
	text := getToolText(res)
	if !strings.Contains(text, "'kind' is required") {
		t.Fatalf("expected kind refusal, got %q", text)
	}
	for _, bucket := range []string{"user", "feedback", "project", "reference"} {
		if !strings.Contains(text, bucket) {
			t.Fatalf("kind refusal must name bucket %q, got %q", bucket, text)
		}
	}

	// Unclassifiable kind → refusal.
	res = callTool(s, "memory_set", map[string]interface{}{"content": "x", "kind": "banana"})
	text = getToolText(res)
	if !strings.Contains(text, "'kind' is required") {
		t.Fatalf("expected kind refusal for unknown kind, got %q", text)
	}

	// Synonym is accepted.
	res = callTool(s, "memory_set", map[string]interface{}{"content": "user likes tea", "kind": "preferences"})
	text = getToolText(res)
	if !strings.Contains(text, "Memory saved successfully") {
		t.Fatalf("synonym kind should be accepted, got %q", text)
	}
	id := memoryIDFromSetText(t, text)
	m, err := s.service.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if m.Kind != db.KindUser {
		t.Fatalf("kind = %q, want %q", m.Kind, db.KindUser)
	}
	if m.ReviewStatus != db.ReviewApproved {
		t.Fatalf("default review status = %q, want approved", m.ReviewStatus)
	}
}

func TestMemorySetStagedFlow(t *testing.T) {
	s := helperServer(t)

	setRes := callTool(s, "memory_set", map[string]interface{}{
		"content": "autonomous guess about the api",
		"kind":    "project",
		"staged":  true,
	})
	text := getToolText(setRes)
	if !strings.Contains(text, "staged as candidate") {
		t.Fatalf("expected staged confirmation, got %q", text)
	}
	id := memoryIDFromSetText(t, text)

	// The staged candidate is not retrievable.
	searchRes := callTool(s, "memory_search", map[string]interface{}{"query": "autonomous guess about the api", "limit": 5})
	if strings.Contains(getToolText(searchRes), "autonomous guess") {
		t.Fatalf("staged candidate leaked into search: %q", getToolText(searchRes))
	}

	// It shows up in the candidates queue.
	candRes := callTool(s, "memory_candidates", map[string]interface{}{})
	if !strings.Contains(getToolText(candRes), id) {
		t.Fatalf("candidates queue missing staged id %s: %q", id, getToolText(candRes))
	}

	// Promote → retrievable.
	promoteRes := callTool(s, "memory_promote", map[string]interface{}{"id": id})
	if !strings.Contains(getToolText(promoteRes), "promoted") {
		t.Fatalf("promote failed: %q", getToolText(promoteRes))
	}
	searchRes = callTool(s, "memory_search", map[string]interface{}{"query": "autonomous guess about the api", "limit": 5})
	if !strings.Contains(getToolText(searchRes), "autonomous guess") {
		t.Fatalf("promoted candidate missing from search: %q", getToolText(searchRes))
	}
}

func TestMemoryRejectRefusesLiveMemories(t *testing.T) {
	s := helperServer(t)

	setRes := callTool(s, "memory_set", map[string]interface{}{"content": "live fact", "kind": "user"})
	liveID := memoryIDFromSetText(t, getToolText(setRes))

	res := callTool(s, "memory_reject", map[string]interface{}{"id": liveID})
	if !strings.Contains(getToolText(res), "Failed to reject memory") {
		t.Fatalf("rejecting a live memory must fail, got %q", getToolText(res))
	}
}

func TestMemoryRejectRemovesCandidate(t *testing.T) {
	s := helperServer(t)

	setRes := callTool(s, "memory_set", map[string]interface{}{"content": "discard me", "kind": "reference", "staged": true})
	id := memoryIDFromSetText(t, getToolText(setRes))

	res := callTool(s, "memory_reject", map[string]interface{}{"id": id})
	if !strings.Contains(getToolText(res), "rejected") {
		t.Fatalf("reject failed: %q", getToolText(res))
	}

	m, err := s.service.Get(id)
	if err == nil && m != nil {
		t.Fatalf("rejected candidate still present: %+v", m)
	}
	if err == nil {
		t.Fatal("rejected candidate still resolvable")
	}
	if _, ok := err.(*NotFoundError); !ok {
		t.Fatalf("expected NotFoundError after reject, got %v", err)
	}
}

func TestMemorySetStageWritesByDefaultConfig(t *testing.T) {
	cfg := config.Defaults()
	cfg.Memory.StageWritesByDefault = true
	s := helperServerCfg(t, cfg)

	// No explicit staged flag → server default stages the write.
	res := callTool(s, "memory_set", map[string]interface{}{"content": "default staged fact", "kind": "user"})
	text := getToolText(res)
	if !strings.Contains(text, "staged as candidate") {
		t.Fatalf("stage_writes_by_default should stage writes, got %q", text)
	}
	id := memoryIDFromSetText(t, text)
	m, err := s.service.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if m.ReviewStatus != db.ReviewStaged {
		t.Fatalf("review status = %q, want staged", m.ReviewStatus)
	}

	// Explicit staged=false still wins.
	res = callTool(s, "memory_set", map[string]interface{}{"content": "explicit live fact", "kind": "user", "staged": false})
	text = getToolText(res)
	if !strings.Contains(text, "Memory saved successfully") {
		t.Fatalf("explicit staged=false must bypass the default, got %q", text)
	}
	id = memoryIDFromSetText(t, text)
	m, err = s.service.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if m.ReviewStatus != db.ReviewApproved {
		t.Fatalf("review status = %q, want approved", m.ReviewStatus)
	}
}

// ---------------------------------------------------------------------------
// Atomic Correction / Supersedes Tests (#556)
// ---------------------------------------------------------------------------

func TestMemorySetSupersedesSuccess(t *testing.T) {
	s := helperServer(t)

	// Step 1: Create initial live memory A.
	setRes1 := callTool(s, "memory_set", map[string]interface{}{
		"content": "User prefers tabs over spaces",
		"kind":    "user",
		"scope":   "global",
	})
	text1 := getToolText(setRes1)
	if !strings.Contains(text1, "Memory saved successfully") {
		t.Fatalf("failed to create initial memory: %q", text1)
	}
	idA := memoryIDFromSetText(t, text1)

	mA, err := s.service.Get(idA)
	if err != nil {
		t.Fatalf("failed to get memory A: %v", err)
	}
	if mA.RetiredAt != nil {
		t.Fatalf("memory A should not be retired initially")
	}
	if mA.SupersededBy != "" {
		t.Fatalf("memory A should not have superseded_by initially")
	}

	// Step 2: Call memory_set with supersedes targeting A.
	setRes2 := callTool(s, "memory_set", map[string]interface{}{
		"content":    "User prefers spaces over tabs (corrected)",
		"kind":       "user",
		"scope":      "global",
		"supersedes": idA,
	})
	text2 := getToolText(setRes2)
	if !strings.Contains(text2, "Memory saved successfully") {
		t.Fatalf("superseding memory_set failed: %q", text2)
	}
	idB := memoryIDFromSetText(t, text2)
	if idB == idA {
		t.Fatalf("superseding memory should have a new ID, got identical %s", idB)
	}

	// Step 3: Verify new memory B is live and not retired.
	mB, err := s.service.Get(idB)
	if err != nil {
		t.Fatalf("failed to get memory B: %v", err)
	}
	if mB.RetiredAt != nil {
		t.Fatalf("memory B should not be retired")
	}
	if mB.SupersededBy != "" {
		t.Fatalf("memory B should not have superseded_by set, got %q", mB.SupersededBy)
	}
	if mB.Kind != db.KindUser {
		t.Fatalf("memory B kind = %q, want %q", mB.Kind, db.KindUser)
	}
	if mB.ReviewStatus != db.ReviewApproved {
		t.Fatalf("memory B review_status = %q, want approved", mB.ReviewStatus)
	}

	// Step 4: Verify old memory A is retired and superseded by B.
	mAUpdated, err := s.service.Get(idA)
	if err != nil {
		t.Fatalf("failed to get updated memory A: %v", err)
	}
	if mAUpdated.RetiredAt == nil {
		t.Fatalf("memory A must have retired_at set")
	}
	if mAUpdated.SupersededBy != idB {
		t.Fatalf("memory A superseded_by = %q, want %q", mAUpdated.SupersededBy, idB)
	}
	if mAUpdated.ValidTo == nil {
		t.Fatalf("memory A valid_to must be closed")
	}

	// Step 5: Verify search excludes A and returns B.
	searchRes := callTool(s, "memory_search", map[string]interface{}{
		"query": "prefers tabs or spaces",
		"limit": 5,
	})
	searchText := getToolText(searchRes)
	if strings.Contains(searchText, idA) {
		t.Fatalf("search returned retired memory A (%s): %s", idA, searchText)
	}
	if !strings.Contains(searchText, idB) {
		t.Fatalf("search missing active memory B (%s): %s", idB, searchText)
	}

	// Step 6: Verify memory_list excludes A and returns B.
	listRes := callTool(s, "memory_list", map[string]interface{}{
		"scope": "global",
	})
	listText := getToolText(listRes)
	if strings.Contains(listText, idA) {
		t.Fatalf("list returned retired memory A (%s): %s", idA, listText)
	}
	if !strings.Contains(listText, idB) {
		t.Fatalf("list missing active memory B (%s): %s", idB, listText)
	}

	// Step 7: Verify supersession history queries.
	history, err := s.DB().GetSupersededHistory(idB)
	if err != nil {
		t.Fatalf("failed to get superseded history: %v", err)
	}
	if len(history) != 1 || history[0].ID != idA {
		t.Fatalf("superseded history for B expected [A (%s)], got %+v", idA, history)
	}
}

func TestMemorySetSupersedesInvalidTargets(t *testing.T) {
	s := helperServer(t)

	// Missing target ID.
	res := callTool(s, "memory_set", map[string]interface{}{
		"content":    "New content",
		"kind":       "project",
		"supersedes": "nonexistent-target-uuid",
	})
	text := getToolText(res)
	if !strings.Contains(text, "target memory \"nonexistent-target-uuid\" not found") {
		t.Fatalf("expected missing target error, got %q", text)
	}

	// Empty / whitespace target.
	res = callTool(s, "memory_set", map[string]interface{}{
		"content":    "New content",
		"kind":       "project",
		"supersedes": "   ",
	})
	text = getToolText(res)
	if !strings.Contains(text, "'supersedes' cannot be empty") {
		t.Fatalf("expected empty supersedes error, got %q", text)
	}

	// Target that is already retired.
	setRes := callTool(s, "memory_set", map[string]interface{}{
		"content": "Original fact",
		"kind":    "user",
	})
	origID := memoryIDFromSetText(t, getToolText(setRes))
	if err := s.DB().RetireMemory(origID, time.Now().UTC()); err != nil {
		t.Fatalf("failed to retire memory: %v", err)
	}

	res = callTool(s, "memory_set", map[string]interface{}{
		"content":    "Attempt to supersede retired fact",
		"kind":       "user",
		"supersedes": origID,
	})
	text = getToolText(res)
	if !strings.Contains(text, "is already retired") {
		t.Fatalf("expected already retired error, got %q", text)
	}

	// Target that is already superseded.
	setRes1 := callTool(s, "memory_set", map[string]interface{}{
		"content": "Decision v1",
		"kind":    "project",
	})
	v1ID := memoryIDFromSetText(t, getToolText(setRes1))

	setRes2 := callTool(s, "memory_set", map[string]interface{}{
		"content":    "Decision v2",
		"kind":       "project",
		"supersedes": v1ID,
	})
	_ = memoryIDFromSetText(t, getToolText(setRes2))

	res = callTool(s, "memory_set", map[string]interface{}{
		"content":    "Decision v3 trying to supersede v1 again",
		"kind":       "project",
		"supersedes": v1ID,
	})
	text = getToolText(res)
	if !strings.Contains(text, "is already retired") && !strings.Contains(text, "is already superseded") {
		t.Fatalf("expected already superseded/retired error, got %q", text)
	}
}

func TestMemorySetSupersedesStagedFlow(t *testing.T) {
	s := helperServer(t)

	// Step 1: Create live memory A.
	setRes1 := callTool(s, "memory_set", map[string]interface{}{
		"content": "Backend uses SQLite database",
		"kind":    "project",
		"scope":   "project",
	})
	idA := memoryIDFromSetText(t, getToolText(setRes1))

	// Step 2: Staged write superseding A with candidate B.
	setRes2 := callTool(s, "memory_set", map[string]interface{}{
		"content":    "Backend uses PostgreSQL database",
		"kind":       "project",
		"scope":      "project",
		"supersedes": idA,
		"staged":     true,
	})
	text2 := getToolText(setRes2)
	if !strings.Contains(text2, "staged as candidate") {
		t.Fatalf("expected staged confirmation, got %q", text2)
	}
	idB := memoryIDFromSetText(t, text2)

	// Step 3: Verify candidate B is staged and has supersedes in metadata.
	mB, err := s.service.Get(idB)
	if err != nil {
		t.Fatalf("failed to get staged candidate B: %v", err)
	}
	if mB.ReviewStatus != db.ReviewStaged {
		t.Fatalf("candidate B review status = %q, want staged", mB.ReviewStatus)
	}
	if mB.Metadata["supersedes"] != idA {
		t.Fatalf("candidate B metadata[supersedes] = %q, want %q", mB.Metadata["supersedes"], idA)
	}

	// Step 4: Verify original memory A is STILL active and untouched during staging.
	mA, err := s.service.Get(idA)
	if err != nil {
		t.Fatalf("failed to get memory A: %v", err)
	}
	if mA.RetiredAt != nil {
		t.Fatalf("memory A must NOT be retired while candidate is staged")
	}
	if mA.SupersededBy != "" {
		t.Fatalf("memory A superseded_by must NOT be set while candidate is staged")
	}

	// Step 5: Search still finds A and does not find staged candidate B.
	searchRes := callTool(s, "memory_search", map[string]interface{}{
		"query": "Backend database SQLite PostgreSQL",
		"scope": "project",
		"limit": 5,
	})
	searchText := getToolText(searchRes)
	if !strings.Contains(searchText, idA) {
		t.Fatalf("search should still return live memory A: %s", searchText)
	}
	if strings.Contains(searchText, idB) {
		t.Fatalf("search must NOT return staged candidate B: %s", searchText)
	}

	// Step 6: Promote candidate B.
	promoteRes := callTool(s, "memory_promote", map[string]interface{}{
		"id": idB,
	})
	promoteText := getToolText(promoteRes)
	if !strings.Contains(promoteText, "promoted") {
		t.Fatalf("expected promotion success, got %q", promoteText)
	}

	// Step 7: Verify candidate B is now approved and memory A is now retired and superseded.
	mBApproved, err := s.service.Get(idB)
	if err != nil {
		t.Fatalf("failed to get approved memory B: %v", err)
	}
	if mBApproved.ReviewStatus != db.ReviewApproved {
		t.Fatalf("memory B review status = %q, want approved", mBApproved.ReviewStatus)
	}

	mAAfterPromote, err := s.service.Get(idA)
	if err != nil {
		t.Fatalf("failed to get retired memory A: %v", err)
	}
	if mAAfterPromote.RetiredAt == nil {
		t.Fatalf("memory A must be retired after promotion")
	}
	if mAAfterPromote.SupersededBy != idB {
		t.Fatalf("memory A superseded_by = %q, want %q", mAAfterPromote.SupersededBy, idB)
	}

	// Step 8: Search now returns B and excludes A.
	searchResAfter := callTool(s, "memory_search", map[string]interface{}{
		"query": "Backend database SQLite PostgreSQL",
		"scope": "project",
		"limit": 5,
	})
	searchAfterText := getToolText(searchResAfter)
	if strings.Contains(searchAfterText, `"id": "`+idA+`"`) {
		t.Fatalf("search must not return retired memory A after promote: %s", searchAfterText)
	}
	if !strings.Contains(searchAfterText, `"id": "`+idB+`"`) {
		t.Fatalf("search must return promoted memory B: %s", searchAfterText)
	}
}

func TestMemorySetSupersedesStagedReject(t *testing.T) {
	s := helperServer(t)

	// Step 1: Create live memory A.
	setRes1 := callTool(s, "memory_set", map[string]interface{}{
		"content": "Frontend uses React framework",
		"kind":    "project",
	})
	idA := memoryIDFromSetText(t, getToolText(setRes1))

	// Step 2: Staged write superseding A with candidate B.
	setRes2 := callTool(s, "memory_set", map[string]interface{}{
		"content":    "Frontend uses Vue framework",
		"kind":       "project",
		"supersedes": idA,
		"staged":     true,
	})
	idB := memoryIDFromSetText(t, getToolText(setRes2))

	// Step 3: Reject candidate B.
	rejectRes := callTool(s, "memory_reject", map[string]interface{}{
		"id": idB,
	})
	if !strings.Contains(getToolText(rejectRes), "rejected") {
		t.Fatalf("reject failed: %q", getToolText(rejectRes))
	}

	// Candidate B is deleted.
	_, err := s.service.Get(idB)
	if err == nil {
		t.Fatalf("candidate B should have been deleted on reject")
	}

	// Memory A remains active and untouched.
	mA, err := s.service.Get(idA)
	if err != nil {
		t.Fatalf("failed to get memory A: %v", err)
	}
	if mA.RetiredAt != nil {
		t.Fatalf("memory A must not be retired after candidate reject")
	}
	if mA.SupersededBy != "" {
		t.Fatalf("memory A superseded_by must not be set after candidate reject")
	}
}

func TestMemorySetSupersedesStagedMissingTarget(t *testing.T) {
	s := helperServer(t)

	// Staged write with nonexistent target fails validation immediately.
	res := callTool(s, "memory_set", map[string]interface{}{
		"content":    "Candidate guess",
		"kind":       "project",
		"supersedes": "missing-uuid-123",
		"staged":     true,
	})
	text := getToolText(res)
	if !strings.Contains(text, "target memory \"missing-uuid-123\" not found") {
		t.Fatalf("staged write must validate supersedes target upfront, got %q", text)
	}

	candidates, err := s.service.Candidates(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 0 {
		t.Fatalf("no candidate should be stored on failed validation, got %d", len(candidates))
	}
}

func TestMemorySetSupersedesAtomicFailureBehavior(t *testing.T) {
	s := helperServer(t)

	// Create initial live memory.
	setRes1 := callTool(s, "memory_set", map[string]interface{}{
		"content": "Initial solid preference",
		"kind":    "user",
	})
	origID := memoryIDFromSetText(t, getToolText(setRes1))

	// Attempt a write with invalid kind and supersedes set -> rejected upfront before mutation.
	res := callTool(s, "memory_set", map[string]interface{}{
		"content":    "Failed correction",
		"kind":       "invalid-kind-name",
		"supersedes": origID,
	})
	text := getToolText(res)
	if !strings.Contains(text, "'kind' is required") {
		t.Fatalf("expected kind error, got %q", text)
	}

	// Verify original memory is untouched.
	mOrig, err := s.service.Get(origID)
	if err != nil {
		t.Fatalf("failed to get memory: %v", err)
	}
	if mOrig.RetiredAt != nil || mOrig.SupersededBy != "" {
		t.Fatalf("original memory must remain untouched on validation failure")
	}

	// Check that count of memories is still 1.
	list, err := s.DB().ListMemories("", 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected exactly 1 memory in store, got %d", len(list))
	}
}
