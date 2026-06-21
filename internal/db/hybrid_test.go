package db

import (
	"testing"

	"github.com/danieljustus/symaira-memory/internal/config"
)

func TestTokenize(t *testing.T) {
	tokens := Tokenize("Hello, World! This is a test.")
	if len(tokens) != 3 {
		t.Errorf("expected 3 tokens (hello, world, test), got %d: %v", len(tokens), tokens)
	}
}

func TestTokenize_StopWordsRemoved(t *testing.T) {
	tokens := Tokenize("the quick brown fox is a very fast animal")
	for _, stop := range []string{"the", "is", "a", "very"} {
		for _, tok := range tokens {
			if tok == stop {
				t.Errorf("stop word %q should be removed", stop)
			}
		}
	}
}

func TestBM25Index_BasicScoring(t *testing.T) {
	idx := newBM25Index()
	idx.addDoc("doc1", "the quick brown fox jumps over the lazy dog")
	idx.addDoc("doc2", "the lazy dog sleeps in the corner")
	idx.addDoc("doc3", "a quick red car drives fast")

	scores := idx.score(Tokenize("quick fox"))
	if scores["doc1"] <= 0 {
		t.Errorf("expected doc1 to score > 0 for 'quick fox', got %f", scores["doc1"])
	}
	// BM25 scores partial matches — doc3 contains "quick" but not "fox",
	// so it gets a positive score from the matching term.
	if scores["doc3"] < 0 {
		t.Errorf("expected doc3 to score >= 0 for 'quick fox', got %f", scores["doc3"])
	}
}

func TestBM25Index_ExactKeywordMatch(t *testing.T) {
	idx := newBM25Index()
	idx.addDoc("doc1", "Alice prefers dark mode in all applications")
	idx.addDoc("doc2", "Bob likes light themes")

	scores := idx.score(Tokenize("dark mode"))
	if scores["doc1"] <= 0 {
		t.Errorf("expected doc1 to score > 0 for 'dark mode'")
	}
	if _, ok := scores["doc2"]; ok {
		t.Errorf("expected doc2 to not score for 'dark mode'")
	}
}

func TestReciprocalRankFusion_MergesResults(t *testing.T) {
	list1 := []string{"a", "b", "c"}
	list2 := []string{"b", "c", "d"}

	scores := ReciprocalRankFusion([][]string{list1, list2}, 60)
	if scores["b"] <= scores["a"] {
		t.Errorf("expected 'b' (in both lists) to score higher than 'a' (in one list)")
	}
	if scores["b"] <= scores["d"] {
		t.Errorf("expected 'b' to score higher than 'd'")
	}
}

func TestSearchMemoriesBM25_FindsExactKeyword(t *testing.T) {
	cfg := config.Defaults()
	cfg.Database.Path = t.TempDir() + "/test.db"
	database, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	dark := &Memory{
		ID:       "dark-mode",
		Content:  "Alice prefers dark mode in all applications",
		Scope:    "global",
		Metadata: map[string]string{},
	}
	if err := database.SaveMemory(dark); err != nil {
		t.Fatal(err)
	}

	light := &Memory{
		ID:       "light-theme",
		Content:  "Bob likes light themes for everything",
		Scope:    "global",
		Metadata: map[string]string{},
	}
	if err := database.SaveMemory(light); err != nil {
		t.Fatal(err)
	}

	results, err := database.SearchMemoriesBM25("dark mode", "global", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	if results[0].Memory.ID != "dark-mode" {
		t.Errorf("expected dark-mode as top result, got %s", results[0].Memory.ID)
	}
}

func TestSearchMemoriesBM25_EmptyQuery(t *testing.T) {
	cfg := config.Defaults()
	cfg.Database.Path = t.TempDir() + "/test.db"
	database, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	results, err := database.SearchMemoriesBM25("the a is", "global", 5)
	if err != nil {
		t.Fatal(err)
	}
	if results != nil {
		t.Errorf("expected nil for stop-word-only query, got %d results", len(results))
	}
}

func TestSearchMemoriesBM25_RejectsInvalidScope(t *testing.T) {
	cfg := config.Defaults()
	cfg.Database.Path = t.TempDir() + "/test.db"
	database, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	injections := []string{
		`" OR 1=1 --`,
		"global OR NOT global",
		"scope:*",
		"global NEAR test",
	}
	for _, scope := range injections {
		_, err := database.SearchMemoriesBM25("test", scope, 5)
		if err == nil {
			t.Errorf("expected error for injected scope %q, got nil", scope)
		}
	}
}
