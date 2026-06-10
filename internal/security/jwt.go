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

	"github.com/danieljustus/symaira-memory/internal/config"
)

// DB interface for revocation persistence, avoiding circular imports.
type RevocationStore interface {
	RevokeToken(jti string) error
	IsTokenRevoked(jti string) (bool, error)
}

// JWTProvider manages API token issuance, validation, revocation, and key rotation.
type JWTProvider struct {
	secret   []byte
	secrets  [][]byte // fallback keys for rotation grace period
	revoked  map[string]time.Time
	revStore RevocationStore
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

type JWTPayload struct {
	JWTID     string `json:"jti"`
	Issuer    string `json:"iss"`
	Subject   string `json:"sub"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

// NewJWTProvider configures a JWT provider with a secret and an optional
// persistent revocation store. When store is nil, only in-memory revocation
// is used. The cfg argument supplies the configured secret path; pass nil
// to fall back to the default ~/.config/symmemory/jwt.secret location.
func NewJWTProvider(cfg *config.Config, store RevocationStore) (*JWTProvider, error) {
	if cfg == nil {
		cfg = config.Defaults()
	}
	secret := ""
	if secret == "" {
		secret = os.Getenv("JWT_SECRET_KEY")
	}
	if secret == "" {
		loaded, err := loadPersistedSecret(cfg)
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
	return &JWTProvider{
		secret:   []byte(secret),
		revoked:  make(map[string]time.Time),
		revStore: store,
	}, nil
}

// loadPersistedSecret reads the signing key from the configured path
// or the default ~/.config/symmemory/jwt.secret.
func loadPersistedSecret(cfg *config.Config) (string, error) {
	if cfg == nil {
		cfg = config.Defaults()
	}
	secretPath := cfg.JWT.SecretPath
	if secretPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		secretPath = filepath.Join(home, ".config", "symmemory", "jwt.secret")
	}

	data, err := os.ReadFile(secretPath)
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
	secretPath := filepath.Join(dir, "jwt.secret")
	if err := os.WriteFile(secretPath, []byte(secret+"\n"), 0600); err != nil {
		return "", err
	}
	return secret, nil
}

// GenerateToken issues a valid signed JWT token for the specified subject (e.g. "extension" or "gpt").
func (jp *JWTProvider) GenerateToken(subject string, duration time.Duration) (string, error) {
	header := jwtHeader{Alg: "HS256", Typ: "JWT"}
	headerBytes, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("marshal header: %w", err)
	}
	headerEncoded := base64URL(headerBytes)

	now := time.Now()
	jtiBytes := make([]byte, 16)
	if _, err := rand.Read(jtiBytes); err != nil {
		return "", fmt.Errorf("failed to generate JTI: %w", err)
	}
	jti := hex.EncodeToString(jtiBytes)

	payload := JWTPayload{
		JWTID:     jti,
		Issuer:    "symaira-memory",
		Subject:   subject,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(duration).Unix(),
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}
	payloadEncoded := base64URL(payloadBytes)

	// Sign the header and payload
	unsignedToken := headerEncoded + "." + payloadEncoded
	signature := jp.sign(unsignedToken)

	return unsignedToken + "." + signature, nil
}

// VerifyToken validates the signature and expiration of an incoming JWT.
// During key rotation, tokens signed with any known secret are accepted.
func (jp *JWTProvider) VerifyToken(token string) (*JWTPayload, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid token structure")
	}

	unsignedToken := parts[0] + "." + parts[1]

	// Try the primary secret first, then fallback secrets for rotation grace period.
	validSig := false
	secrets := append([][]byte{jp.secret}, jp.secrets...)
	for _, s := range secrets {
		if hmac.Equal([]byte(parts[2]), []byte(jp.signWith(unsignedToken, s))) {
			validSig = true
			break
		}
	}
	if !validSig {
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

	// Check in-memory revocation list
	if revokedAt, ok := jp.revoked[payload.JWTID]; ok && time.Now().After(revokedAt) {
		return nil, errors.New("token has been revoked")
	}

	// Check persistent revocation store
	if jp.revStore != nil {
		if isRevoked, err := jp.revStore.IsTokenRevoked(payload.JWTID); err == nil && isRevoked {
			return nil, errors.New("token has been revoked")
		}
	}

	// Check expiration
	if time.Now().Unix() > payload.ExpiresAt {
		return nil, errors.New("token expired")
	}

	return &payload, nil
}

// RevokeToken invalidates a token by its JWT ID so it can no longer be used.
// When a persistent store is available, the revocation is also persisted for
// cross-process consistency.
func (jp *JWTProvider) RevokeToken(jti string) {
	jp.revoked[jti] = time.Now()
	if jp.revStore != nil {
		_ = jp.revStore.RevokeToken(jti)
	}
}

// AddFallbackSecret registers an additional signing key for rotation.
// Tokens signed with the fallback key are accepted during the grace period.
func (jp *JWTProvider) AddFallbackSecret(secret string) {
	jp.secrets = append(jp.secrets, []byte(secret))
}

// RotateSecret replaces the primary signing key and keeps the current key
// as a fallback so existing tokens remain valid during the transition.
func (jp *JWTProvider) RotateSecret(newSecret string) {
	jp.AddFallbackSecret(string(jp.secret))
	jp.secret = []byte(newSecret)
}

func (jp *JWTProvider) sign(text string) string {
	return jp.signWith(text, jp.secret)
}

func (jp *JWTProvider) signWith(text string, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
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
