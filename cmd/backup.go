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
	"syscall"

	"github.com/danieljustus/symaira-corekit/exitcodes"
	"github.com/danieljustus/symaira-memory/internal/security"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	_ "modernc.org/sqlite"
)

var (
	backupPassword     string
	backupPasswordFile string
)

const maxTarEntrySize = 1 << 30 // 1 GiB

func init() {
	backupCmd.AddCommand(exportCmd)
	backupCmd.AddCommand(importCmd)

	backupCmd.PersistentFlags().StringVarP(&backupPassword, "password", "p", "", "Password to encrypt/decrypt the backup payload (deprecated: use --password-file, env var, or stdin prompt)")
	backupCmd.PersistentFlags().StringVar(&backupPasswordFile, "password-file", "", "Read the encryption password from this file")
	rootCmd.AddCommand(backupCmd)
}

// resolveBackupPassword determines the backup password from multiple sources
// in priority order: --password flag (deprecated), --password-file, env var,
// or interactive stdin prompt. Returns the resolved password.
func resolveBackupPassword(operation string) (string, error) {
	// 1. --password flag (deprecated, prints warning)
	if backupPassword != "" {
		fmt.Fprintf(os.Stderr, "Warning: --password / -p flag is deprecated and exposes the password in process listings. Use --password-file, SYMMEMORY_BACKUP_PASSWORD env var, or omit for an interactive prompt.\n")
		return backupPassword, nil
	}

	// 2. --password-file flag
	if backupPasswordFile != "" {
		data, err := os.ReadFile(backupPasswordFile)
		if err != nil {
			return "", fmt.Errorf("failed to read password file %s: %w", backupPasswordFile, err)
		}
		pw := strings.TrimSpace(string(data))
		if pw == "" {
			return "", fmt.Errorf("password file %s is empty", backupPasswordFile)
		}
		return pw, nil
	}

	// 3. Environment variable
	if envPW := os.Getenv("SYMMEMORY_BACKUP_PASSWORD"); envPW != "" {
		return envPW, nil
	}

	// 4. Interactive stdin prompt (TTY only)
	if term.IsTerminal(int(syscall.Stdin)) {
		fmt.Fprintf(os.Stderr, "Enter backup %s password: ", operation)
		pw, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Fprintln(os.Stderr) // newline after hidden input
		if err != nil {
			return "", fmt.Errorf("failed to read password from stdin: %w", err)
		}
		if len(pw) == 0 {
			return "", fmt.Errorf("empty password provided")
		}
		return string(pw), nil
	}

	return "", fmt.Errorf("no password source available: use --password-file, set SYMMEMORY_BACKUP_PASSWORD, or run on an interactive terminal")
}

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Export and import encrypted local memory backups",
	Long:  `Export or import compressed backups of your local memory database. Exports are encrypted with AES-256-GCM and require a password source.`,
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
		return fmt.Errorf("validation I/O error: %w", err)
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("validation I/O error: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("validation I/O error: %w", err)
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
	Short: "Export local SQLite memory database to an encrypted backup",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		destPath := args[0]

		cfg := GetConfig()
		dbPath := cfg.Database.Path
		if dbPath == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to resolve user home directory")
			}
			dbPath = filepath.Join(home, ".local", "share", "symmemory", "default.db")
		}

		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			return exitcodes.Wrapf(nil, exitcodes.ExitNotFound, exitcodes.KindNotFound, "database file does not exist yet; add memories first")
		}

		if err := checkpointAndClose(dbPath); err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "WAL checkpoint failed")
		}

		dbBytes, err := os.ReadFile(dbPath)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to read database file")
		}

		password, err := resolveBackupPassword("encryption")
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitNoInput, exitcodes.KindValidation, "failed to resolve backup password")
		}
		if password == "" {
			return exitcodes.Wrapf(nil, exitcodes.ExitNoInput, exitcodes.KindValidation, "backup export requires an encryption password")
		}

		fmt.Println("🔒 Encrypting backup with AES-256-GCM...")
		crypto := security.NewCryptoEngine()
		finalBytes, err := crypto.Encrypt(dbBytes, password)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "encryption failed")
		}

		file, err := os.Create(destPath)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to create target file")
		}
		defer file.Close()

		gw := gzip.NewWriter(file)
		defer gw.Close()

		tw := tar.NewWriter(gw)
		defer tw.Close()

		header := &tar.Header{
			Name: "default.db.enc",
			Size: int64(len(finalBytes)),
			Mode: 0600,
		}

		if err := tw.WriteHeader(header); err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "tar header write failed")
		}

		if _, err := tw.Write(finalBytes); err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "tar payload write failed")
		}

		fmt.Printf("⚡ Backup exported successfully to %s!\n", destPath)
		return nil
	},
}

var importCmd = &cobra.Command{
	Use:   "restore [source.tar.gz]",
	Short: "Restore local SQLite database from a compressed backup",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sourcePath := args[0]

		file, err := os.Open(sourcePath)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to open source file")
		}
		defer file.Close()

		gr, err := gzip.NewReader(file)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitData, exitcodes.KindValidation, "gzip unpack failed; file may not be a valid gzip archive")
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
				return exitcodes.Wrapf(err, exitcodes.ExitData, exitcodes.KindValidation, "tar extraction failed")
			}

			if strings.HasPrefix(header.Name, "default.db") {
				filename = header.Name
				limited := io.LimitReader(tr, maxTarEntrySize+1)
				var buf bytesBuffer
				if _, err := io.Copy(&buf, limited); err != nil {
					return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to copy archive entry")
				}
				if int64(buf.Len()) > maxTarEntrySize {
					return exitcodes.Wrapf(nil, exitcodes.ExitData, exitcodes.KindValidation, "database entry exceeds maximum allowed size (%d bytes)", maxTarEntrySize)
				}
				payload = buf.Bytes()
				break
			}
		}

		if len(payload) == 0 {
			return exitcodes.Wrapf(nil, exitcodes.ExitNotFound, exitcodes.KindNotFound, "no database file found in the archive")
		}

		var dbBytes []byte
		if strings.HasSuffix(filename, ".enc") {
			password, err := resolveBackupPassword("decryption")
			if err != nil {
				return exitcodes.Wrapf(err, exitcodes.ExitNoInput, exitcodes.KindValidation, "failed to resolve backup password")
			}
			fmt.Println("🔓 Decrypting database payload with AES-256-GCM...")
			crypto := security.NewCryptoEngine()
			plainBytes, err := crypto.Decrypt(payload, password)
			if err != nil {
				return exitcodes.Wrapf(err, exitcodes.ExitNoAuth, exitcodes.KindAuth, "decryption failed; wrong password or corrupted data")
			}
			dbBytes = plainBytes
		} else {
			dbBytes = payload
		}

		if err := validateSQLiteFile(dbBytes); err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitData, exitcodes.KindValidation, "backup is not a valid SQLite database")
		}

		cfg := GetConfig()
		dbPath := cfg.Database.Path
		if dbPath == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to resolve user home directory")
			}
			dbPath = filepath.Join(home, ".local", "share", "symmemory", "default.db")
		}

		dbDir := filepath.Dir(dbPath)
		_ = os.MkdirAll(dbDir, 0700)

		tmpFile, err := os.CreateTemp(dbDir, "symmemory-restore-*.db.tmp")
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to create staging file")
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
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to set staging file permissions")
		}

		if _, err := tmpFile.Write(dbBytes); err != nil {
			tmpFile.Close()
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to write staging file")
		}

		if err := tmpFile.Sync(); err != nil {
			tmpFile.Close()
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to sync staging file")
		}

		if err := tmpFile.Close(); err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to close staging file")
		}

		for _, suffix := range []string{"-wal", "-shm"} {
			_ = os.Remove(dbPath + suffix)
		}

		if err := os.Rename(stagingPath, dbPath); err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to atomically replace database")
		}

		success = true
		fmt.Println("⚡ Memory database successfully restored!")
		return nil
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

func (b *bytesBuffer) Len() int {
	return len(b.buf)
}
