package config

import (
	"github.com/danieljustus/symaira-corekit/configkit"
)

// DegradationConfig controls content degradation levels for greedy budget fill.
type DegradationConfig struct {
	Enabled       bool    `json:"enabled"`        // enable degradation ladder (default true)
	FullTokens    int     `json:"full_tokens"`    // budget per full item (default 200)
	SummaryTokens int     `json:"summary_tokens"` // budget per summary (default 80)
	RefTokens     int     `json:"ref_tokens"`     // budget per reference (default 30)
	SummaryRatio  float64 `json:"summary_ratio"`  // fraction of token budget that drops to summary (default 0.3)
}

// DeltaPackConfig controls content-hash based delta context packs.
type DeltaPackConfig struct {
	Enabled                 bool `json:"enabled"`                    // enable delta pack creation (default true)
	MaxSnapshots            int  `json:"max_snapshots"`              // max snapshots to retain per session (default 10)
	SnapshotOnTokenPct      int  `json:"snapshot_on_token_pct"`      // snapshot when used-token delta > this % (default 20)
	MinSnapshotIntervalSecs int  `json:"min_snapshot_interval_secs"` // minimum seconds between snapshots (default 60)
}

// ProfileLayerConfig controls the synthesized static/dynamic profile layer.
type ProfileLayerConfig struct {
	Enabled       bool `json:"enabled"`         // attach profile layer without search query (default true)
	MaxStaticLen  int  `json:"max_static_len"`  // max chars for static profile fields (default 200)
	MaxDynamicLen int  `json:"max_dynamic_len"` // max chars for dynamic/session fields (default 500)
	TokenBudget   int  `json:"token_budget"`    // token budget for profile layer (default 150)
}

// SessionContextConfig controls source-linked expandable session context.
type SessionContextConfig struct {
	Enabled         bool `json:"enabled"`            // enable expandable session context layer (default true)
	MaxSources      int  `json:"max_sources"`        // max expandable sources to attach (default 5)
	TokenBudget     int  `json:"token_budget"`       // token budget for session context layer (default 300)
	MaxExpandPerSrc int  `json:"max_expand_per_src"` // max expand steps per source (default 3)
}

// Config holds all runtime configuration loaded from TOML files.
type Config struct {
	Database      DatabaseConfig       `json:"database"`
	Ollama        OllamaConfig         `json:"ollama"`
	JWT           JWTConfig            `json:"jwt"`
	Security      SecurityConfig       `json:"security"`
	Server        ServerConfig         `json:"server"`
	Consolidation ConsolidationConfig  `json:"consolidation"`
	Ranking       RankingConfig        `json:"ranking"`
	Context       ContextConfig        `json:"context"`
	Degradation   DegradationConfig    `json:"degradation"`
	DeltaPack     DeltaPackConfig      `json:"delta_pack"`
	ProfileLayer  ProfileLayerConfig   `json:"profile_layer"`
	SessionCtx    SessionContextConfig `json:"session_context"`
	Retention     RetentionConfig      `json:"retention"`
	HybridSearch  HybridSearchConfig   `json:"hybrid_search"`
	Search        SearchConfig         `json:"search"`
	Import        ImportConfig         `json:"import"`
	WorkingMemory WorkingMemoryConfig  `json:"working_memory"`
	MCP           MCPConfig            `json:"mcp"`
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
	RelevanceWeight           float64 `json:"relevance_weight"`            // cosine similarity weight (default 0.6)
	RecencyWeight             float64 `json:"recency_weight"`              // recency decay weight (default 0.2)
	ImportanceWeight          float64 `json:"importance_weight"`           // importance weight (default 0.2)
	AccessReinforcementWeight float64 `json:"access_reinforcement_weight"` // access frequency boost (default 0 = disabled)
	RecencyHalfLife           float64 `json:"recency_half_life"`           // half-life in days (default 30)
	AccessHalfLife            float64 `json:"access_half_life"`            // half-life for last-access recency decay (default 14)
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
	Enabled          bool    `json:"enabled"`            // enable hybrid search (default true)
	BM25Weight       float64 `json:"bm25_weight"`        // BM25 weight in fusion (default 0.3)
	VectorWeight     float64 `json:"vector_weight"`      // vector weight in fusion (default 0.7)
	MMREnabled       bool    `json:"mmr_enabled"`        // enable MMR diversity (default false)
	MMRLambda        float64 `json:"mmr_lambda"`         // MMR lambda (0=diversity, 1=relevance, default 0.7)
	QuantizeToBinary bool    `json:"quantize_to_binary"` // store sign-bit binary vectors for fast Hamming prefilter
	PrefilterEnabled bool    `json:"prefilter_enabled"`  // use Hamming prefilter before cosine scoring (opt-in)
	SparsemaxEnabled bool    `json:"sparsemax_enabled"`  // apply sparsemax (α=2) to fused scores (default false)
	PerArmMultiplier int     `json:"per_arm_multiplier"` // per-arm result cap multiplier before fusion (default 3)
}

// SearchConfig controls default retrieval behavior for memory_search / search.
type SearchConfig struct {
	MinScore                  float64 `json:"min_score"`                   // minimum similarity score; results below are dropped (default 0 = disabled)
	TemporalExtractionEnabled bool    `json:"temporal_extraction_enabled"` // enable inference of time window constraints from query text (default false)
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

// WorkingMemoryConfig controls the working-memory tier lifecycle.
type WorkingMemoryConfig struct {
	TTL              string `json:"ttl"`                // expiration duration for working memories (default "24h")
	MaxItems         int    `json:"max_items"`          // max working memories in context assembly (default 50)
	IncludeInContext bool   `json:"include_in_context"` // include working memories in assembled context (default true)
}

// MCPConfig controls the MCP stdio server behavior.
type MCPConfig struct {
	// ClientID pins the attribution identity (created_by/updated_by) for all
	// MCP writes, winning over the client identity captured from the
	// initialize handshake. Empty means "derive from the handshake, fall
	// back to 'mcp' when nothing is resolvable".
	ClientID string `json:"client_id"`
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
			RelevanceWeight:           0.6,
			RecencyWeight:             0.2,
			ImportanceWeight:          0.2,
			AccessReinforcementWeight: 0.0,
			RecencyHalfLife:           30,
			AccessHalfLife:            14,
		},
		Context: ContextConfig{
			TokenBudget:          2000,
			WorkingContextTokens: 500,
			SummaryTokens:        500,
			RetrievalTokens:      1000,
			MaxWorkingTurns:      5,
		},
		Degradation: DegradationConfig{
			Enabled:       true,
			FullTokens:    200,
			SummaryTokens: 80,
			RefTokens:     30,
			SummaryRatio:  0.3,
		},
		DeltaPack: DeltaPackConfig{
			Enabled:                 true,
			MaxSnapshots:            10,
			SnapshotOnTokenPct:      20,
			MinSnapshotIntervalSecs: 60,
		},
		ProfileLayer: ProfileLayerConfig{
			Enabled:       true,
			MaxStaticLen:  200,
			MaxDynamicLen: 500,
			TokenBudget:   150,
		},
		SessionCtx: SessionContextConfig{
			Enabled:         true,
			MaxSources:      5,
			TokenBudget:     300,
			MaxExpandPerSrc: 3,
		},
		Retention: RetentionConfig{
			SessionTTL:       "720h",
			AutoPurgeEnabled: false,
			AuditLogEnabled:  true,
			AuditRetention:   "720h",
		},
		HybridSearch: HybridSearchConfig{
			Enabled:          true,
			BM25Weight:       0.3,
			VectorWeight:     0.7,
			MMREnabled:       false,
			MMRLambda:        0.7,
			SparsemaxEnabled: false,
			PerArmMultiplier: 3,
		},
		Import: ImportConfig{
			ExtractOnImport: true,
		},
		Search: SearchConfig{
			MinScore: 0,
		},
		WorkingMemory: WorkingMemoryConfig{
			TTL:              "24h",
			MaxItems:         50,
			IncludeInContext: true,
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
