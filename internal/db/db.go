package db

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/danieljustus/symaira-corekit/sqlitekit"
	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/paths"
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
		var err error
		dbPath, err = paths.DatabasePath()
		if err != nil {
			return nil, fmt.Errorf("failed to resolve database path: %w", err)
		}
	}

	conn, err := sqlitekit.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)
	conn.SetConnMaxLifetime(0)

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
	path, err := paths.DatabasePath()
	if err != nil {
		return ""
	}
	return path
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

// Metrics returns current connection pool metrics.
func (db *DB) Metrics() DBMetrics {
	stats := db.conn.Stats()
	return DBMetrics{
		MaxOpenConnections: stats.MaxOpenConnections,
		OpenConnections:    stats.OpenConnections,
		InUse:              stats.InUse,
		Idle:               stats.Idle,
		WaitCount:          stats.WaitCount,
		WaitDuration:       stats.WaitDuration,
		MaxIdleClosed:      stats.MaxIdleClosed,
		MaxIdleTimeClosed:  stats.MaxIdleTimeClosed,
		MaxLifetimeClosed:  stats.MaxLifetimeClosed,
	}
}

// DBMetrics holds connection pool metrics.
type DBMetrics struct {
	MaxOpenConnections int
	OpenConnections    int
	InUse              int
	Idle               int
	WaitCount          int64
	WaitDuration       time.Duration
	MaxIdleClosed      int64
	MaxIdleTimeClosed  int64
	MaxLifetimeClosed  int64
}

// SQLExecer is an interface for executing SQL statements, satisfied by both *sql.DB and *sql.Tx.
type SQLExecer interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}
