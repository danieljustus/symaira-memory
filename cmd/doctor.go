package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/danieljustus/symaira-corekit/exitcodes"
	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/extractor"
	"github.com/spf13/cobra"
)

type checkResult struct {
	name        string
	passed      bool
	warning     bool // non-blocking issue; does not cause exit 1
	detail      string
	remediation string // symmemory command to run; empty means no action needed
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check system health and diagnose common issues",
	Long: `Run a series of health checks on your Symaira Memory installation.
Reports pass/fail for database, Ollama, JWT secrets, configuration, and file permissions.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var results []checkResult

		results = append(results, checkDatabase())
		results = append(results, checkDBSize())
		results = append(results, checkOllama())
		results = append(results, checkEmbeddingBackend())
		results = append(results, checkMemoryCount())
		results = append(results, checkJWTSecret())
		results = append(results, checkConfig())
		results = append(results, checkFilePermissions())
		results = append(results, checkProfiles())

		allPassed := true
		for _, r := range results {
			icon := "✅"
			if r.warning {
				icon = "⚠️"
			} else if !r.passed {
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
			return exitcodes.Wrapf(nil, exitcodes.ExitSoftware, exitcodes.KindInternal, "some health checks failed")
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

func checkDBSize() checkResult {
	cfg := GetConfig()
	if cfg == nil {
		cfg = config.Defaults()
	}

	dbPath := db.ResolvePath(cfg)
	info, err := os.Stat(dbPath)
	if os.IsNotExist(err) {
		return checkResult{name: "DB Size", passed: true, detail: "database not yet created"}
	}
	if err != nil {
		return checkResult{name: "DB Size", passed: false, detail: fmt.Sprintf("cannot stat: %v", err)}
	}

	size := info.Size()
	const (
		warnThreshold  int64 = 500 * 1024 * 1024      // 500 MB
		errorThreshold int64 = 2 * 1024 * 1024 * 1024 // 2 GB
	)

	switch {
	case size >= errorThreshold:
		return checkResult{name: "DB Size", passed: false, detail: fmt.Sprintf("%d MB exceeds 2 GB limit", size/(1024*1024))}
	case size >= warnThreshold:
		return checkResult{
			name:    "DB Size",
			passed:  true,
			warning: true,
			detail:  fmt.Sprintf("%d MB — consider pruning (warns at 500 MB)", size/(1024*1024)),
		}
	}

	return checkResult{name: "DB Size", passed: true, detail: fmt.Sprintf("%d MB", size/(1024*1024))}
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

	return checkOllamaEndpoint(url, cfg.Ollama.Model)
}

func checkOllamaEndpoint(url, model string) checkResult {
	if model == "" {
		model = "nomic-embed-text"
	}

	body, err := json.Marshal(map[string]string{
		"model":  model,
		"prompt": "symmemory health test",
	})
	if err != nil {
		return checkResult{name: "Ollama", passed: false, detail: fmt.Sprintf("cannot build request: %v", err)}
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return checkResult{name: "Ollama", passed: false, detail: fmt.Sprintf("not reachable: %v", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return checkResult{name: "Ollama", passed: false, detail: "model not found or endpoint unavailable"}
	}
	if resp.StatusCode != http.StatusOK {
		return checkResult{name: "Ollama", passed: false, detail: fmt.Sprintf("returned status %d", resp.StatusCode)}
	}

	var result struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return checkResult{name: "Ollama", passed: false, detail: fmt.Sprintf("invalid response: %v", err)}
	}
	if len(result.Embedding) == 0 {
		return checkResult{name: "Ollama", passed: false, detail: "empty embedding in response"}
	}

	return checkResult{name: "Ollama", passed: true, detail: fmt.Sprintf("embedding returned (%d dimensions)", len(result.Embedding))}
}

func checkEmbeddingBackend() checkResult {
	cfg := GetConfig()
	if cfg == nil {
		cfg = config.Defaults()
	}
	eg := extractor.NewEmbeddingsGenerator(cfg)
	backend := eg.ActiveBackend()
	model := eg.Model
	dims := eg.Dimensions()

	if backend == "lexical" {
		return checkResult{
			name:    "Embedding Backend",
			passed:  true,
			warning: true,
			detail:  fmt.Sprintf("lexical fallback (model: %s, dims: %d)", model, dims),
		}
	}
	return checkResult{
		name:   "Embedding Backend",
		passed: true,
		detail: fmt.Sprintf("ollama (model: %s, dims: %d)", model, dims),
	}
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

var commonAgentProfiles = []string{"claude-code", "opencode", "codex"}

func checkMemoryCount() checkResult {
	cfg := GetConfig()
	if cfg == nil {
		cfg = config.Defaults()
	}

	database, err := db.Open(cfg)
	if err != nil {
		return checkResult{name: "Memories", passed: false, detail: fmt.Sprintf("cannot open database: %v", err)}
	}
	defer database.Close()

	var count int
	err = database.Conn().QueryRow("SELECT COUNT(*) FROM memories").Scan(&count)
	if err != nil {
		return checkResult{name: "Memories", passed: false, detail: fmt.Sprintf("cannot count memories: %v", err)}
	}

	return checkResult{name: "Memories", passed: true, detail: fmt.Sprintf("%d memories stored", count)}
}

func checkProfiles() checkResult {
	cfg := GetConfig()
	if cfg == nil {
		cfg = config.Defaults()
	}

	database, err := db.Open(cfg)
	if err != nil {
		return checkResult{name: "Profiles", passed: false, detail: fmt.Sprintf("cannot open database: %v", err)}
	}
	defer database.Close()

	profiles, err := database.ListProfiles()
	if err != nil {
		return checkResult{name: "Profiles", passed: false, detail: fmt.Sprintf("cannot list profiles: %v", err)}
	}

	if len(profiles) == 0 {
		return checkResult{
			name:    "Profiles",
			passed:  true,
			warning: true,
			detail:  "no profiles configured",
		}
	}

	roles := make(map[string]int)
	for _, p := range profiles {
		roles[p.Role]++
	}
	roleSummary := ""
	for role, count := range roles {
		if roleSummary != "" {
			roleSummary += ", "
		}
		roleSummary += fmt.Sprintf("%s=%d", role, count)
	}

	byName := make(map[string]bool, len(profiles))
	for _, p := range profiles {
		byName[p.Name] = true
	}
	var missing []string
	for _, name := range commonAgentProfiles {
		if !byName[name] {
			missing = append(missing, name)
		}
	}

	detail := fmt.Sprintf("%d profile(s) [%s]", len(profiles), roleSummary)
	if len(missing) > 0 {
		detail += fmt.Sprintf("; missing common profiles: %v", missing)
		return checkResult{
			name:    "Profiles",
			passed:  true,
			warning: true,
			detail:  detail,
		}
	}

	return checkResult{name: "Profiles", passed: true, detail: detail}
}

func checkDuplicateCandidates() checkResult {
	cfg := GetConfig()
	if cfg == nil {
		cfg = config.Defaults()
	}

	database, err := db.Open(cfg)
	if err != nil {
		return checkResult{name: "Duplicate Candidates", passed: false, detail: fmt.Sprintf("cannot open database: %v", err)}
	}
	defer database.Close()

	// Gracefully handle missing content_hash column (pre-migration 018 schemas)
	var dupGroupCount int
	err = database.Conn().QueryRow(`
		SELECT COUNT(*) FROM (
			SELECT content_hash FROM memories
			WHERE content_hash != '' AND content_hash IS NOT NULL
			GROUP BY content_hash
			HAVING COUNT(*) > 1
		)
	`).Scan(&dupGroupCount)
	if err != nil {
		if strings.Contains(err.Error(), "no such column") {
			return checkResult{
				name:    "Duplicate Candidates",
				passed:  true,
				warning: true,
				detail:  "content_hash column missing — run migrations to enable dedup checks",
			}
		}
		return checkResult{name: "Duplicate Candidates", passed: false, detail: fmt.Sprintf("query failed: %v", err)}
	}

	if dupGroupCount == 0 {
		return checkResult{name: "Duplicate Candidates", passed: true, detail: "no duplicate candidates found"}
	}

	var dupRowCount int
	err = database.Conn().QueryRow(`
		SELECT COUNT(*) FROM memories
		WHERE content_hash IN (
			SELECT content_hash FROM memories
			WHERE content_hash != '' AND content_hash IS NOT NULL
			GROUP BY content_hash
			HAVING COUNT(*) > 1
		)
	`).Scan(&dupRowCount)
	if err != nil {
		return checkResult{name: "Duplicate Candidates", passed: false, detail: fmt.Sprintf("query failed: %v", err)}
	}

	return checkResult{
		name:        "Duplicate Candidates",
		passed:      true,
		warning:     true,
		detail:      fmt.Sprintf("%d groups, %d memories", dupGroupCount, dupRowCount),
		remediation: "symmemory dream --dry-run",
	}
}

func checkNeverRecalled() checkResult {
	cfg := GetConfig()
	if cfg == nil {
		cfg = config.Defaults()
	}

	database, err := db.Open(cfg)
	if err != nil {
		return checkResult{name: "Never-Recalled (old)", passed: false, detail: fmt.Sprintf("cannot open database: %v", err)}
	}
	defer database.Close()

	// Detect if #409 access-tracking columns exist; gracefully degrade if not.
	var hasRecallCol bool
	err = database.Conn().QueryRow(
		`SELECT COUNT(*) > 0 FROM pragma_table_info('memories') WHERE name='recalled_count'`,
	).Scan(&hasRecallCol)
	if err != nil {
		hasRecallCol = false
	}

	var oldUnrecalled int

	if hasRecallCol {
		err = database.Conn().QueryRow(
			`SELECT COUNT(*) FROM memories WHERE recalled_count = 0 AND created_at < datetime('now', '-30 days')`,
		).Scan(&oldUnrecalled)
	} else {
		// Graceful degradation: use updated_at == created_at as proxy for "never recalled"
		err = database.Conn().QueryRow(
			`SELECT COUNT(*) FROM memories WHERE updated_at = created_at AND created_at < datetime('now', '-30 days')`,
		).Scan(&oldUnrecalled)
	}
	if err != nil {
		return checkResult{name: "Never-Recalled (old)", passed: false, detail: fmt.Sprintf("query failed: %v", err)}
	}

	if oldUnrecalled == 0 {
		return checkResult{name: "Never-Recalled (old)", passed: true, detail: "no old unrecalled memories"}
	}

	suffix := ""
	if !hasRecallCol {
		suffix = " (estimated via updated_at — install #409 for precise tracking)"
	}

	return checkResult{
		name:        "Never-Recalled (old)",
		passed:      true,
		warning:     true,
		detail:      fmt.Sprintf("%d memories older than 30 days never recalled%s", oldUnrecalled, suffix),
		remediation: "symmemory purge --session-ttl 30d --dry-run",
	}
}

func checkDurableRatio() checkResult {
	cfg := GetConfig()
	if cfg == nil {
		cfg = config.Defaults()
	}

	database, err := db.Open(cfg)
	if err != nil {
		return checkResult{name: "Durable Ratio", passed: false, detail: fmt.Sprintf("cannot open database: %v", err)}
	}
	defer database.Close()

	var total int
	err = database.Conn().QueryRow(`SELECT COUNT(*) FROM memories`).Scan(&total)
	if err != nil {
		return checkResult{name: "Durable Ratio", passed: false, detail: fmt.Sprintf("cannot count memories: %v", err)}
	}

	if total == 0 {
		return checkResult{name: "Durable Ratio", passed: true, detail: "no memories stored"}
	}

	var durable int
	err = database.Conn().QueryRow(`SELECT COUNT(*) FROM memories WHERE importance >= 0.7`).Scan(&durable)
	if err != nil {
		// Gracefully handle missing importance column (pre-migration 011 schemas)
		if strings.Contains(err.Error(), "no such column") {
			return checkResult{
				name:    "Durable Ratio",
				passed:  true,
				warning: true,
				detail:  "importance column missing — run migrations to enable ratio checks",
			}
		}
		return checkResult{name: "Durable Ratio", passed: false, detail: fmt.Sprintf("query failed: %v", err)}
	}

	ratio := float64(durable) / float64(total) * 100

	if ratio < 30 && total >= 10 {
		return checkResult{
			name:        "Durable Ratio",
			passed:      true,
			warning:     true,
			detail:      fmt.Sprintf("%.0f%% durable (%d/%d)", ratio, durable, total),
			remediation: "symmemory dream --dry-run",
		}
	}

	return checkResult{
		name:   "Durable Ratio",
		passed: true,
		detail: fmt.Sprintf("%.0f%% durable (%d/%d)", ratio, durable, total),
	}
}

func printNextSteps(results []checkResult) {
	var hasActionable bool
	for _, r := range results {
		if r.remediation != "" {
			if !hasActionable {
				fmt.Println()
				fmt.Println("Next steps:")
				hasActionable = true
			}
			icon := "⚠️"
			if !r.passed {
				icon = "❌"
			}
			fmt.Printf("  %s %s: run `%s`\n", icon, r.name, r.remediation)
		}
	}
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
