package cmd

import (
	"archive/tar"
	"compress/gzip"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/danieljustus/symaira-memory/internal/security"
	"github.com/spf13/cobra"
	_ "modernc.org/sqlite"
)

var (
	backupPassword string
)

func init() {
	backupCmd.AddCommand(exportCmd)
	backupCmd.AddCommand(importCmd)

	backupCmd.PersistentFlags().StringVarP(&backupPassword, "password", "p", "", "Optional password to encrypt/decrypt the backup payload")
	rootCmd.AddCommand(backupCmd)
}

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Export and import local memory backups (with optional E2E encryption)",
	Long:  `Export or import compressed backups of your local memory database, with optional zero-knowledge AES-256 encryption.`,
}

// sqliteHeaderMagic is the 16-byte prefix of every valid SQLite database file.
var sqliteHeaderMagic = []byte("SQLite format 3\x00")

// checkpointAndClose opens the database, forces a WAL checkpoint to flush all
// WAL-sidecar data into the main database file, and closes the connection.
// After a successful TRUNCATE checkpoint the main .db file is SQLite-consistent.
func checkpointAndClose(dbPath string) error {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("open for checkpoint: %w", err)
	}
	defer db.Close()

	if _, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE);"); err != nil {
		return fmt.Errorf("wal checkpoint: %w", err)
	}
	return nil
}

// validateSQLiteFile checks that data starts with the SQLite header magic and
// can be opened by the pure-Go SQLite driver with a passing integrity_check.
func validateSQLiteFile(data []byte) error {
	if len(data) < len(sqliteHeaderMagic) {
		return fmt.Errorf("database file too small (%d bytes) to be valid", len(data))
	}
	if string(data[:len(sqliteHeaderMagic)]) != string(sqliteHeaderMagic) {
		return fmt.Errorf("missing SQLite header magic; file is not a valid SQLite database")
	}

	tmp, err := os.CreateTemp("", "symmemory-validate-*.db")
	if err != nil {
		return nil
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	if _, err := tmp.Write(data); err != nil {
		return nil
	}
	if err := tmp.Close(); err != nil {
		return nil
	}

	testDB, err := sql.Open("sqlite", tmp.Name())
	if err != nil {
		return fmt.Errorf("failed to open backup payload as SQLite: %w", err)
	}
	defer testDB.Close()

	var result string
	if err := testDB.QueryRow("PRAGMA integrity_check;").Scan(&result); err != nil {
		return fmt.Errorf("integrity_check failed: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("integrity_check returned: %s", result)
	}
	return nil
}

var exportCmd = &cobra.Command{
	Use:   "export [destination.tar.gz]",
	Short: "Export local SQLite memory database to a compressed backup",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		destPath := args[0]

		cfg := GetConfig()
		dbPath := cfg.Database.Path
		if dbPath == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: failed to resolve user home: %v\n", err)
				os.Exit(1)
			}
			dbPath = filepath.Join(home, ".local", "share", "symmemory", "default.db")
		}

		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Error: database file does not exist yet. Add memories first!\n")
			os.Exit(1)
		}

		if err := checkpointAndClose(dbPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error: WAL checkpoint failed: %v\n", err)
			os.Exit(1)
		}

		dbBytes, err := os.ReadFile(dbPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading database: %v\n", err)
			os.Exit(1)
		}

		var finalBytes []byte
		if backupPassword != "" {
			fmt.Println("🔒 Encrypting backup with AES-256-GCM...")
			crypto := security.NewCryptoEngine()
			cipherBytes, err := crypto.Encrypt(dbBytes, backupPassword)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Encryption failure: %v\n", err)
				os.Exit(1)
			}
			finalBytes = cipherBytes
		} else {
			finalBytes = dbBytes
		}

		file, err := os.Create(destPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create target file: %v\n", err)
			os.Exit(1)
		}
		defer file.Close()

		gw := gzip.NewWriter(file)
		defer gw.Close()

		tw := tar.NewWriter(gw)
		defer tw.Close()

		filename := "default.db"
		if backupPassword != "" {
			filename = "default.db.enc"
		}

		header := &tar.Header{
			Name: filename,
			Size: int64(len(finalBytes)),
			Mode: 0600,
		}

		if err := tw.WriteHeader(header); err != nil {
			fmt.Fprintf(os.Stderr, "Tar header failed: %v\n", err)
			os.Exit(1)
		}

		if _, err := tw.Write(finalBytes); err != nil {
			fmt.Fprintf(os.Stderr, "Tar payload write failed: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("⚡ Backup exported successfully to %s!\n", destPath)
	},
}

var importCmd = &cobra.Command{
	Use:   "restore [source.tar.gz]",
	Short: "Restore local SQLite database from a compressed backup",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		sourcePath := args[0]

		file, err := os.Open(sourcePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open source file: %v\n", err)
			os.Exit(1)
		}
		defer file.Close()

		gr, err := gzip.NewReader(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Gzip unpack failed: %v\n", err)
			os.Exit(1)
		}
		defer gr.Close()

		tr := tar.NewReader(gr)
		var payload []byte
		var filename string

		for {
			header, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "Tar extraction failed: %v\n", err)
				os.Exit(1)
			}

			if strings.HasPrefix(header.Name, "default.db") {
				filename = header.Name
				var buf bytesBuffer
				if _, err := io.Copy(&buf, tr); err != nil {
					fmt.Fprintf(os.Stderr, "Copy failure: %v\n", err)
					os.Exit(1)
				}
				payload = buf.Bytes()
				break
			}
		}

		if len(payload) == 0 {
			fmt.Fprintf(os.Stderr, "Error: no database file found in the archive\n")
			os.Exit(1)
		}

		var dbBytes []byte
		if strings.HasSuffix(filename, ".enc") {
			if backupPassword == "" {
				fmt.Fprintf(os.Stderr, "Error: backup is encrypted. Provide decryption password using -p / --password\n")
				os.Exit(1)
			}
			fmt.Println("🔓 Decrypting database payload with AES-256-GCM...")
			crypto := security.NewCryptoEngine()
			plainBytes, err := crypto.Decrypt(payload, backupPassword)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Decryption failure: %v\n", err)
				os.Exit(1)
			}
			dbBytes = plainBytes
		} else {
			dbBytes = payload
		}

		if err := validateSQLiteFile(dbBytes); err != nil {
			fmt.Fprintf(os.Stderr, "Error: backup is not a valid SQLite database: %v\n", err)
			os.Exit(1)
		}

		cfg := GetConfig()
		dbPath := cfg.Database.Path
		if dbPath == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			dbPath = filepath.Join(home, ".local", "share", "symmemory", "default.db")
		}

		dbDir := filepath.Dir(dbPath)
		_ = os.MkdirAll(dbDir, 0700)

		tmpFile, err := os.CreateTemp(dbDir, "symmemory-restore-*.db.tmp")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create staging file: %v\n", err)
			os.Exit(1)
		}
		stagingPath := tmpFile.Name()

		success := false
		defer func() {
			if !success {
				_ = os.Remove(stagingPath)
			}
		}()

		if err := tmpFile.Chmod(0600); err != nil {
			tmpFile.Close()
			fmt.Fprintf(os.Stderr, "Failed to set staging file permissions: %v\n", err)
			os.Exit(1)
		}

		if _, err := tmpFile.Write(dbBytes); err != nil {
			tmpFile.Close()
			fmt.Fprintf(os.Stderr, "Failed to write staging file: %v\n", err)
			os.Exit(1)
		}

		if err := tmpFile.Sync(); err != nil {
			tmpFile.Close()
			fmt.Fprintf(os.Stderr, "Failed to sync staging file: %v\n", err)
			os.Exit(1)
		}

		if err := tmpFile.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to close staging file: %v\n", err)
			os.Exit(1)
		}

		for _, suffix := range []string{"-wal", "-shm"} {
			_ = os.Remove(dbPath + suffix)
		}

		if err := os.Rename(stagingPath, dbPath); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to atomically replace database: %v\n", err)
			os.Exit(1)
		}

		success = true
		fmt.Println("⚡ Memory database successfully restored!")
	},
}

type bytesBuffer struct {
	buf []byte
}

func (b *bytesBuffer) Write(p []byte) (n int, err error) {
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *bytesBuffer) Bytes() []byte {
	return b.buf
}
