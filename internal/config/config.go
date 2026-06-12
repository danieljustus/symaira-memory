package config

import (
	"github.com/danieljustus/symaira-corekit/configkit"
)

// Config holds all runtime configuration loaded from TOML files.
type Config struct {
	Database DatabaseConfig `json:"database"`
	Ollama   OllamaConfig   `json:"ollama"`
	JWT      JWTConfig      `json:"jwt"`
	Security SecurityConfig `json:"security"`
	Server   ServerConfig   `json:"server"`
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
	PIIEnabled *bool `json:"pii_enabled"`
}

type ServerConfig struct {
	HTTPPort int `json:"http_port"`
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
			PIIEnabled: &trueVal,
		},
		Server: ServerConfig{
			HTTPPort: 0,
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
