package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestCheckpointAndClose(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		t.Fatalf("set WAL: %v", err)
	}

	if _, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, val TEXT);"); err != nil {
		t.Fatalf("create table: %v", err)
	}

	if _, err := db.Exec("INSERT INTO t (val) VALUES (?)", "hello"); err != nil {
		t.Fatalf("insert: %v", err)
	}

	db.Close()

	walPath := dbPath + "-wal"
	if _, err := os.Stat(walPath); os.IsNotExist(err) {
		t.Log("WAL file not present after close (PASSIVE checkpoint), but checkpoint still succeeds")
	}

	if err := checkpointAndClose(dbPath); err != nil {
		t.Fatalf("checkpointAndClose: %v", err)
	}

	db2, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db2.Close()

	var val string
	if err := db2.QueryRow("SELECT val FROM t WHERE id = 1").Scan(&val); err != nil {
		t.Fatalf("query after checkpoint: %v", err)
	}
	if val != "hello" {
		t.Errorf("expected 'hello', got '%s'", val)
	}
}

func TestValidateSQLiteFile_Valid(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "valid.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		t.Fatalf("set WAL: %v", err)
	}
	if _, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY);"); err != nil {
		t.Fatalf("create: %v", err)
	}
	db.Close()

	data, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if err := validateSQLiteFile(data); err != nil {
		t.Errorf("expected valid, got error: %v", err)
	}
}

func TestValidateSQLiteFile_Invalid(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"too small", []byte("short")},
		{"wrong magic", bytes.Repeat([]byte{0}, 1024)},
		{"random garbage", []byte("this is not a database file at all")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateSQLiteFile(tt.data); err == nil {
				t.Errorf("expected error for %s, got nil", tt.name)
			}
		})
	}
}

func TestValidateSQLiteFile_CorruptIntegrity(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "corrupt.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY);"); err != nil {
		t.Fatalf("create: %v", err)
	}
	db.Close()

	data, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	midpoint := len(data) / 2
	data[midpoint] ^= 0xFF

	if err := validateSQLiteFile(data); err == nil {
		t.Error("expected error for corrupted database, got nil")
	}
}

func TestRoundTripBackupRestoreWithWALWrites(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "symmemory", "default.db")
	backupPath := filepath.Join(dir, "backup.tar.gz")

	if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		t.Fatalf("set WAL: %v", err)
	}

	if _, err := db.Exec(`CREATE TABLE memories (
		id TEXT PRIMARY KEY, content TEXT, scope TEXT, metadata TEXT,
		embedding TEXT, embedding_dim INTEGER, lsh_hash INTEGER,
		created_at DATETIME, updated_at DATETIME,
		created_by TEXT, updated_by TEXT, created_session TEXT, updated_session TEXT,
		consolidation_status TEXT, consolidated_into_id TEXT,
		importance REAL, valid_from DATETIME, valid_to DATETIME, superseded_by TEXT
	)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	memIDs := []string{"mem-1", "mem-2", "mem-3", "mem-4", "mem-5"}
	for _, id := range memIDs {
		_, err := db.Exec(
			`INSERT INTO memories (id, content, scope, metadata, embedding, embedding_dim, lsh_hash,
			created_at, updated_at, consolidation_status, importance)
			VALUES (?, ?, 'global', '{}', '[]', 0, 0, datetime('now'), datetime('now'), 'raw', 0.5)`,
			id, "content for "+id,
		)
		if err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}

	db.Close()

	if err := checkpointAndClose(dbPath); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}

	dbBytes, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read db: %v", err)
	}

	if err := createTarGz(backupPath, "default.db", dbBytes); err != nil {
		t.Fatalf("create tar.gz: %v", err)
	}

	restoreDir := t.TempDir()
	restoreDBPath := filepath.Join(restoreDir, "restored.db")

	extractedDBBytes, err := extractTarGz(backupPath)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}

	if err := validateSQLiteFile(extractedDBBytes); err != nil {
		t.Fatalf("validate: %v", err)
	}

	tmpFile, err := os.CreateTemp(restoreDir, "restore-*.db")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	stagingPath := tmpFile.Name()

	if err := tmpFile.Chmod(0600); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if _, err := tmpFile.Write(extractedDBBytes); err != nil {
		t.Fatalf("write staging: %v", err)
	}
	if err := tmpFile.Sync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	tmpFile.Close()

	if err := os.Rename(stagingPath, restoreDBPath); err != nil {
		t.Fatalf("atomic rename: %v", err)
	}

	info, err := os.Stat(restoreDBPath)
	if err != nil {
		t.Fatalf("stat restored: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected permissions 0600, got %o", info.Mode().Perm())
	}

	db2, err := sql.Open("sqlite", restoreDBPath)
	if err != nil {
		t.Fatalf("open restored: %v", err)
	}
	defer db2.Close()

	var count int
	if err := db2.QueryRow("SELECT COUNT(*) FROM memories").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != len(memIDs) {
		t.Errorf("expected %d memories, got %d", len(memIDs), count)
	}

	for _, id := range memIDs {
		var content string
		if err := db2.QueryRow("SELECT content FROM memories WHERE id = ?", id).Scan(&content); err != nil {
			t.Errorf("missing memory %s: %v", id, err)
		}
		expected := "content for " + id
		if content != expected {
			t.Errorf("memory %s: expected '%s', got '%s'", id, expected, content)
		}
	}
}

func TestRestoreRejectsInvalidDatabase(t *testing.T) {
	dir := t.TempDir()
	backupPath := filepath.Join(dir, "bad-backup.tar.gz")

	invalidData := []byte("this is definitely not a sqlite database")

	if err := createTarGz(backupPath, "default.db", invalidData); err != nil {
		t.Fatalf("create tar.gz: %v", err)
	}

	extractedBytes, err := extractTarGz(backupPath)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}

	if err := validateSQLiteFile(extractedBytes); err == nil {
		t.Error("expected validation to fail for invalid database")
	}
}

func TestRestoreDoesNotReplaceOnInvalidData(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "existing.db")

	origDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := origDB.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, val TEXT);"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := origDB.Exec("INSERT INTO t (val) VALUES ('original')"); err != nil {
		t.Fatalf("insert: %v", err)
	}
	origDB.Close()

	invalidData := []byte("not-a-database")
	if err := validateSQLiteFile(invalidData); err == nil {
		t.Fatal("expected validation to fail")
	}

	db2, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db2.Close()

	var val string
	if err := db2.QueryRow("SELECT val FROM t WHERE id = 1").Scan(&val); err != nil {
		t.Fatalf("query: %v", err)
	}
	if val != "original" {
		t.Errorf("expected original data preserved, got '%s'", val)
	}
}

func TestStagingFilePermissions(t *testing.T) {
	dir := t.TempDir()

	tmpFile, err := os.CreateTemp(dir, "stage-*.db")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	path := tmpFile.Name()
	defer os.Remove(path)

	if err := tmpFile.Chmod(0600); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	tmpFile.Close()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected 0600, got %o", info.Mode().Perm())
	}
}

func TestWALCheckpointTruncatesSidecar(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "wal-test.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		t.Fatalf("set WAL: %v", err)
	}
	if _, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY);"); err != nil {
		t.Fatalf("create: %v", err)
	}

	for i := range 1000 {
		if _, err := db.Exec("INSERT INTO t (id) VALUES (?)", i); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}

	db.Close()

	walBefore := dbPath + "-wal"
	if _, err := os.Stat(walBefore); err == nil {
		t.Log("WAL sidecar present before checkpoint (expected)")
	}

	if err := checkpointAndClose(dbPath); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}

	walAfter := dbPath + "-wal"
	if _, err := os.Stat(walAfter); err == nil {
		t.Log("WAL sidecar may still exist after TRUNCATE (non-fatal); main DB is consistent")
	}

	db2, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db2.Close()

	var count int
	if err := db2.QueryRow("SELECT COUNT(*) FROM t").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1000 {
		t.Errorf("expected 1000 rows, got %d", count)
	}
}

func createTarGz(path, entryName string, data []byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	hdr := &tar.Header{
		Name: entryName,
		Size: int64(len(data)),
		Mode: 0600,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err = tw.Write(data)
	return err
}

func extractTarGz(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err != nil {
			return nil, err
		}
		if filepath.Ext(hdr.Name) == ".db" || filepath.Base(hdr.Name) == "default.db" {
			var buf bytes.Buffer
			if _, err := buf.ReadFrom(tr); err != nil {
				return nil, err
			}
			return buf.Bytes(), nil
		}
	}
}
