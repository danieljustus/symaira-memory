package cmd

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/danieljustus/symaira-memory/internal/security"
	"github.com/spf13/cobra"
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

var exportCmd = &cobra.Command{
	Use:   "export [destination.tar.gz]",
	Short: "Export local SQLite memory database to a compressed backup",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		destPath := args[0]

		// Find source database path
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to resolve user home: %v\n", err)
			os.Exit(1)
		}

		dbDir := filepath.Join(home, ".local", "share", "symmemory")
		dbPath := filepath.Join(dbDir, "default.db")

		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Error: database file does not exist yet. Add memories first!\n")
			os.Exit(1)
		}

		// Read SQLite database file bytes
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

		// Write to target compressed archive tar.gz
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

		// Save database locally, overwriting previous DB
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		dbDir := filepath.Join(home, ".local", "share", "symmemory")
		_ = os.MkdirAll(dbDir, 0700)
		dbPath := filepath.Join(dbDir, "default.db")

		// Write database
		if err := os.WriteFile(dbPath, dbBytes, 0600); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to restore database file: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("⚡ Memory database successfully restored!")
	},
}

// Simple buffer helper to avoid external dependencies
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
