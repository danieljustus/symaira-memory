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
	"github.com/danieljustus/symaira-memory/internal/secrets"
)

// DefaultRotationGracePeriod is the default duration that a rotated secret
// remains valid as a fallback after key rotation.
const DefaultRotationGracePeriod = 24 * time.Hour

// DB interface for revocation persistence, avoiding circular imports.
type RevocationStore interface {
	RevokeToken(jti string) error
	IsTokenRevoked(jti string) (bool, error)
}

// JWTProvider manages API token issuance, validation, revocation, and key rotation.
type JWTProvider struct {
	secret         []byte
	secrets        [][]byte // fallback keys for rotation grace period
	secretExpiries []time.Time
	revoked        map[string]time.Time
	revStore       RevocationStore
	cfg            *config.Config
	gracePeriod    time.Duration
}

type fallbackEntry struct {
	Secret    string    `json:"secret"`
	ExpiresAt time.Time `json:"expires_at"`
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
//
// Secret resolution order:
//  1. cfg.JWT.Secret — vault:// URI resolved via symvault subprocess (5s timeout)
//  2. JWT_SECRET_KEY environment variable
//  3. File at cfg.JWT.SecretPath (or default ~/.config/symmemory/jwt.secret)
//  4. Auto-generate and persist a random 32-byte hex secret
func NewJWTProvider(cfg *config.Config, store RevocationStore) (*JWTProvider, error) {
	if cfg == nil {
		cfg = config.Defaults()
	}

	// 1. Try vault:// resolution from config
	secret, err := secrets.Resolve(cfg.JWT.Secret, "JWT_SECRET_KEY")
	if err != nil {
		// vault:// resolution failed — propagate error with context
		return nil, fmt.Errorf("JWT secret vault:// resolution failed: %w", err)
	}

	// 2. Try env var if vault:// didn't produce a value
	if secret == "" {
		secret = os.Getenv("JWT_SECRET_KEY")
	}

	// 3. Try loading from file
	if secret == "" {
		loaded, loadErr := loadPersistedSecret(cfg)
		if loadErr == nil && loaded != "" {
			secret = loaded
		}
	}

	// 4. Auto-generate as last resort
	if secret == "" {
		generated, genErr := generateAndPersistSecret(cfg)
		if genErr != nil {
			return nil, fmt.Errorf("failed to generate JWT secret: %w", genErr)
		}
		secret = generated
	}

	provider := &JWTProvider{
		secret:      []byte(secret),
		revoked:     make(map[string]time.Time),
		revStore:    store,
		cfg:         cfg,
		gracePeriod: DefaultRotationGracePeriod,
	}

	entries, err := loadFallbackSecrets(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to load fallback JWT secrets: %v\n", err)
	}
	now := time.Now()
	var validEntries []fallbackEntry
	for _, e := range entries {
		if e.ExpiresAt.After(now) {
			provider.secrets = append(provider.secrets, []byte(e.Secret))
			provider.secretExpiries = append(provider.secretExpiries, e.ExpiresAt)
			validEntries = append(validEntries, e)
		}
	}
	if len(validEntries) != len(entries) {
		if persistErr := persistFallbackSecrets(cfg, validEntries); persistErr != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to purge expired fallback secrets: %v\n", persistErr)
		}
	}

	return provider, nil
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
func generateAndPersistSecret(cfg *config.Config) (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	secret := hex.EncodeToString(bytes)

	secretPath := cfg.JWT.SecretPath
	if secretPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		secretPath = filepath.Join(home, ".config", "symmemory", "jwt.secret")
	}

	dir := filepath.Dir(secretPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	if err := os.WriteFile(secretPath, []byte(secret+"\n"), 0600); err != nil {
		return "", err
	}
	return secret, nil
}

func fallbackSecretsPath(cfg *config.Config) string {
	if cfg == nil {
		cfg = config.Defaults()
	}
	secretPath := cfg.JWT.SecretPath
	if secretPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		secretPath = filepath.Join(home, ".config", "symmemory", "jwt.secret")
	}
	return strings.TrimSuffix(secretPath, filepath.Ext(secretPath)) + ".secrets"
}

func loadFallbackSecrets(cfg *config.Config) ([]fallbackEntry, error) {
	path := fallbackSecretsPath(cfg)
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var entries []fallbackEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func persistFallbackSecrets(cfg *config.Config, entries []fallbackEntry) error {
	path := fallbackSecretsPath(cfg)
	if path == "" {
		return fmt.Errorf("cannot determine fallback secrets path")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
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
	now := time.Now()
	secrets := append([][]byte{jp.secret}, jp.secrets...)
	for i, s := range secrets {
		if i > 0 && i-1 < len(jp.secretExpiries) {
			if exp := jp.secretExpiries[i-1]; !exp.IsZero() && exp.Before(now) {
				continue
			}
		}
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
// The old secret is persisted to disk with an expiry based on the configured
// grace period, so tokens remain valid across process restarts.
func (jp *JWTProvider) RotateSecret(newSecret string) {
	now := time.Now()
	expiresAt := now.Add(jp.gracePeriod)

	jp.secrets = append(jp.secrets, []byte(jp.secret))
	jp.secretExpiries = append(jp.secretExpiries, expiresAt)
	jp.secret = []byte(newSecret)

	var keptSecrets [][]byte
	var keptExpiries []time.Time
	var keptEntries []fallbackEntry
	for i, s := range jp.secrets {
		exp := time.Time{}
		if i < len(jp.secretExpiries) {
			exp = jp.secretExpiries[i]
		}
		if !exp.IsZero() && exp.Before(now) {
			continue
		}
		keptSecrets = append(keptSecrets, s)
		keptExpiries = append(keptExpiries, exp)
		keptEntries = append(keptEntries, fallbackEntry{
			Secret:    string(s),
			ExpiresAt: exp,
		})
	}
	jp.secrets = keptSecrets
	jp.secretExpiries = keptExpiries

	if jp.cfg != nil {
		if err := persistFallbackSecrets(jp.cfg, keptEntries); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to persist fallback JWT secrets: %v\n", err)
		}
	}
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
