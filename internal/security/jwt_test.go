package security

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/danieljustus/symaira-memory/internal/config"
)

// testConfigWithSecret builds a Config whose JWT secret_path points at a
// temp file containing the given secret. The previous NewJWTProvider(secret
// string) signature was replaced by config injection in #65; tests that need
// a stable secret now route the value through the file system to mirror the
// production path.
func testConfigWithSecret(t *testing.T, secret string) *config.Config {
	t.Helper()
	dir := t.TempDir()
	secretPath := filepath.Join(dir, "jwt.test.secret")
	if err := os.WriteFile(secretPath, []byte(secret), 0600); err != nil {
		t.Fatalf("failed to write test secret: %v", err)
	}
	return &config.Config{
		JWT: config.JWTConfig{SecretPath: secretPath},
	}
}

func jwtProviderWithSecret(t *testing.T, secret string) *JWTProvider {
	t.Helper()
	provider, err := NewJWTProvider(testConfigWithSecret(t, secret), nil)
	if err != nil {
		t.Fatalf("failed to create jwt provider: %v", err)
	}
	return provider
}

func TestJWTGenerationAndVerification(t *testing.T) {
	secret := "my_custom_secure_test_signing_key_2026"
	provider := jwtProviderWithSecret(t, secret)

	subject := "test-agent"
	duration := 10 * time.Minute

	// Test Token Generation
	token, err := provider.GenerateToken(subject, duration)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	if token == "" {
		t.Fatalf("generated token is empty")
	}

	// Test Token Verification
	payload, err := provider.VerifyToken(token)
	if err != nil {
		t.Fatalf("failed to verify valid token: %v", err)
	}

	if payload.Subject != subject {
		t.Errorf("expected subject '%s', got '%s'", subject, payload.Subject)
	}
	if payload.Issuer != "symaira-memory" {
		t.Errorf("expected issuer 'symaira-memory', got '%s'", payload.Issuer)
	}
	if payload.JWTID == "" {
		t.Errorf("expected non-empty jti in payload")
	}

	// Test Tampering Rejection (modify one character in signature)
	originalChar := token[len(token)-5]
	tamperedChar := "A"
	if originalChar == 'A' {
		tamperedChar = "B"
	}
	tamperedToken := token[:len(token)-5] + tamperedChar + token[len(token)-4:]
	_, err = provider.VerifyToken(tamperedToken)
	if err == nil {
		t.Errorf("verification should fail on tampered token")
	}

	// Test Expiration Verification
	expiredProvider := jwtProviderWithSecret(t, secret)
	expiredToken, err := expiredProvider.GenerateToken(subject, -5*time.Second) // expired 5s ago
	if err != nil {
		t.Fatalf("failed to generate expired token: %v", err)
	}

	_, err = expiredProvider.VerifyToken(expiredToken)
	if err == nil {
		t.Errorf("verification should fail on expired token")
	} else if !strings.Contains(err.Error(), "expired") {
		t.Errorf("expected expiration error message, got: %v", err)
	}
}

func TestJWTRevocation(t *testing.T) {
	provider := jwtProviderWithSecret(t, "revocation-test-secret")

	token, err := provider.GenerateToken("agent", 10*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	payload, err := provider.VerifyToken(token)
	if err != nil {
		t.Fatalf("token should be valid before revocation: %v", err)
	}

	provider.RevokeToken(payload.JWTID)

	_, err = provider.VerifyToken(token)
	if err == nil {
		t.Errorf("verification should fail after revocation")
	} else if !strings.Contains(err.Error(), "revoked") {
		t.Errorf("expected revocation error, got: %v", err)
	}
}

func TestJWTKeyRotation(t *testing.T) {
	provider := jwtProviderWithSecret(t, "old-secret-v1")

	oldToken, err := provider.GenerateToken("agent", 10*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate token with old key: %v", err)
	}

	provider.RotateSecret("new-secret-v2")

	// Old token should still verify (grace period)
	_, err = provider.VerifyToken(oldToken)
	if err != nil {
		t.Errorf("old token should still be valid after rotation: %v", err)
	}

	// New token should verify
	newToken, err := provider.GenerateToken("agent", 10*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate token with new key: %v", err)
	}
	_, err = provider.VerifyToken(newToken)
	if err != nil {
		t.Errorf("new token should verify with rotated key: %v", err)
	}

	// Token signed with unrelated key should fail
	forgedProvider := jwtProviderWithSecret(t, "unrelated-key")
	forgedToken, _ := forgedProvider.GenerateToken("agent", 10*time.Minute)
	_, err = provider.VerifyToken(forgedToken)
	if err == nil {
		t.Errorf("token from unrelated key should fail verification")
	}
}

func TestRotateSecretPersistsFallbackToDisk(t *testing.T) {
	dir := t.TempDir()
	secretPath := filepath.Join(dir, "jwt.test.secret")
	cfg := &config.Config{
		JWT: config.JWTConfig{SecretPath: secretPath},
	}

	provider, err := NewJWTProvider(cfg, nil)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	oldSecret := string(provider.secret)
	oldToken, err := provider.GenerateToken("agent", 10*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	provider.RotateSecret("new-secret-v2")

	fallbackPath := strings.TrimSuffix(secretPath, filepath.Ext(secretPath)) + ".secrets"
	data, err := os.ReadFile(fallbackPath)
	if err != nil {
		t.Fatalf("fallback secrets file not created: %v", err)
	}

	if !strings.Contains(string(data), oldSecret) {
		t.Errorf("fallback file should contain old secret %q, got: %s", oldSecret, string(data))
	}

	_, err = provider.VerifyToken(oldToken)
	if err != nil {
		t.Errorf("old token should still verify after rotation: %v", err)
	}
}

func TestLoadFallbackSecretsOnStartup(t *testing.T) {
	dir := t.TempDir()
	secretPath := filepath.Join(dir, "jwt.test.secret")
	if err := os.WriteFile(secretPath, []byte("primary-secret"), 0600); err != nil {
		t.Fatalf("failed to write secret: %v", err)
	}

	fallbackPath := strings.TrimSuffix(secretPath, filepath.Ext(secretPath)) + ".secrets"
	fallbackJSON := `[{"secret":"old-fallback-secret","expires_at":"2099-12-31T23:59:59Z"}]`
	if err := os.WriteFile(fallbackPath, []byte(fallbackJSON), 0600); err != nil {
		t.Fatalf("failed to write fallback: %v", err)
	}

	cfg := &config.Config{
		JWT: config.JWTConfig{SecretPath: secretPath},
	}

	provider, err := NewJWTProvider(cfg, nil)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	if len(provider.secrets) != 1 {
		t.Fatalf("expected 1 fallback secret, got %d", len(provider.secrets))
	}
	if string(provider.secrets[0]) != "old-fallback-secret" {
		t.Errorf("expected fallback secret 'old-fallback-secret', got %q", string(provider.secrets[0]))
	}

	oldProvider := &JWTProvider{secret: []byte("old-fallback-secret")}
	oldToken, _ := oldProvider.GenerateToken("agent", 10*time.Minute)

	_, err = provider.VerifyToken(oldToken)
	if err != nil {
		t.Errorf("token signed with loaded fallback should verify: %v", err)
	}
}

func TestGracePeriodExpirationPurgesOldSecrets(t *testing.T) {
	dir := t.TempDir()
	secretPath := filepath.Join(dir, "jwt.test.secret")
	if err := os.WriteFile(secretPath, []byte("primary-secret"), 0600); err != nil {
		t.Fatalf("failed to write secret: %v", err)
	}

	fallbackPath := strings.TrimSuffix(secretPath, filepath.Ext(secretPath)) + ".secrets"
	expiredJSON := `[{"secret":"expired-secret","expires_at":"2020-01-01T00:00:00Z"}]`
	if err := os.WriteFile(fallbackPath, []byte(expiredJSON), 0600); err != nil {
		t.Fatalf("failed to write fallback: %v", err)
	}

	cfg := &config.Config{
		JWT: config.JWTConfig{SecretPath: secretPath},
	}

	provider, err := NewJWTProvider(cfg, nil)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	if len(provider.secrets) != 0 {
		t.Errorf("expired fallback should be purged, got %d secrets", len(provider.secrets))
	}

	data, err := os.ReadFile(fallbackPath)
	if err != nil {
		t.Fatalf("failed to read fallback file: %v", err)
	}
	if strings.Contains(string(data), "expired-secret") {
		t.Errorf("expired secret should be purged from disk, got: %s", string(data))
	}
}

func TestRotationPurgesExpiredFallbacks(t *testing.T) {
	dir := t.TempDir()
	secretPath := filepath.Join(dir, "jwt.test.secret")
	cfg := &config.Config{
		JWT: config.JWTConfig{SecretPath: secretPath},
	}

	provider, err := NewJWTProvider(cfg, nil)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	provider.gracePeriod = 1 * time.Millisecond

	provider.RotateSecret("second-secret")

	time.Sleep(5 * time.Millisecond)

	provider.RotateSecret("third-secret")

	fallbackPath := strings.TrimSuffix(secretPath, filepath.Ext(secretPath)) + ".secrets"
	data, err := os.ReadFile(fallbackPath)
	if err != nil {
		t.Fatalf("failed to read fallback file: %v", err)
	}

	if strings.Contains(string(data), string([]byte("first"))) {
		t.Logf("fallback file contents: %s", string(data))
	}

	if len(provider.secrets) > 1 {
		t.Errorf("expected at most 1 fallback after purge, got %d", len(provider.secrets))
	}
}
