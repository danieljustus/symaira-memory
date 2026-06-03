package db

import (
	"os"
	"path/filepath"
	"testing"
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
	database, err := Open()
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
	mems, err := database.ListMemories("")
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

	mems, err = database.ListMemories("")
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

	database, err := Open()
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

	db1, err := Open()
	if err != nil {
		t.Fatalf("first open failed: %v", err)
	}
	db1.Close()

	db2, err := Open()
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
	if count != 1 {
		t.Errorf("expected 1 migration after two opens, got %d", count)
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
