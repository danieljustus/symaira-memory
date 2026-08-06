package cmd

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/security"
)

// tokenTestEnv wires a config with a stable JWT secret plus an open database
// (used as the persistent revocation store), mirroring how the serve command
// constructs its JWT provider.
func tokenTestEnv(t *testing.T) *config.Config {
	t.Helper()
	tempDir := t.TempDir()
	setTestHome(t, tempDir)

	cfg := &config.Config{JWT: config.JWTConfig{SecretPath: filepath.Join(tempDir, "jwt.test.secret")}}
	SetConfig(cfg)
	SetDB(helperTestDB(t))
	return cfg
}

func TestTokenRevokeCommandStructure(t *testing.T) {
	revokeCmd := findSubcommand(rootCmd, "token", "revoke [token-or-jti]")
	if revokeCmd == nil {
		t.Fatal("token revoke command not found")
	}

	if err := revokeCmd.Args(revokeCmd, []string{}); err == nil {
		t.Error("expected error for revoke with no args")
	}
	if err := revokeCmd.Args(revokeCmd, []string{"some-jti"}); err != nil {
		t.Errorf("expected no error for revoke with 1 arg, got: %v", err)
	}
}

func TestTokenRevokeCLIByTokenValue(t *testing.T) {
	cfg := tokenTestEnv(t)

	provider, err := security.NewJWTProvider(cfg, GetDB())
	if err != nil {
		t.Fatalf("failed to create JWT provider: %v", err)
	}
	token, err := provider.GenerateToken("cli-test", time.Hour)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}
	if _, err := provider.VerifyToken(token); err != nil {
		t.Fatalf("token should verify before revocation: %v", err)
	}

	if err := tokenRevokeCmd.RunE(tokenRevokeCmd, []string{token}); err != nil {
		t.Fatalf("token revoke by value failed: %v", err)
	}

	// A fresh provider backed by the same database must reject the token,
	// proving the CLI revocation reached the persistent store.
	fresh, err := security.NewJWTProvider(cfg, GetDB())
	if err != nil {
		t.Fatalf("failed to create second JWT provider: %v", err)
	}
	if _, err := fresh.VerifyToken(token); err == nil || !strings.Contains(err.Error(), "revoked") {
		t.Errorf("expected revoked error after CLI revoke by value, got: %v", err)
	}
}

func TestTokenRevokeCLIByJTI(t *testing.T) {
	cfg := tokenTestEnv(t)

	provider, err := security.NewJWTProvider(cfg, GetDB())
	if err != nil {
		t.Fatalf("failed to create JWT provider: %v", err)
	}
	token, err := provider.GenerateToken("cli-test", time.Hour)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}
	payload, err := provider.VerifyToken(token)
	if err != nil {
		t.Fatalf("failed to verify token: %v", err)
	}

	if err := tokenRevokeCmd.RunE(tokenRevokeCmd, []string{payload.JWTID}); err != nil {
		t.Fatalf("token revoke by jti failed: %v", err)
	}

	fresh, err := security.NewJWTProvider(cfg, GetDB())
	if err != nil {
		t.Fatalf("failed to create second JWT provider: %v", err)
	}
	if _, err := fresh.VerifyToken(token); err == nil || !strings.Contains(err.Error(), "revoked") {
		t.Errorf("expected revoked error after CLI revoke by jti, got: %v", err)
	}
}

func TestTokenRevokeCLIRejectsInvalidInput(t *testing.T) {
	tokenTestEnv(t)

	// Malformed token value (contains dots but is not a JWT).
	if err := tokenRevokeCmd.RunE(tokenRevokeCmd, []string{"abc.def.ghi"}); err == nil {
		t.Error("expected error for malformed token value")
	}
	// Empty argument.
	if err := tokenRevokeCmd.RunE(tokenRevokeCmd, []string{""}); err == nil {
		t.Error("expected error for empty argument")
	}
}
