package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/BurntSushi/toml"
)

var (
	cachedCfg  *Config
	cachedOnce sync.Once
	cachedErr  error
)

// Config holds all runtime configuration loaded from TOML files.
type Config struct {
	Database DatabaseConfig `toml:"database"`
	Ollama   OllamaConfig   `toml:"ollama"`
	JWT      JWTConfig      `toml:"jwt"`
	Security SecurityConfig `toml:"security"`
	Server   ServerConfig   `toml:"server"`
}

type DatabaseConfig struct {
	Path string `toml:"path"`
}

type OllamaConfig struct {
	URL   string `toml:"url"`
	Model string `toml:"model"`
}

type JWTConfig struct {
	SecretPath string `toml:"secret_path"`
}

type SecurityConfig struct {
	PIIEnabled *bool `toml:"pii_enabled"`
}

type ServerConfig struct {
	HTTPPort int `toml:"http_port"`
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

// Load reads the global config from ~/.config/symmemory/config.toml,
// then merges a project-level .symmemory.toml override if present.
// The config is loaded once and cached for subsequent calls.
func Load() (*Config, error) {
	cachedOnce.Do(func() {
		cachedCfg, cachedErr = loadOnce()
	})
	return cachedCfg, cachedErr
}

// Reload reads a fresh config from disk (global + project files) and applies
// environment variable overrides. Unlike Load, it never returns a cached value.
// Intended for long-running servers that need to pick up config changes without
// restarting.
func Reload() (*Config, error) {
	return loadOnce()
}

// resetCache clears the cached config so the next Load() call reads from disk again.
// It is used only by tests.
func resetCache() {
	cachedCfg = nil
	cachedErr = nil
	cachedOnce = sync.Once{}
}

func loadOnce() (*Config, error) {
	cfg := Defaults()

	home, err := os.UserHomeDir()
	if err != nil {
		return cfg, fmt.Errorf("cannot determine home directory: %w", err)
	}

	globalPath := filepath.Join(home, ".config", "symmemory", "config.toml")
	if err := mergeFile(cfg, globalPath); err != nil {
		return cfg, fmt.Errorf("global config error: %w", err)
	}

	cwd, err := os.Getwd()
	if err == nil {
		projectPath := filepath.Join(cwd, ".symmemory.toml")
		if err := mergeFile(cfg, projectPath); err != nil {
			return cfg, fmt.Errorf("project config error: %w", err)
		}
	}

	// Environment variables override TOML values
	if v := os.Getenv("SYMMEMORY_DB_PATH"); v != "" {
		cfg.Database.Path = v
	}
	if v := os.Getenv("OLLAMA_API_URL"); v != "" {
		cfg.Ollama.URL = v
	}
	if v := os.Getenv("OLLAMA_MODEL"); v != "" {
		cfg.Ollama.Model = v
	}

	return cfg, nil
}

// mergeFile decodes a TOML file into cfg, overriding any matching keys.
// Missing files are silently skipped.
func mergeFile(cfg *Config, path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	var overlay Config
	if _, err := toml.DecodeFile(path, &overlay); err != nil {
		return fmt.Errorf("failed to parse %s: %w", path, err)
	}

	if overlay.Database.Path != "" {
		cfg.Database.Path = overlay.Database.Path
	}
	if overlay.Ollama.URL != "" {
		cfg.Ollama.URL = overlay.Ollama.URL
	}
	if overlay.Ollama.Model != "" {
		cfg.Ollama.Model = overlay.Ollama.Model
	}
	if overlay.JWT.SecretPath != "" {
		cfg.JWT.SecretPath = overlay.JWT.SecretPath
	}
	if overlay.Server.HTTPPort != 0 {
		cfg.Server.HTTPPort = overlay.Server.HTTPPort
	}
	// PII can be explicitly disabled only when the key is present in the TOML file
	if overlay.Security.PIIEnabled != nil {
		cfg.Security.PIIEnabled = overlay.Security.PIIEnabled
	}

	return nil
}
