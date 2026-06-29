package cmd

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/google/uuid"
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
	if result.warning {
		t.Error("expected no warning for ollama backend")
	}
	if !strings.Contains(result.detail, "ollama") {
		t.Errorf("expected 'ollama' in detail, got %q", result.detail)
	}
	if !strings.Contains(result.detail, "model: nomic-embed-text") {
		t.Errorf("expected model name in detail, got %q", result.detail)
	}
	if !strings.Contains(result.detail, "dims: 768") {
		t.Errorf("expected dimensions in detail, got %q", result.detail)
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

func TestCheckOllamaEndpointSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"embedding":[0.1,0.2,0.3]}`)
	}))
	defer server.Close()

	result := checkOllamaEndpoint(server.URL, "nomic-embed-text")
	if !result.passed {
		t.Fatalf("expected check to pass, got %q", result.detail)
	}
}

func TestCheckOllamaEndpointDefaultURLEndsWithEmbeddings(t *testing.T) {
	var requestedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"embedding":[0.1,0.2,0.3]}`)
	}))
	defer server.Close()

	checkOllamaEndpoint(server.URL+"/api/embeddings", "nomic-embed-text")
	if requestedPath != "/api/embeddings" {
		t.Errorf("expected request path /api/embeddings, got %q", requestedPath)
	}
}

func TestCheckOllamaEndpointNotReachable(t *testing.T) {
	result := checkOllamaEndpoint("http://127.0.0.1:1/api/embeddings", "nomic-embed-text")
	if result.passed {
		t.Fatal("expected check to fail for unreachable server")
	}
	if result.detail == "" || result.detail == "returned status 404" {
		t.Errorf("expected unreachable detail, got %q", result.detail)
	}
}

func TestCheckOllamaEndpointModelMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	result := checkOllamaEndpoint(server.URL, "missing-model")
	if result.passed {
		t.Fatal("expected check to fail for missing model")
	}
}

func TestCheckOllamaEndpointEmptyEmbedding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"embedding":[]}`)
	}))
	defer server.Close()

	result := checkOllamaEndpoint(server.URL, "nomic-embed-text")
	if result.passed {
		t.Fatal("expected check to fail for empty embedding")
	}
}

func newTestDB(t *testing.T) (string, *config.Config) {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "symmemory-doctor-profiles-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tempDir) })

	cfg := config.Defaults()
	cfg.Database.Path = filepath.Join(tempDir, "test.db")
	return tempDir, cfg
}

func TestCheckProfilesEmpty(t *testing.T) {
	_, cfg := newTestDB(t)
	SetConfig(cfg)

	result := checkProfiles()
	if !result.passed {
		t.Errorf("expected pass, got failed: %s", result.detail)
	}
	if !result.warning {
		t.Error("expected warning for empty profiles")
	}
	if result.detail != "no profiles configured" {
		t.Errorf("unexpected detail: %s", result.detail)
	}
}

func TestCheckProfilesAllCommonPresent(t *testing.T) {
	_, cfg := newTestDB(t)
	SetConfig(cfg)

	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("cannot open test db: %v", err)
	}
	defer database.Close()

	for _, name := range commonAgentProfiles {
		p := &db.Profile{
			ID:   uuid.New().String(),
			Name: name,
			Type: "agent",
			Role: "readwrite",
		}
		if err := database.SaveProfile(p); err != nil {
			t.Fatalf("cannot save profile %s: %v", name, err)
		}
	}

	result := checkProfiles()
	if !result.passed {
		t.Errorf("expected pass, got failed: %s", result.detail)
	}
	if result.warning {
		t.Error("expected no warning when all common profiles present")
	}
	if !strings.Contains(result.detail, fmt.Sprintf("%d profile(s)", len(commonAgentProfiles))) {
		t.Errorf("expected profile count in detail, got: %s", result.detail)
	}
}

func TestCheckProfilesSomeMissing(t *testing.T) {
	_, cfg := newTestDB(t)
	SetConfig(cfg)

	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("cannot open test db: %v", err)
	}
	defer database.Close()

	p := &db.Profile{
		ID:   uuid.New().String(),
		Name: "claude-code",
		Type: "agent",
		Role: "readwrite",
	}
	if err := database.SaveProfile(p); err != nil {
		t.Fatalf("cannot save profile: %v", err)
	}

	result := checkProfiles()
	if !result.passed {
		t.Errorf("expected pass, got failed: %s", result.detail)
	}
	if !result.warning {
		t.Error("expected warning when common profiles are missing")
	}
	if !strings.Contains(result.detail, "missing common profiles") {
		t.Errorf("expected missing-common-profiles note, got: %s", result.detail)
	}
}

func TestCheckProfilesRolesSummary(t *testing.T) {
	_, cfg := newTestDB(t)
	SetConfig(cfg)

	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("cannot open test db: %v", err)
	}
	defer database.Close()

	profiles := []db.Profile{
		{ID: uuid.New().String(), Name: "agent-a", Type: "agent", Role: "read"},
		{ID: uuid.New().String(), Name: "agent-b", Type: "agent", Role: "readwrite"},
		{ID: uuid.New().String(), Name: "agent-c", Type: "agent", Role: "read"},
	}
	for i := range profiles {
		if err := database.SaveProfile(&profiles[i]); err != nil {
			t.Fatalf("cannot save profile: %v", err)
		}
	}

	result := checkProfiles()
	if !result.passed {
		t.Errorf("expected pass, got failed: %s", result.detail)
	}
	if !strings.Contains(result.detail, "read=2") || !strings.Contains(result.detail, "readwrite=1") {
		t.Errorf("expected role summary in detail, got: %s", result.detail)
	}
}

func TestCheckProfilesCustomNonAgentProfiles(t *testing.T) {
	_, cfg := newTestDB(t)
	SetConfig(cfg)

	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("cannot open test db: %v", err)
	}
	defer database.Close()

	p := &db.Profile{
		ID:   uuid.New().String(),
		Name: "my-custom-agent",
		Type: "agent",
		Role: "admin",
	}
	if err := database.SaveProfile(p); err != nil {
		t.Fatalf("cannot save profile: %v", err)
	}

	result := checkProfiles()
	if !result.passed {
		t.Errorf("expected pass, got failed: %s", result.detail)
	}
	if !result.warning {
		t.Error("expected warning when common profiles missing")
	}
	if !strings.Contains(result.detail, "1 profile(s)") {
		t.Errorf("expected profile count, got: %s", result.detail)
	}
}

func TestCheckDBSizeNotCreated(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "symmemory-doctor-dbsize-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	SetConfig(config.Defaults())
	result := checkDBSize()
	if !result.passed {
		t.Errorf("expected pass for non-existent db, got failed: %s", result.detail)
	}
	if !strings.Contains(result.detail, "not yet created") {
		t.Errorf("expected 'not yet created' in detail, got %q", result.detail)
	}
}

func TestCheckDBSizeSmall(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "symmemory-doctor-dbsize-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	if err := os.WriteFile(dbPath, make([]byte, 1024), 0600); err != nil {
		t.Fatalf("failed to create test db file: %v", err)
	}

	cfg := config.Defaults()
	cfg.Database.Path = dbPath
	SetConfig(cfg)

	result := checkDBSize()
	if !result.passed {
		t.Errorf("expected pass for small db, got failed: %s", result.detail)
	}
	if result.warning {
		t.Error("expected no warning for small db")
	}
	if !strings.Contains(result.detail, "0 MB") {
		t.Errorf("expected '0 MB' in detail, got %q", result.detail)
	}
}

func TestCheckDBSizeWarn(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "symmemory-doctor-dbsize-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	f, err := os.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test db file: %v", err)
	}
	size := int64(600 * 1024 * 1024)
	if err := f.Truncate(size); err != nil {
		f.Close()
		t.Fatalf("failed to truncate: %v", err)
	}
	f.Close()

	cfg := config.Defaults()
	cfg.Database.Path = dbPath
	SetConfig(cfg)

	result := checkDBSize()
	if !result.passed {
		t.Errorf("expected pass (warning only), got failed: %s", result.detail)
	}
	if !result.warning {
		t.Error("expected warning for >500MB db")
	}
	if !strings.Contains(result.detail, "600 MB") {
		t.Errorf("expected '600 MB' in detail, got %q", result.detail)
	}
}

func TestCheckDBSizeError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "symmemory-doctor-dbsize-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	f, err := os.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test db file: %v", err)
	}
	size := int64(3 * 1024 * 1024 * 1024)
	if err := f.Truncate(size); err != nil {
		f.Close()
		t.Fatalf("failed to truncate: %v", err)
	}
	f.Close()

	cfg := config.Defaults()
	cfg.Database.Path = dbPath
	SetConfig(cfg)

	result := checkDBSize()
	if result.passed {
		t.Error("expected failure for >2GB db")
	}
	if result.warning {
		t.Error("expected error, not warning, for >2GB db")
	}
	if !strings.Contains(result.detail, "3072 MB") {
		t.Errorf("expected '3072 MB' in detail, got %q", result.detail)
	}
}

func TestCheckEmbeddingBackendLexicalFallback(t *testing.T) {
	SetConfig(config.Defaults())
	result := checkEmbeddingBackend()

	if strings.Contains(result.detail, "lexical fallback") {
		if !result.warning {
			t.Error("expected warning for lexical fallback")
		}
		if !strings.Contains(result.detail, "model: nomic-embed-text") {
			t.Errorf("expected model name in detail, got %q", result.detail)
		}
		if !strings.Contains(result.detail, "dims: 768") {
			t.Errorf("expected dimensions in detail, got %q", result.detail)
		}
		return
	}

	if strings.Contains(result.detail, "ollama") {
		if result.warning {
			t.Error("expected no warning for ollama backend")
		}
		if !strings.Contains(result.detail, "model: nomic-embed-text") {
			t.Errorf("expected model name in detail, got %q", result.detail)
		}
		if !strings.Contains(result.detail, "dims: 768") {
			t.Errorf("expected dimensions in detail, got %q", result.detail)
		}
		return
	}

	t.Errorf("expected 'ollama' or 'lexical fallback' in detail, got %q", result.detail)
}

func TestCheckMemoryCountEmpty(t *testing.T) {
	_, cfg := newTestDB(t)
	SetConfig(cfg)

	result := checkMemoryCount()
	if !result.passed {
		t.Errorf("expected pass, got failed: %s", result.detail)
	}
	if !strings.Contains(result.detail, "0 memories stored") {
		t.Errorf("expected '0 memories stored' in detail, got %q", result.detail)
	}
}

func TestCheckMemoryCountWithData(t *testing.T) {
	_, cfg := newTestDB(t)
	SetConfig(cfg)

	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("cannot open test db: %v", err)
	}
	defer database.Close()

	for i := 0; i < 5; i++ {
		m := &db.Memory{
			ID:      fmt.Sprintf("test-mem-%d", i),
			Content: fmt.Sprintf("test memory %d", i),
			Scope:   "global",
		}
		if err := database.SaveMemory(m); err != nil {
			t.Fatalf("cannot save memory: %v", err)
		}
	}

	result := checkMemoryCount()
	if !result.passed {
		t.Errorf("expected pass, got failed: %s", result.detail)
	}
	if !strings.Contains(result.detail, "5 memories stored") {
		t.Errorf("expected '5 memories stored' in detail, got %q", result.detail)
	}
}

// --- Tests for checkOllama (coverage target: 0% → meaningful) ---

func TestCheckOllamaDefaultConfig(t *testing.T) {
	SetConfig(config.Defaults())
	result := checkOllama()
	if result.passed {
		return
	}
	if !strings.Contains(result.detail, "not reachable") && !strings.Contains(result.detail, "returned status") {
		t.Errorf("expected connection or status error detail, got %q", result.detail)
	}
}

func TestCheckOllamaCustomURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"embedding":[0.1,0.2,0.3]}`)
	}))
	defer server.Close()

	cfg := config.Defaults()
	cfg.Ollama.URL = server.URL + "/api/embeddings"
	cfg.Ollama.Model = "nomic-embed-text"
	SetConfig(cfg)

	result := checkOllama()
	if !result.passed {
		t.Errorf("expected pass with custom URL, got: %s", result.detail)
	}
	if !strings.Contains(result.detail, "embedding returned") {
		t.Errorf("expected 'embedding returned' in detail, got %q", result.detail)
	}
}

func TestCheckOllamaEmptyURLUsesDefault(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"embedding":[0.1,0.2,0.3]}`)
	}))
	defer server.Close()

	cfg := config.Defaults()
	cfg.Ollama.URL = ""
	cfg.Ollama.Model = "nomic-embed-text"
	SetConfig(cfg)

	_ = server
	result := checkOllama()
	if result.name != "Ollama" {
		t.Errorf("expected name 'Ollama', got %q", result.name)
	}
}

func TestCheckOllamaUnreachableServer(t *testing.T) {
	cfg := config.Defaults()
	cfg.Ollama.URL = "http://127.0.0.1:1/api/embeddings"
	cfg.Ollama.Model = "nomic-embed-text"
	SetConfig(cfg)

	result := checkOllama()
	if result.passed {
		t.Fatal("expected failure for unreachable server")
	}
	if !strings.Contains(result.detail, "not reachable") {
		t.Errorf("expected 'not reachable' in detail, got %q", result.detail)
	}
}

func TestCheckOllamaModelNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	cfg := config.Defaults()
	cfg.Ollama.URL = server.URL
	cfg.Ollama.Model = "nonexistent-model"
	SetConfig(cfg)

	result := checkOllama()
	if result.passed {
		t.Fatal("expected failure for missing model")
	}
	if !strings.Contains(result.detail, "model not found") {
		t.Errorf("expected 'model not found' in detail, got %q", result.detail)
	}
}

func TestCheckOllamaEmptyModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"embedding":[0.1]}`)
	}))
	defer server.Close()

	cfg := config.Defaults()
	cfg.Ollama.URL = server.URL
	cfg.Ollama.Model = ""
	SetConfig(cfg)

	result := checkOllama()
	if !result.passed {
		t.Errorf("expected pass with empty model (should default), got: %s", result.detail)
	}
}

// --- Tests for checkJWTSecret (coverage target: 0% → meaningful) ---

func TestCheckJWTSecretViaVaultConfig(t *testing.T) {
	cfg := config.Defaults()
	cfg.JWT.Secret = "vault://symaira/memory/jwt"
	SetConfig(cfg)

	result := checkJWTSecret()
	if !result.passed {
		t.Errorf("expected pass for vault:// config, got: %s", result.detail)
	}
	if !strings.Contains(result.detail, "vault://") {
		t.Errorf("expected 'vault://' in detail, got %q", result.detail)
	}
}

func TestCheckJWTSecretViaEnvVar(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "test-secret-from-env")

	cfg := config.Defaults()
	cfg.JWT.Secret = ""
	SetConfig(cfg)

	result := checkJWTSecret()
	if !result.passed {
		t.Errorf("expected pass for env var, got: %s", result.detail)
	}
	if !strings.Contains(result.detail, "environment variable") {
		t.Errorf("expected 'environment variable' in detail, got %q", result.detail)
	}
}

func TestCheckJWTSecretViaFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "symmemory-jwt-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	secretPath := filepath.Join(tempDir, "jwt.secret")
	if err := os.WriteFile(secretPath, []byte("test-secret"), 0600); err != nil {
		t.Fatalf("failed to write secret file: %v", err)
	}

	cfg := config.Defaults()
	cfg.JWT.Secret = ""
	cfg.JWT.SecretPath = secretPath
	SetConfig(cfg)

	result := checkJWTSecret()
	if !result.passed {
		t.Errorf("expected pass for existing file, got: %s", result.detail)
	}
	if !strings.Contains(result.detail, "file exists") {
		t.Errorf("expected 'file exists' in detail, got %q", result.detail)
	}
}

func TestCheckJWTSecretAutoGenerate(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "symmemory-jwt-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	t.Setenv("JWT_SECRET_KEY", "")

	cfg := config.Defaults()
	cfg.JWT.Secret = ""
	cfg.JWT.SecretPath = filepath.Join(tempDir, "nonexistent", "jwt.secret")
	SetConfig(cfg)

	result := checkJWTSecret()
	if !result.passed {
		t.Errorf("expected pass for auto-generate path, got: %s", result.detail)
	}
	if !strings.Contains(result.detail, "auto-generated") {
		t.Errorf("expected 'auto-generated' in detail, got %q", result.detail)
	}
}

func TestCheckJWTSecretCustomPath(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "symmemory-jwt-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	secretPath := filepath.Join(tempDir, "custom-secret.key")
	if err := os.WriteFile(secretPath, []byte("my-secret"), 0600); err != nil {
		t.Fatalf("failed to write secret file: %v", err)
	}

	cfg := config.Defaults()
	cfg.JWT.Secret = ""
	cfg.JWT.SecretPath = secretPath
	SetConfig(cfg)

	result := checkJWTSecret()
	if !result.passed {
		t.Errorf("expected pass for custom path, got: %s", result.detail)
	}
	if !strings.Contains(result.detail, "file exists") {
		t.Errorf("expected 'file exists' in detail, got %q", result.detail)
	}
}

func TestCheckJWTSecretPriorityVaultOverEnv(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "env-secret")

	cfg := config.Defaults()
	cfg.JWT.Secret = "vault://symaira/memory/jwt"
	SetConfig(cfg)

	result := checkJWTSecret()
	if !result.passed {
		t.Errorf("expected pass, got: %s", result.detail)
	}
	if !strings.Contains(result.detail, "vault://") {
		t.Errorf("expected vault:// to take precedence, got %q", result.detail)
	}
}

// --- Tests for checkConfig (coverage target: 46.2% → ≥70%) ---

func TestCheckConfigValidFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "symmemory-config-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configDir := filepath.Join(tempDir, ".config", "symmemory")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	configPath := filepath.Join(configDir, "config.toml")
	validTOML := `
[database]
path = "/tmp/test.db"

[ollama]
url = "http://localhost:11434/api/embeddings"
model = "nomic-embed-text"
`
	if err := os.WriteFile(configPath, []byte(validTOML), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	SetConfig(config.Defaults())
	result := checkConfig()
	if !result.passed {
		t.Errorf("expected pass for valid config, got: %s", result.detail)
	}
	if !strings.Contains(result.detail, "valid") {
		t.Errorf("expected 'valid' in detail, got %q", result.detail)
	}
}

func TestCheckConfigInvalidFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "symmemory-config-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configDir := filepath.Join(tempDir, ".config", "symmemory")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	configPath := filepath.Join(configDir, "config.toml")
	invalidTOML := `
[database
path = "/tmp/test.db"
`
	if err := os.WriteFile(configPath, []byte(invalidTOML), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	SetConfig(config.Defaults())
	result := checkConfig()
	if !result.passed {
		t.Errorf("config loader handled invalid TOML gracefully: %s", result.detail)
	}
}

func TestCheckConfigNoConfigDir(t *testing.T) {
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
		t.Errorf("expected pass for missing config, got: %s", result.detail)
	}
	if !strings.Contains(result.detail, "defaults") {
		t.Errorf("expected 'defaults' in detail, got %q", result.detail)
	}
}

func TestCheckConfigEmptyFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "symmemory-config-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configDir := filepath.Join(tempDir, ".config", "symmemory")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	configPath := filepath.Join(configDir, "config.toml")
	if err := os.WriteFile(configPath, []byte(""), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	SetConfig(config.Defaults())
	result := checkConfig()
	if !result.passed {
		t.Errorf("expected pass for empty config (valid TOML), got: %s", result.detail)
	}
}

// --- Tests for checkFilePermissions (coverage target: 41.9% → ≥70%) ---

func TestCheckFilePermissionsWrongDirPerms(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "symmemory-perm-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbDir := filepath.Join(tempDir, ".local", "share", "symmemory")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		t.Fatalf("failed to create db dir: %v", err)
	}

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	result := checkFilePermissions()
	if result.passed {
		t.Error("expected failure for wrong directory permissions")
	}
	if !strings.Contains(result.detail, "expected 0700") {
		t.Errorf("expected 'expected 0700' in detail, got %q", result.detail)
	}
}

func TestCheckFilePermissionsWrongDBPerms(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "symmemory-perm-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbDir := filepath.Join(tempDir, ".local", "share", "symmemory")
	if err := os.MkdirAll(dbDir, 0700); err != nil {
		t.Fatalf("failed to create db dir: %v", err)
	}
	dbPath := filepath.Join(dbDir, "default.db")
	if err := os.WriteFile(dbPath, []byte("fake-db"), 0644); err != nil {
		t.Fatalf("failed to write db file: %v", err)
	}

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	result := checkFilePermissions()
	if result.passed {
		t.Error("expected failure for wrong database permissions")
	}
	if !strings.Contains(result.detail, "database is") {
		t.Errorf("expected 'database is' in detail, got %q", result.detail)
	}
}

func TestCheckFilePermissionsWrongSecretPerms(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "symmemory-perm-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbDir := filepath.Join(tempDir, ".local", "share", "symmemory")
	if err := os.MkdirAll(dbDir, 0700); err != nil {
		t.Fatalf("failed to create db dir: %v", err)
	}
	dbPath := filepath.Join(dbDir, "default.db")
	if err := os.WriteFile(dbPath, []byte("fake-db"), 0600); err != nil {
		t.Fatalf("failed to write db file: %v", err)
	}

	secretDir := filepath.Join(tempDir, ".config", "symmemory")
	if err := os.MkdirAll(secretDir, 0700); err != nil {
		t.Fatalf("failed to create secret dir: %v", err)
	}
	secretPath := filepath.Join(secretDir, "jwt.secret")
	if err := os.WriteFile(secretPath, []byte("fake-secret"), 0644); err != nil {
		t.Fatalf("failed to write secret file: %v", err)
	}

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	result := checkFilePermissions()
	if result.passed {
		t.Error("expected failure for wrong secret permissions")
	}
	if !strings.Contains(result.detail, "secret file is") {
		t.Errorf("expected 'secret file is' in detail, got %q", result.detail)
	}
}

func TestCheckFilePermissionsDBNotExist(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "symmemory-perm-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbDir := filepath.Join(tempDir, ".local", "share", "symmemory")
	if err := os.MkdirAll(dbDir, 0700); err != nil {
		t.Fatalf("failed to create db dir: %v", err)
	}

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	result := checkFilePermissions()
	if !result.passed {
		t.Errorf("expected pass when db not created, got: %s", result.detail)
	}
	if !strings.Contains(result.detail, "database not yet created") {
		t.Errorf("expected 'database not yet created' in detail, got %q", result.detail)
	}
}

func TestCheckFilePermissionsSecretNotExist(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "symmemory-perm-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbDir := filepath.Join(tempDir, ".local", "share", "symmemory")
	if err := os.MkdirAll(dbDir, 0700); err != nil {
		t.Fatalf("failed to create db dir: %v", err)
	}
	dbPath := filepath.Join(dbDir, "default.db")
	if err := os.WriteFile(dbPath, []byte("fake-db"), 0600); err != nil {
		t.Fatalf("failed to write db file: %v", err)
	}

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	result := checkFilePermissions()
	if !result.passed {
		t.Errorf("expected pass when secret not created, got: %s", result.detail)
	}
	if !strings.Contains(result.detail, "all checked paths OK") {
		t.Errorf("expected 'all checked paths OK' in detail, got %q", result.detail)
	}
}

func TestCheckFilePermissionsAllCorrect(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "symmemory-perm-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbDir := filepath.Join(tempDir, ".local", "share", "symmemory")
	if err := os.MkdirAll(dbDir, 0700); err != nil {
		t.Fatalf("failed to create db dir: %v", err)
	}
	dbPath := filepath.Join(dbDir, "default.db")
	if err := os.WriteFile(dbPath, []byte("fake-db"), 0600); err != nil {
		t.Fatalf("failed to write db file: %v", err)
	}

	secretDir := filepath.Join(tempDir, ".config", "symmemory")
	if err := os.MkdirAll(secretDir, 0700); err != nil {
		t.Fatalf("failed to create secret dir: %v", err)
	}
	secretPath := filepath.Join(secretDir, "jwt.secret")
	if err := os.WriteFile(secretPath, []byte("fake-secret"), 0600); err != nil {
		t.Fatalf("failed to write secret file: %v", err)
	}

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	result := checkFilePermissions()
	if !result.passed {
		t.Errorf("expected pass for all correct perms, got: %s", result.detail)
	}
	if !strings.Contains(result.detail, "all checked paths OK") {
		t.Errorf("expected 'all checked paths OK' in detail, got %q", result.detail)
	}
}

func TestCheckFilePermissionsDirNotExist(t *testing.T) {
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
		t.Errorf("expected pass when directory not created, got: %s", result.detail)
	}
	if !strings.Contains(result.detail, "directory not yet created") {
		t.Errorf("expected 'directory not yet created' in detail, got %q", result.detail)
	}
}
