package consolidation

import (
	"os"
	"testing"
	"time"

	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/db"
)

// testLinkerDB creates a temporary database for linker tests.
func testLinkerDB(t *testing.T) *db.DB {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "symmemory-linker-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tempDir) })

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	t.Cleanup(func() { os.Getenv("HOME"); os.Setenv("HOME", oldHome) })

	database, err := db.Open(config.Defaults())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestNewLinkerDefaults(t *testing.T) {
	database := testLinkerDB(t)
	linker := NewLinker(database, 0, 0)

	if linker.similarityThreshold != 0.85 {
		t.Errorf("expected default similarity threshold 0.85, got %f", linker.similarityThreshold)
	}
	if linker.contentThreshold != 0.80 {
		t.Errorf("expected content threshold 0.80, got %f", linker.contentThreshold)
	}
	if linker.maxPairs != 10000 {
		t.Errorf("expected default maxPairs 10000, got %d", linker.maxPairs)
	}
}

func TestNewLinkerCustomParams(t *testing.T) {
	database := testLinkerDB(t)
	linker := NewLinker(database, 0.9, 500)

	if linker.similarityThreshold != 0.9 {
		t.Errorf("expected similarity threshold 0.9, got %f", linker.similarityThreshold)
	}
	if linker.maxPairs != 500 {
		t.Errorf("expected maxPairs 500, got %d", linker.maxPairs)
	}
}

func TestSimilarityScoreIdenticalContent(t *testing.T) {
	database := testLinkerDB(t)
	linker := NewLinker(database, 0, 0)

	m1 := &db.Memory{Content: "hello world"}
	m2 := &db.Memory{Content: "hello world"}

	score := linker.similarityScore(m1, m2)
	if score != 1.0 {
		t.Errorf("expected 1.0 for identical content, got %f", score)
	}
}

func TestSimilarityScoreWithEmbeddings(t *testing.T) {
	database := testLinkerDB(t)
	linker := NewLinker(database, 0, 0)

	// Identical embeddings should yield cosine similarity of 1.0
	m1 := &db.Memory{Content: "fact A", Embedding: []float32{1, 0, 0}}
	m2 := &db.Memory{Content: "fact B", Embedding: []float32{1, 0, 0}}

	score := linker.similarityScore(m1, m2)
	if score < 0.99 {
		t.Errorf("expected ~1.0 for identical embeddings, got %f", score)
	}
}

func TestSimilarityScoreEmbeddingMismatchSource(t *testing.T) {
	database := testLinkerDB(t)
	linker := NewLinker(database, 0, 0)

	// Different embedding sources → fall back to Jaccard content
	m1 := &db.Memory{Content: "the cat sat on the mat", Embedding: []float32{1, 0}, EmbeddingSource: "ollama"}
	m2 := &db.Memory{Content: "the cat sat on the mat", Embedding: []float32{0, 1}, EmbeddingSource: "openai"}

	score := linker.similarityScore(m1, m2)
	// Same content → Jaccard should be high (1.0 since identical tokens)
	if score < 0.9 {
		t.Errorf("expected high Jaccard score for identical content with mismatched sources, got %f", score)
	}
}

func TestSimilarityScoreNoEmbeddings(t *testing.T) {
	database := testLinkerDB(t)
	linker := NewLinker(database, 0, 0)

	m1 := &db.Memory{Content: "the cat sat on the mat"}
	m2 := &db.Memory{Content: "the cat sat on the rug"}

	score := linker.similarityScore(m1, m2)
	// Should use Jaccard content similarity
	if score <= 0 || score >= 1.0 {
		t.Errorf("expected Jaccard score between 0 and 1, got %f", score)
	}
}

func TestJaccardContentIdentical(t *testing.T) {
	database := testLinkerDB(t)
	linker := NewLinker(database, 0, 0)

	score := linker.jaccardContent("hello world", "hello world")
	if score != 1.0 {
		t.Errorf("expected 1.0 for identical strings, got %f", score)
	}
}

func TestJaccardContentDisjoint(t *testing.T) {
	database := testLinkerDB(t)
	linker := NewLinker(database, 0, 0)

	score := linker.jaccardContent("hello", "world")
	if score != 0.0 {
		t.Errorf("expected 0.0 for disjoint tokens, got %f", score)
	}
}

func TestJaccardContentEmpty(t *testing.T) {
	database := testLinkerDB(t)
	linker := NewLinker(database, 0, 0)

	score := linker.jaccardContent("", "hello")
	if score != 0.0 {
		t.Errorf("expected 0.0 for empty input, got %f", score)
	}

	score = linker.jaccardContent("hello", "")
	if score != 0.0 {
		t.Errorf("expected 0.0 for empty input, got %f", score)
	}
}

func TestJaccardContentPartialOverlap(t *testing.T) {
	database := testLinkerDB(t)
	linker := NewLinker(database, 0, 0)

	// "the" is a stop word and gets filtered out
	// "the cat sat" → {cat, sat}, "the dog sat" → {dog, sat}
	// intersection=1 (sat), union=3 → 1/3 ≈ 0.333
	score := linker.jaccardContent("the cat sat", "the dog sat")
	if score < 0.3 || score > 0.4 {
		t.Errorf("expected ~0.333 for partial overlap (stop words filtered), got %f", score)
	}
}

func TestCreateLinkSkipsArchived(t *testing.T) {
	database := testLinkerDB(t)
	linker := NewLinker(database, 0, 0)

	m1 := &db.Memory{Content: "fact A", ConsolidationStatus: "archived"}
	m2 := &db.Memory{Content: "fact B"}

	result := &LinkResult{}
	err := linker.createLink(m1, m2, 0.99, result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Superseded != 0 || result.SynergiesCreated != 0 {
		t.Errorf("expected no links for archived memories, got %+v", result)
	}
}

func TestCreateLinkSupersede(t *testing.T) {
	database := testLinkerDB(t)
	linker := NewLinker(database, 0, 0)

	now := time.Now()
	older := now.Add(-1 * time.Hour)
	newer := now

	// Save two memories with different scopes
	m1 := &db.Memory{
		ID:        "mem-1",
		Content:   "identical fact",
		Scope:     "claude-code",
		CreatedAt: older,
		UpdatedAt: older,
		Metadata:  map[string]string{},
	}
	m2 := &db.Memory{
		ID:        "mem-2",
		Content:   "identical fact",
		Scope:     "hermes",
		CreatedAt: newer,
		UpdatedAt: newer,
		Metadata:  map[string]string{},
	}

	if err := database.SaveMemory(m1); err != nil {
		t.Fatalf("save m1: %v", err)
	}
	if err := database.SaveMemory(m2); err != nil {
		t.Fatalf("save m2: %v", err)
	}

	result := &LinkResult{}
	err := linker.createLink(m1, m2, 1.0, result) // score >= 0.95 → supersede
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Superseded != 1 {
		t.Errorf("expected 1 supersede, got %d", result.Superseded)
	}
	if result.SynergiesCreated != 0 {
		t.Errorf("expected 0 synergies, got %d", result.SynergiesCreated)
	}
}

func TestCreateLinkSynergy(t *testing.T) {
	database := testLinkerDB(t)
	linker := NewLinker(database, 0, 0)

	now := time.Now()
	m1 := &db.Memory{
		ID:        "mem-1",
		Content:   "similar fact A",
		Scope:     "claude-code",
		CreatedAt: now,
		UpdatedAt: now,
		Metadata:  map[string]string{},
	}
	m2 := &db.Memory{
		ID:        "mem-2",
		Content:   "similar fact B",
		Scope:     "hermes",
		CreatedAt: now,
		UpdatedAt: now,
		Metadata:  map[string]string{},
	}

	if err := database.SaveMemory(m1); err != nil {
		t.Fatalf("save m1: %v", err)
	}
	if err := database.SaveMemory(m2); err != nil {
		t.Fatalf("save m2: %v", err)
	}

	result := &LinkResult{}
	err := linker.createLink(m1, m2, 0.90, result) // 0.85 <= score < 0.95 → synergy
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.SynergiesCreated != 1 {
		t.Errorf("expected 1 synergy, got %d", result.SynergiesCreated)
	}
	if result.Superseded != 0 {
		t.Errorf("expected 0 superseded, got %d", result.Superseded)
	}
}

func TestLinkCrossToolEmptyScopes(t *testing.T) {
	database := testLinkerDB(t)
	linker := NewLinker(database, 0, 0)

	result, err := linker.LinkCrossTool(true) // dry run
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.LinksCreated != 0 || result.SynergiesCreated != 0 || result.Superseded != 0 {
		t.Errorf("expected empty result for empty scopes, got %+v", result)
	}
}

func TestLinkCrossToolWithMemories(t *testing.T) {
	database := testLinkerDB(t)
	linker := NewLinker(database, 0.5, 0) // low threshold to ensure matches

	now := time.Now()

	// Create memories with embeddings in two different tool scopes
	// Identical embeddings will yield cosine similarity = 1.0
	embedding := []float32{0.1, 0.2, 0.3, 0.4, 0.5}
	m1 := &db.Memory{
		ID:            "mem-claude-1",
		Content:       "user prefers dark mode",
		Scope:         "claude-code",
		Embedding:     embedding,
		EmbeddingSource: "ollama",
		CreatedAt:     now,
		UpdatedAt:     now,
		Metadata:      map[string]string{},
	}
	m2 := &db.Memory{
		ID:            "mem-hermes-1",
		Content:       "user prefers dark mode",
		Scope:         "hermes",
		Embedding:     embedding,
		EmbeddingSource: "ollama",
		CreatedAt:     now,
		UpdatedAt:     now,
		Metadata:      map[string]string{},
	}

	if err := database.SaveMemory(m1); err != nil {
		t.Fatalf("save m1: %v", err)
	}
	if err := database.SaveMemory(m2); err != nil {
		t.Fatalf("save m2: %v", err)
	}

	result, err := linker.LinkCrossTool(false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With identical embeddings and low threshold, should create at least one link
	if result.LinksCreated == 0 && result.SynergiesCreated == 0 && result.Superseded == 0 {
		t.Errorf("expected some linking activity, got %+v", result)
	}
}

func TestLinkCrossToolDryRun(t *testing.T) {
	database := testLinkerDB(t)
	linker := NewLinker(database, 0.5, 0)

	now := time.Now()
	embedding := []float32{0.1, 0.2, 0.3, 0.4, 0.5}
	m1 := &db.Memory{
		ID:            "mem-claude-1",
		Content:       "user prefers dark mode",
		Scope:         "claude-code",
		Embedding:     embedding,
		EmbeddingSource: "ollama",
		CreatedAt:     now,
		UpdatedAt:     now,
		Metadata:      map[string]string{},
	}
	m2 := &db.Memory{
		ID:            "mem-hermes-1",
		Content:       "user prefers dark mode",
		Scope:         "hermes",
		Embedding:     embedding,
		EmbeddingSource: "ollama",
		CreatedAt:     now,
		UpdatedAt:     now,
		Metadata:      map[string]string{},
	}

	if err := database.SaveMemory(m1); err != nil {
		t.Fatalf("save m1: %v", err)
	}
	if err := database.SaveMemory(m2); err != nil {
		t.Fatalf("save m2: %v", err)
	}

	result, err := linker.LinkCrossTool(true) // dry run
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Dry run should count but not create actual links
	if result.LinksCreated == 0 {
		t.Errorf("dry run should count potential links, got 0")
	}
}
