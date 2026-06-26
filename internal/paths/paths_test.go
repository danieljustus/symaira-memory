package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigDir_Default(t *testing.T) {
	// Clear XDG_CONFIG_HOME to test default behavior
	t.Setenv("XDG_CONFIG_HOME", "")

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home dir: %v", err)
	}

	dir, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir() error: %v", err)
	}
	expected := filepath.Join(home, ".config", "symmemory")
	if dir != expected {
		t.Errorf("ConfigDir() = %q, want %q", dir, expected)
	}
}

func TestConfigDir_XDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/test-xdg-config")

	dir, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir() error: %v", err)
	}
	expected := "/tmp/test-xdg-config/symmemory"
	if dir != expected {
		t.Errorf("ConfigDir() = %q, want %q", dir, expected)
	}
}

func TestDataDir_Default(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home dir: %v", err)
	}

	dir, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir() error: %v", err)
	}
	expected := filepath.Join(home, ".local", "share", "symmemory")
	if dir != expected {
		t.Errorf("DataDir() = %q, want %q", dir, expected)
	}
}

func TestDataDir_XDG(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/tmp/test-xdg-data")

	dir, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir() error: %v", err)
	}
	expected := "/tmp/test-xdg-data/symmemory"
	if dir != expected {
		t.Errorf("DataDir() = %q, want %q", dir, expected)
	}
}

func TestSecretPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/test-xdg-config")

	path, err := SecretPath("jwt.secret")
	if err != nil {
		t.Fatalf("SecretPath() error: %v", err)
	}
	expected := "/tmp/test-xdg-config/symmemory/jwt.secret"
	if path != expected {
		t.Errorf("SecretPath() = %q, want %q", path, expected)
	}
}

func TestSecretPath_CustomName(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/test-xdg-config")

	path, err := SecretPath("api.key")
	if err != nil {
		t.Fatalf("SecretPath() error: %v", err)
	}
	expected := "/tmp/test-xdg-config/symmemory/api.key"
	if path != expected {
		t.Errorf("SecretPath() = %q, want %q", path, expected)
	}
}

func TestDatabasePath_Default(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home dir: %v", err)
	}

	path, err := DatabasePath()
	if err != nil {
		t.Fatalf("DatabasePath() error: %v", err)
	}
	expected := filepath.Join(home, ".local", "share", "symmemory", "default.db")
	if path != expected {
		t.Errorf("DatabasePath() = %q, want %q", path, expected)
	}
}

func TestDatabasePath_XDG(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/tmp/test-xdg-data")

	path, err := DatabasePath()
	if err != nil {
		t.Fatalf("DatabasePath() error: %v", err)
	}
	expected := "/tmp/test-xdg-data/symmemory/default.db"
	if path != expected {
		t.Errorf("DatabasePath() = %q, want %q", path, expected)
	}
}

func TestEnsureConfigDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	result, err := EnsureConfigDir()
	if err != nil {
		t.Fatalf("EnsureConfigDir() error: %v", err)
	}

	info, err := os.Stat(result)
	if err != nil {
		t.Fatalf("EnsureConfigDir() did not create directory: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("EnsureConfigDir() created %q, want a directory", result)
	}
}

func TestEnsureDataDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)

	result, err := EnsureDataDir()
	if err != nil {
		t.Fatalf("EnsureDataDir() error: %v", err)
	}

	info, err := os.Stat(result)
	if err != nil {
		t.Fatalf("EnsureDataDir() did not create directory: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("EnsureDataDir() created %q, want a directory", result)
	}
}
