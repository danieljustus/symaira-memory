package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/danieljustus/symaira-memory/internal/config"
)

func TestDoctorCommandExists(t *testing.T) {
	cmd := rootCmd
	found := false
	for _, c := range cmd.Commands() {
		if c.Name() == "doctor" {
			found = true
			break
		}
	}
	if !found {
		t.Error("doctor command not registered")
	}
}

func TestCheckDatabase(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "symmemory-doctor-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	cfg := config.Defaults()
	database, err := config.Load()
	if err != nil {
		database = cfg
	}
	_ = database

	SetConfig(cfg)
	result := checkDatabase()
	if !result.passed {
		t.Errorf("checkDatabase failed: %s", result.detail)
	}
}

func TestCheckConfig(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "symmemory-config-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	SetConfig(config.Defaults())
	result := checkConfig()
	if !result.passed {
		t.Errorf("checkConfig failed: %s", result.detail)
	}
}

func TestCheckEmbeddingBackendDefaultOllama(t *testing.T) {
	SetConfig(config.Defaults())
	result := checkEmbeddingBackend()
	if !result.passed {
		t.Errorf("expected checkEmbeddingBackend to pass on fresh config, got: %s", result.detail)
	}
	if result.detail != "ollama" {
		t.Errorf("expected detail 'ollama', got %q", result.detail)
	}
}

func TestCheckFilePermissions(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "symmemory-perm-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	result := checkFilePermissions()
	if !result.passed {
		t.Errorf("checkFilePermissions failed: %s", result.detail)
	}

	dbDir := filepath.Join(tempDir, ".local", "share", "symmemory")
	if err := os.MkdirAll(dbDir, 0700); err != nil {
		t.Fatalf("failed to create db dir: %v", err)
	}

	result = checkFilePermissions()
	if !result.passed {
		t.Errorf("checkFilePermissions failed after creating dir: %s", result.detail)
	}
}
