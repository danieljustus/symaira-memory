package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempConfig(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", name, err)
	}
	return path
}

func TestDefaults(t *testing.T) {
	cfg := Defaults()
	if cfg.Ollama.URL != "http://localhost:11434/api/embeddings" {
		t.Errorf("expected default Ollama URL, got %q", cfg.Ollama.URL)
	}
	if cfg.Ollama.Model != "nomic-embed-text" {
		t.Errorf("expected default Ollama model, got %q", cfg.Ollama.Model)
	}
	if !cfg.Security.PIIEnabled {
		t.Error("expected PII enabled by default")
	}
}

func TestMergeFileOverrides(t *testing.T) {
	dir := t.TempDir()
	writeTempConfig(t, dir, "test.toml", `
[database]
path = "/custom/db/path.db"
[ollama]
url = "http://ollama:11434/api/embeddings"
model = "llama3"
[jwt]
secret_path = "/etc/secrets/jwt.key"
[server]
http_port = 9090
[security]
pii_enabled = false
`)

	cfg := Defaults()
	if err := mergeFile(cfg, filepath.Join(dir, "test.toml")); err != nil {
		t.Fatalf("mergeFile failed: %v", err)
	}

	if cfg.Database.Path != "/custom/db/path.db" {
		t.Errorf("expected database.path override, got %q", cfg.Database.Path)
	}
	if cfg.Ollama.URL != "http://ollama:11434/api/embeddings" {
		t.Errorf("expected ollama.url override, got %q", cfg.Ollama.URL)
	}
	if cfg.Ollama.Model != "llama3" {
		t.Errorf("expected ollama.model override, got %q", cfg.Ollama.Model)
	}
	if cfg.JWT.SecretPath != "/etc/secrets/jwt.key" {
		t.Errorf("expected jwt.secret_path override, got %q", cfg.JWT.SecretPath)
	}
	if cfg.Server.HTTPPort != 9090 {
		t.Errorf("expected server.http_port override, got %d", cfg.Server.HTTPPort)
	}
	if cfg.Security.PIIEnabled {
		t.Error("expected PII disabled after override")
	}
}

func TestMergeFileMissing(t *testing.T) {
	cfg := Defaults()
	err := mergeFile(cfg, "/nonexistent/path/config.toml")
	if err != nil {
		t.Errorf("expected nil error for missing file, got: %v", err)
	}
}

func TestMergeFilePartial(t *testing.T) {
	dir := t.TempDir()
	writeTempConfig(t, dir, "partial.toml", `
[ollama]
model = "custom-model"
`)

	cfg := Defaults()
	if err := mergeFile(cfg, filepath.Join(dir, "partial.toml")); err != nil {
		t.Fatalf("mergeFile failed: %v", err)
	}

	if cfg.Ollama.Model != "custom-model" {
		t.Errorf("expected model override, got %q", cfg.Ollama.Model)
	}
	if cfg.Ollama.URL != "http://localhost:11434/api/embeddings" {
		t.Errorf("expected default URL preserved, got %q", cfg.Ollama.URL)
	}
}

func TestLoadGlobalConfig(t *testing.T) {
	dir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	defer os.Setenv("HOME", oldHome)

	configDir := filepath.Join(dir, ".config", "symmemory")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	writeTempConfig(t, configDir, "config.toml", `
[ollama]
url = "http://custom:1234/api/embeddings"
[server]
http_port = 8080
`)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Ollama.URL != "http://custom:1234/api/embeddings" {
		t.Errorf("expected global ollama.url, got %q", cfg.Ollama.URL)
	}
	if cfg.Server.HTTPPort != 8080 {
		t.Errorf("expected global server.http_port, got %d", cfg.Server.HTTPPort)
	}
}

func TestLoadProjectOverride(t *testing.T) {
	dir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	defer os.Setenv("HOME", oldHome)

	configDir := filepath.Join(dir, ".config", "symmemory")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	writeTempConfig(t, configDir, "config.toml", `
[ollama]
model = "global-model"
`)

	oldCwd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldCwd)

	writeTempConfig(t, dir, ".symmemory.toml", `
[ollama]
model = "project-model"
`)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Ollama.Model != "project-model" {
		t.Errorf("expected project override, got %q", cfg.Ollama.Model)
	}
}

func TestDefaultsArePureGo(t *testing.T) {
	cfg := Defaults()
	_ = cfg.Database.Path
	_ = cfg.Ollama.URL
	_ = cfg.JWT.SecretPath
}
