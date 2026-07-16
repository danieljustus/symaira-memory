package config

import (
	"github.com/danieljustus/symaira-corekit/configkit"
)

// Config holds all runtime configuration loaded from TOML files.
type Config struct {
	Database      DatabaseConfig      `json:"database"`
	Ollama        OllamaConfig        `json:"ollama"`
	JWT           JWTConfig           `json:"jwt"`
	Security      SecurityConfig      `json:"security"`
	Server        ServerConfig        `json:"server"`
	Consolidation ConsolidationConfig `json:"consolidation"`
	Ranking       RankingConfig       `json:"ranking"`
	Context       ContextConfig       `json:"context"`
	Retention     RetentionConfig     `json:"retention"`
	HybridSearch  HybridSearchConfig  `json:"hybrid_search"`
	Import        ImportConfig        `json:"import"`
}

type DatabaseConfig struct {
	Path string `json:"path"`
}

type OllamaConfig struct {
	URL   string `json:"url"`
	Model string `json:"model"`
}

type JWTConfig struct {
	SecretPath string `json:"secret_path"`
	Secret     string `json:"secret"`
}

type SecurityConfig struct {
	PIIEnabled     *bool    `json:"pii_enabled"`
	TrustedProxies []string `json:"trusted_proxies"`
	// RequireProfile denies write access to JWT subjects without a stored profile.
	// When false (default), unknown subjects keep default access but a warning is logged.
	RequireProfile bool `json:"require_profile"`
}

type ServerConfig struct {
	HTTPPort int `json:"http_port"`
}

type ConsolidationConfig struct {
	Enabled     bool   `json:"enabled"`
	Schedule    string `json:"schedule"`
	IdleTimeout string `json:"idle_timeout"`
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	URL         string `json:"url"`
}

// RankingConfig controls retrieval ranking weights.
type RankingConfig struct {
	RelevanceWeight  float64 `json:"relevance_weight"`  // cosine similarity weight (default 0.6)
	RecencyWeight    float64 `json:"recency_weight"`    // recency decay weight (default 0.2)
	ImportanceWeight float64 `json:"importance_weight"` // importance weight (default 0.2)
	RecencyHalfLife  float64 `json:"recency_half_life"` // half-life in days (default 30)
}

// ContextConfig controls the context assembler.
type ContextConfig struct {
	TokenBudget          int `json:"token_budget"`           // max tokens for assembled context (default 2000)
	WorkingContextTokens int `json:"working_context_tokens"` // budget for working context (default 500)
	SummaryTokens        int `json:"summary_tokens"`         // budget for session summary (default 500)
	RetrievalTokens      int `json:"retrieval_tokens"`       // budget for semantic retrieval (default 1000)
	MaxWorkingTurns      int `json:"max_working_turns"`      // max recent turns to include (default 5)
}

// RetentionConfig controls data lifecycle governance.
type RetentionConfig struct {
	SessionTTL       string `json:"session_ttl"`        // e.g. "24h", "7d" (default "720h" = 30d)
	AutoPurgeEnabled bool   `json:"auto_purge_enabled"` // enable background purge (default false)
	AuditLogEnabled  bool   `json:"audit_log_enabled"`  // enable audit logging (default true)
	AuditRetention   string `json:"audit_retention"`    // how long to keep audit logs (default "720h")
}

// HybridSearchConfig controls hybrid vector + BM25 retrieval.
type HybridSearchConfig struct {
	Enabled      bool    `json:"enabled"`       // enable hybrid search (default true)
	BM25Weight   float64 `json:"bm25_weight"`   // BM25 weight in fusion (default 0.3)
	VectorWeight float64 `json:"vector_weight"` // vector weight in fusion (default 0.7)
	MMREnabled   bool    `json:"mmr_enabled"`   // enable MMR diversity (default false)
	MMRLambda    float64 `json:"mmr_lambda"`    // MMR lambda (0=diversity, 1=relevance, default 0.7)
}

// ImportConfig holds per-tool import settings.
type ImportConfig struct {
	Tools           map[string]ImportToolConfig `json:"tools"`
	ExtractOnImport bool                        `json:"extract_on_import"` // run extraction/summarization on transcript imports
}

// ImportToolConfig holds configuration for a single importer.
type ImportToolConfig struct {
	Path    string            `json:"path"`
	Token   string            `json:"token"`
	Options map[string]string `json:"options"`
}

// Defaults returns a Config with sensible default values.
func Defaults() *Config {
	trueVal := true
	return &Config{
		Ollama: OllamaConfig{
			URL:   "http://localhost:11434/api/embeddings",
			Model: "nomic-embed-text",
		},
		Security: SecurityConfig{
			PIIEnabled:     &trueVal,
			RequireProfile: false,
		},
		Server: ServerConfig{
			HTTPPort: 0,
		},
		Consolidation: ConsolidationConfig{
			Enabled:     true,
			Schedule:    "0 2 * * *",
			IdleTimeout: "30m",
			Provider:    "",
			Model:       "",
		},
		Ranking: RankingConfig{
			RelevanceWeight:  0.6,
			RecencyWeight:    0.2,
			ImportanceWeight: 0.2,
			RecencyHalfLife:  30,
		},
		Context: ContextConfig{
			TokenBudget:          2000,
			WorkingContextTokens: 500,
			SummaryTokens:        500,
			RetrievalTokens:      1000,
			MaxWorkingTurns:      5,
		},
		Retention: RetentionConfig{
			SessionTTL:       "720h",
			AutoPurgeEnabled: false,
			AuditLogEnabled:  true,
			AuditRetention:   "720h",
		},
		HybridSearch: HybridSearchConfig{
			Enabled:      true,
			BM25Weight:   0.3,
			VectorWeight: 0.7,
			MMREnabled:   false,
			MMRLambda:    0.7,
		},
		Import: ImportConfig{
			ExtractOnImport: true,
		},
	}
}

var loader = configkit.NewLoader[Config](
	configkit.Options{
		AppName:   "symmemory",
		EnvPrefix: "SYMMEMORY",
	},
	Defaults,
)

// Load reads the global config from ~/.config/symmemory/config.toml,
// then merges a project-level .symmemory.toml override if present.
// The config is loaded once and cached for subsequent calls.
func Load() (*Config, error) {
	return loader.Load()
}

// Reload reads a fresh config from disk (global + project files) and applies
// environment variable overrides. Unlike Load, it never returns a cached value.
// Intended for long-running servers that need to pick up config changes without
// restarting.
func Reload() (*Config, error) {
	return loader.Reload()
}

// resetCache clears the cached config so the next Load() call reads from disk again.
// It is used only by tests.
func resetCache() {
	loader.ResetCache()
}
