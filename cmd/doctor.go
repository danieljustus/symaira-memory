package cmd

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/spf13/cobra"
)

type checkResult struct {
	name   string
	passed bool
	detail string
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check system health and diagnose common issues",
	Long: `Run a series of health checks on your Symaira Memory installation.
Reports pass/fail for database, Ollama, JWT secrets, configuration, and file permissions.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var results []checkResult

		results = append(results, checkDatabase())
		results = append(results, checkOllama())
		results = append(results, checkJWTSecret())
		results = append(results, checkConfig())
		results = append(results, checkFilePermissions())

		allPassed := true
		for _, r := range results {
			icon := "✅"
			if !r.passed {
				icon = "❌"
				allPassed = false
			}
			fmt.Printf("%s %s", icon, r.name)
			if r.detail != "" {
				fmt.Printf(": %s", r.detail)
			}
			fmt.Println()
		}

		fmt.Println()
		if allPassed {
			fmt.Println("All checks passed.")
		} else {
			fmt.Println("Some checks failed. Review the issues above.")
		}

		if !allPassed {
			os.Exit(1)
		}
		return nil
	},
}

func checkDatabase() checkResult {
	cfg := GetConfig()
	if cfg == nil {
		cfg = config.Defaults()
	}

	database, err := db.Open(cfg)
	if err != nil {
		return checkResult{name: "Database", passed: false, detail: fmt.Sprintf("cannot open: %v", err)}
	}
	defer database.Close()

	var count int
	err = database.Conn().QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count)
	if err != nil {
		return checkResult{name: "Database", passed: false, detail: fmt.Sprintf("migrations table missing: %v", err)}
	}

	if count == 0 {
		return checkResult{name: "Database", passed: false, detail: "no migrations applied"}
	}

	return checkResult{name: "Database", passed: true, detail: fmt.Sprintf("%d migrations applied", count)}
}

func checkOllama() checkResult {
	cfg := GetConfig()
	if cfg == nil {
		cfg = config.Defaults()
	}

	url := cfg.Ollama.URL
	if url == "" {
		url = "http://localhost:11434/api/embeddings"
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(strings.TrimSuffix(url, "/embeddings"))
	if err != nil {
		return checkResult{name: "Ollama", passed: false, detail: fmt.Sprintf("not reachable: %v", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return checkResult{name: "Ollama", passed: true, detail: "reachable"}
	}
	return checkResult{name: "Ollama", passed: false, detail: fmt.Sprintf("returned status %d", resp.StatusCode)}
}

func checkJWTSecret() checkResult {
	cfg := GetConfig()
	if cfg == nil {
		cfg = config.Defaults()
	}

	envSecret := os.Getenv("JWT_SECRET_KEY")

	secretPath := cfg.JWT.SecretPath
	if secretPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return checkResult{name: "JWT Secret", passed: false, detail: fmt.Sprintf("cannot determine home dir: %v", err)}
		}
		secretPath = filepath.Join(home, ".config", "symmemory", "jwt.secret")
	}

	_, err := os.Stat(secretPath)
	fileExists := err == nil

	if cfg.JWT.Secret != "" {
		return checkResult{name: "JWT Secret", passed: true, detail: "configured via vault://"}
	}
	if envSecret != "" {
		return checkResult{name: "JWT Secret", passed: true, detail: "configured via environment variable"}
	}
	if fileExists {
		return checkResult{name: "JWT Secret", passed: true, detail: "file exists"}
	}

	return checkResult{name: "JWT Secret", passed: true, detail: "will be auto-generated on first use"}
}

func checkConfig() checkResult {
	home, err := os.UserHomeDir()
	if err != nil {
		return checkResult{name: "Configuration", passed: false, detail: fmt.Sprintf("cannot determine home dir: %v", err)}
	}

	configPath := filepath.Join(home, ".config", "symmemory", "config.toml")
	_, err = os.Stat(configPath)
	if os.IsNotExist(err) {
		return checkResult{name: "Configuration", passed: true, detail: "using defaults (no config file)"}
	}
	if err != nil {
		return checkResult{name: "Configuration", passed: false, detail: fmt.Sprintf("cannot read: %v", err)}
	}

	_, err = config.Load()
	if err != nil {
		return checkResult{name: "Configuration", passed: false, detail: fmt.Sprintf("invalid: %v", err)}
	}

	return checkResult{name: "Configuration", passed: true, detail: "valid"}
}

func checkFilePermissions() checkResult {
	home, err := os.UserHomeDir()
	if err != nil {
		return checkResult{name: "File Permissions", passed: false, detail: fmt.Sprintf("cannot determine home dir: %v", err)}
	}

	dbDir := filepath.Join(home, ".local", "share", "symmemory")
	info, err := os.Stat(dbDir)
	if os.IsNotExist(err) {
		return checkResult{name: "File Permissions", passed: true, detail: "directory not yet created"}
	}
	if err != nil {
		return checkResult{name: "File Permissions", passed: false, detail: fmt.Sprintf("cannot stat: %v", err)}
	}

	perm := info.Mode().Perm()
	if perm != 0700 {
		return checkResult{name: "File Permissions", passed: false, detail: fmt.Sprintf("directory is %o, expected 0700", perm)}
	}

	dbPath := filepath.Join(dbDir, "default.db")
	dbInfo, err := os.Stat(dbPath)
	if os.IsNotExist(err) {
		return checkResult{name: "File Permissions", passed: true, detail: "directory OK, database not yet created"}
	}
	if err != nil {
		return checkResult{name: "File Permissions", passed: false, detail: fmt.Sprintf("cannot stat db: %v", err)}
	}

	dbPerm := dbInfo.Mode().Perm()
	if dbPerm != 0600 {
		return checkResult{name: "File Permissions", passed: false, detail: fmt.Sprintf("database is %o, expected 0600", dbPerm)}
	}

	secretPath := filepath.Join(home, ".config", "symmemory", "jwt.secret")
	secretInfo, err := os.Stat(secretPath)
	if os.IsNotExist(err) {
		return checkResult{name: "File Permissions", passed: true, detail: "all checked paths OK"}
	}
	if err != nil {
		return checkResult{name: "File Permissions", passed: false, detail: fmt.Sprintf("cannot stat secret: %v", err)}
	}

	secretPerm := secretInfo.Mode().Perm()
	if secretPerm != 0600 {
		return checkResult{name: "File Permissions", passed: false, detail: fmt.Sprintf("secret file is %o, expected 0600", secretPerm)}
	}

	return checkResult{name: "File Permissions", passed: true, detail: "all checked paths OK"}
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
