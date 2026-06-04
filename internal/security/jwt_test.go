package security

import (
	"strings"
	"testing"
	"time"
)

func TestJWTGenerationAndVerification(t *testing.T) {
	secret := "my_custom_secure_test_signing_key_2026"
	provider, err := NewJWTProvider(secret, nil)
	if err != nil {
		t.Fatalf("failed to create jwt provider: %v", err)
	}

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
	expiredProvider, err := NewJWTProvider(secret, nil)
	if err != nil {
		t.Fatalf("failed to create expired jwt provider: %v", err)
	}
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
	provider, err := NewJWTProvider("revocation-test-secret", nil)
	if err != nil {
		t.Fatalf("failed to create jwt provider: %v", err)
	}

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
	provider, err := NewJWTProvider("old-secret-v1", nil)
	if err != nil {
		t.Fatalf("failed to create jwt provider: %v", err)
	}

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
	forgedProvider, _ := NewJWTProvider("unrelated-key", nil)
	forgedToken, _ := forgedProvider.GenerateToken("agent", 10*time.Minute)
	_, err = provider.VerifyToken(forgedToken)
	if err == nil {
		t.Errorf("token from unrelated key should fail verification")
	}
}
