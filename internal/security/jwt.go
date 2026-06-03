package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// JWTProvider manages API token issuance and validation.
type JWTProvider struct {
	secret []byte
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

type JWTPayload struct {
	Issuer    string `json:"iss"`
	Subject   string `json:"sub"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

// NewJWTProvider configures a JWT provider with a secret.
// Sources the signing secret from (in order): explicit argument, JWT_SECRET_KEY env var,
// persisted file at ~/.config/symmemory/jwt.secret, or auto-generates and persists a new one.
func NewJWTProvider(secret string) (*JWTProvider, error) {
	if secret == "" {
		secret = os.Getenv("JWT_SECRET_KEY")
	}
	if secret == "" {
		loaded, err := loadPersistedSecret()
		if err == nil && loaded != "" {
			secret = loaded
		}
	}
	if secret == "" {
		generated, err := generateAndPersistSecret()
		if err != nil {
			return nil, fmt.Errorf("failed to generate JWT secret: %w", err)
		}
		secret = generated
	}
	return &JWTProvider{secret: []byte(secret)}, nil
}

// loadPersistedSecret reads the signing key from ~/.config/symmemory/jwt.secret.
func loadPersistedSecret() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(home, ".config", "symmemory", "jwt.secret")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// generateAndPersistSecret creates a random 32-byte hex secret and persists it.
func generateAndPersistSecret() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	secret := hex.EncodeToString(bytes)

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".config", "symmemory")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "jwt.secret")
	if err := os.WriteFile(path, []byte(secret+"\n"), 0600); err != nil {
		return "", err
	}
	return secret, nil
}

// GenerateToken issues a valid signed JWT token for the specified subject (e.g. "extension" or "gpt").
func (jp *JWTProvider) GenerateToken(subject string, duration time.Duration) (string, error) {
	header := jwtHeader{Alg: "HS256", Typ: "JWT"}
	headerBytes, _ := json.Marshal(header)
	headerEncoded := base64URL(headerBytes)

	now := time.Now()
	payload := JWTPayload{
		Issuer:    "symaira-memory",
		Subject:   subject,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(duration).Unix(),
	}
	payloadBytes, _ := json.Marshal(payload)
	payloadEncoded := base64URL(payloadBytes)

	// Sign the header and payload
	unsignedToken := headerEncoded + "." + payloadEncoded
	signature := jp.sign(unsignedToken)

	return unsignedToken + "." + signature, nil
}

// VerifyToken validates the signature and expiration of an incoming JWT.
func (jp *JWTProvider) VerifyToken(token string) (*JWTPayload, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid token structure")
	}

	unsignedToken := parts[0] + "." + parts[1]
	expectedSig := jp.sign(unsignedToken)

	// Constant-time signature comparison to prevent timing attacks
	if !hmac.Equal([]byte(parts[2]), []byte(expectedSig)) {
		return nil, errors.New("invalid signature")
	}

	// Decode payload
	payloadBytes, err := base64URLDecode(parts[1])
	if err != nil {
		return nil, err
	}

	var payload JWTPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, err
	}

	// Check expiration
	if time.Now().Unix() > payload.ExpiresAt {
		return nil, errors.New("token expired")
	}

	return &payload, nil
}

func (jp *JWTProvider) sign(text string) string {
	mac := hmac.New(sha256.New, jp.secret)
	mac.Write([]byte(text))
	return base64URL(mac.Sum(nil))
}

func base64URL(b []byte) string {
	encoded := base64.URLEncoding.EncodeToString(b)
	return strings.TrimRight(encoded, "=")
}

func base64URLDecode(s string) ([]byte, error) {
	// Pad if needed
	if idx := len(s) % 4; idx != 0 {
		s += strings.Repeat("=", 4-idx)
	}
	return base64.URLEncoding.DecodeString(s)
}
