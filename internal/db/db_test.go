package db

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/danieljustus/symaira-memory/internal/config"
)

func TestDBSchemaAndOperations(t *testing.T) {
	// Temporarily redirect home dir for testing to prevent polluting standard DB
	tempDir, err := os.MkdirTemp("", "symmemory-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	// Open temp DB
	database, err := Open(config.Defaults())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Verify database file was created inside temp directory
	expectedPath := filepath.Join(tempDir, ".local", "share", "symmemory", "default.db")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("database file was not created at expected path: %s", expectedPath)
	}

	// Create a test memory
	m := &Memory{
		ID:        "test-id-123",
		Content:   "User works on symaira.com",
		Scope:     "global",
		Metadata:  map[string]string{"source": "test"},
		Embedding: []float32{1.0, 0.0, 0.0},
	}

	// Test Save
	if err := database.SaveMemory(m); err != nil {
		t.Fatalf("failed to save memory: %v", err)
	}

	// Test List
	mems, err := database.ListMemories("", 0, 100)
	if err != nil {
		t.Fatalf("failed to list memories: %v", err)
	}

	if len(mems) != 1 {
		t.Errorf("expected 1 memory, got %d", len(mems))
	} else {
		if mems[0].ID != "test-id-123" {
			t.Errorf("expected memory ID 'test-id-123', got '%s'", mems[0].ID)
		}
		if mems[0].Content != "User works on symaira.com" {
			t.Errorf("expected content 'User works on symaira.com', got '%s'", mems[0].Content)
		}
	}

	// Test Search (Vector Cosine Similarity check)
	query := []float32{1.0, 0.0, 0.0} // Perfect match
	results, err := database.SearchMemories(query, "", 5)
	if err != nil {
		t.Fatalf("failed to search memories: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 search result, got %d", len(results))
	} else {
		if results[0].Score != 1.0 {
			t.Errorf("expected perfect similarity score 1.0, got %.4f", results[0].Score)
		}
	}

	// Test Session summaries
	if err := database.SaveSessionSummary("session-1", "Extracted test summary"); err != nil {
		t.Fatalf("failed to save session summary: %v", err)
	}

	sum, err := database.GetSessionSummary("session-1")
	if err != nil {
		t.Fatalf("failed to get summary: %v", err)
	}
	if sum != "Extracted test summary" {
		t.Errorf("expected summary 'Extracted test summary', got '%s'", sum)
	}

	// Test Delete
	if err := database.DeleteMemory("test-id-123"); err != nil {
		t.Fatalf("failed to delete memory: %v", err)
	}

	mems, err = database.ListMemories("", 0, 100)
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}
	if len(mems) != 0 {
		t.Errorf("expected empty database after deletion, got %d memories", len(mems))
	}
}

func TestMigrationsApplied(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "symmemory-migrate-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	database, err := Open(config.Defaults())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	var count int
	if err := database.conn.QueryRow(
		"SELECT COUNT(*) FROM schema_migrations",
	).Scan(&count); err != nil {
		t.Fatalf("schema_migrations table not created: %v", err)
	}
	if count == 0 {
		t.Errorf("expected at least one migration applied, got 0")
	}

	var version string
	if err := database.conn.QueryRow(
		"SELECT version FROM schema_migrations WHERE version = ?", "001_init",
	).Scan(&version); err != nil {
		t.Errorf("migration 001_init not recorded: %v", err)
	} else if version != "001_init" {
		t.Errorf("expected version '001_init', got '%s'", version)
	}
}

func TestMigrationsIdempotent(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "symmemory-idem-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	db1, err := Open(config.Defaults())
	if err != nil {
		t.Fatalf("first open failed: %v", err)
	}
	db1.Close()

	db2, err := Open(config.Defaults())
	if err != nil {
		t.Fatalf("second open failed: %v", err)
	}
	defer db2.Close()

	var count int
	if err := db2.conn.QueryRow(
		"SELECT COUNT(*) FROM schema_migrations",
	).Scan(&count); err != nil {
		t.Fatalf("failed to query migrations: %v", err)
	}
	if count != 8 {
		t.Errorf("expected 8 migrations after two opens, got %d", count)
	}
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a        []float32
		b        []float32
		expected float32
	}{
		{
			name:     "Perfect alignment",
			a:        []float32{1.0, 0.0},
			b:        []float32{1.0, 0.0},
			expected: 1.0,
		},
		{
			name:     "Orthogonal vectors",
			a:        []float32{1.0, 0.0},
			b:        []float32{0.0, 1.0},
			expected: 0.0,
		},
		{
			name:     "Opposing vectors",
			a:        []float32{1.0, 0.0},
			b:        []float32{-1.0, 0.0},
			expected: -1.0,
		},
		{
			name:     "Empty vectors",
			a:        []float32{},
			b:        []float32{},
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := CosineSimilarity(tt.a, tt.b)
			// Check similarity with small float delta
			if mathAbs(score-tt.expected) > 1e-5 {
				t.Errorf("expected similarity %f, got %f", tt.expected, score)
			}
		})
	}
}

func mathAbs(f float32) float32 {
	if f < 0 {
		return -f
	}
	return f
}

func TestLSHConsistency(t *testing.T) {
	vec := make([]float32, EmbeddingDim)
	for i := range vec {
		vec[i] = float32(i) * 0.01
	}

	h1 := ComputeLSH(vec)
	h2 := ComputeLSH(vec)
	if h1 != h2 {
		t.Errorf("LSH not deterministic: %d vs %d", h1, h2)
	}
}

func TestLSHDifferentVectors(t *testing.T) {
	a := make([]float32, EmbeddingDim)
	b := make([]float32, EmbeddingDim)
	for i := range a {
		a[i] = float32(i) * 0.01
		b[i] = -float32(i) * 0.01
	}

	ha := ComputeLSH(a)
	hb := ComputeLSH(b)
	if ha == hb {
		t.Error("expected different LSH hashes for opposing vectors")
	}
}

func TestLSHEmptyVector(t *testing.T) {
	h := ComputeLSH(nil)
	if h != 0 {
		t.Errorf("expected LSH 0 for nil vector, got %d", h)
	}
	h = ComputeLSH([]float32{})
	if h != 0 {
		t.Errorf("expected LSH 0 for empty vector, got %d", h)
	}
}

func TestSearchWithLSHIndex(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "symmemory-lsh-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	database, err := Open(config.Defaults())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Seed 100 memories with varying embeddings
	for i := 0; i < 100; i++ {
		emb := make([]float32, EmbeddingDim)
		emb[i%EmbeddingDim] = 1.0
		emb[(i+37)%EmbeddingDim] = 0.5
		m := &Memory{
			ID:        fmt.Sprintf("lsh-mem-%d", i),
			Content:   fmt.Sprintf("Memory number %d with specific pattern", i),
			Scope:     "global",
			Embedding: emb,
		}
		if err := database.SaveMemory(m); err != nil {
			t.Fatalf("failed to save memory %d: %v", i, err)
		}
	}

	// Search should only load a fraction of rows thanks to LSH pre-filter
	queryVec := make([]float32, EmbeddingDim)
	queryVec[0] = 1.0

	results, err := database.SearchMemories(queryVec, "global", 5)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected at least one search result")
	}
}

// BenchmarkSearchWithLSH measures search performance with 1000 indexed memories.
func BenchmarkSearchWithLSH(b *testing.B) {
	tempDir, err := os.MkdirTemp("", "symmemory-bench-*")
	if err != nil {
		b.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	database, err := Open(config.Defaults())
	if err != nil {
		b.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Seed 1000 memories
	n := 1000
	for i := 0; i < n; i++ {
		emb := make([]float32, EmbeddingDim)
		emb[i%EmbeddingDim] = 1.0
		emb[(i+37)%EmbeddingDim] = 0.3
		m := &Memory{
			ID:        fmt.Sprintf("bench-%d", i),
			Content:   fmt.Sprintf("Benchmark memory entry %d", i),
			Scope:     "global",
			Embedding: emb,
		}
		if err := database.SaveMemory(m); err != nil {
			b.Fatalf("failed to seed: %v", err)
		}
	}

	queryVec := make([]float32, EmbeddingDim)
	queryVec[0] = 1.0
	queryVec[1] = 0.3

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := database.SearchMemories(queryVec, "global", 5)
		if err != nil {
			b.Fatalf("search failed: %v", err)
		}
	}
}

func TestDatabaseFilePermissions(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "symmemory-perm-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	database, err := Open(config.Defaults())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	dbPath := filepath.Join(tempDir, ".local", "share", "symmemory", "default.db")

	// Check directory permissions
	dirPath := filepath.Dir(dbPath)
	dirInfo, err := os.Stat(dirPath)
	if err != nil {
		t.Fatalf("failed to stat directory: %v", err)
	}
	if dirInfo.Mode().Perm() != 0700 {
		t.Errorf("expected directory permissions 0700, got %o", dirInfo.Mode().Perm())
	}

	// Create a memory to ensure the DB file exists
	m := &Memory{
		ID:        "perm-test",
		Content:   "test content",
		Scope:     "global",
		Metadata:  map[string]string{},
		Embedding: []float32{1.0},
	}
	if err := database.SaveMemory(m); err != nil {
		t.Fatalf("failed to save memory: %v", err)
	}

	// Check database file permissions
	dbInfo, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("failed to stat database file: %v", err)
	}
	if dbInfo.Mode().Perm() != 0600 {
		t.Errorf("expected database file permissions 0600, got %o", dbInfo.Mode().Perm())
	}
}

func TestGetMemoriesSinceFilter(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "symmemory-since-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	database, err := Open(config.Defaults())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	oldTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	newTime := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	cutoff := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < 10; i++ {
		_, err := database.conn.Exec(
			`INSERT INTO memories (id, content, scope, metadata, embedding, embedding_dim, lsh_hash, created_at, updated_at, created_by, updated_by, created_session, updated_session)
			 VALUES (?, ?, 'global', '{}', '[]', 0, 0, ?, ?, '', '', '', '')`,
			fmt.Sprintf("since-mem-%d", i), fmt.Sprintf("content %d", i), oldTime, oldTime,
		)
		if err != nil {
			t.Fatalf("failed to insert memory %d: %v", i, err)
		}
	}

	for i := 0; i < 3; i++ {
		_, err := database.conn.Exec(
			"UPDATE memories SET updated_at = ? WHERE id = ?",
			newTime, fmt.Sprintf("since-mem-%d", i),
		)
		if err != nil {
			t.Fatalf("failed to update memory %d: %v", i, err)
		}
	}

	results, err := database.GetMemoriesSince(cutoff)
	if err != nil {
		t.Fatalf("GetMemoriesSince failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("expected 3 updated memories, got %d", len(results))
	}

	for _, m := range results {
		if !m.UpdatedAt.After(cutoff) {
			t.Errorf("returned memory %s with updated_at %v not after cutoff %v", m.ID, m.UpdatedAt, cutoff)
		}
	}
}
