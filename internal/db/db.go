package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/danieljustus/symaira-corekit/sqlitekit"
	"github.com/danieljustus/symaira-memory/internal/config"
	_ "modernc.org/sqlite"
)

// DB wraps the SQL connection.
type DB struct {
	conn *sql.DB
}

// Open initializes the SQLite database at the standard XDG path,
// or at the path specified in the supplied configuration. The caller
// (typically cmd/) is responsible for loading configuration via
// config.Load(); library code never reads from disk directly.
func Open(cfg *config.Config) (*DB, error) {
	if cfg == nil {
		cfg = config.Defaults()
	}

	var dbPath string
	if cfg.Database.Path != "" {
		dbPath = cfg.Database.Path
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get user home dir: %w", err)
		}
		dbPath = filepath.Join(home, ".local", "share", "symmemory", "default.db")
	}

	conn, err := sqlitekit.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	db := &DB{conn: conn}
	if err := db.runMigrations(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	if _, err := os.Stat(dbPath); err == nil {
		if err := os.Chmod(dbPath, 0600); err != nil {
			conn.Close()
			return nil, fmt.Errorf("failed to set db file permissions: %w", err)
		}
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		sibling := dbPath + suffix
		if _, err := os.Stat(sibling); err == nil {
			_ = os.Chmod(sibling, 0600)
		}
	}

	return db, nil
}

// ResolvePath returns the filesystem path to the SQLite database file
// for the given configuration. When cfg is nil or cfg.Database.Path is
// empty the standard XDG default is used. The file may or may not exist.
func ResolvePath(cfg *config.Config) string {
	if cfg != nil && cfg.Database.Path != "" {
		return cfg.Database.Path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "symmemory", "default.db")
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// Conn returns the underlying SQL connection.
func (db *DB) Conn() *sql.DB {
	return db.conn
}

// BeginTransaction starts a new database transaction.
func (db *DB) BeginTransaction() (*sql.Tx, error) {
	return db.conn.Begin()
}

// SQLExecer is an interface for executing SQL statements, satisfied by both *sql.DB and *sql.Tx.
type SQLExecer interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}
