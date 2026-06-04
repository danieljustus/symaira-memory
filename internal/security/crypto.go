package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"

	"golang.org/x/crypto/pbkdf2"
)

const (
	// PBKDF2Iterations defines the iteration count for key derivation.
	// OWASP 2023 recommends 600,000 for SHA-256.
	PBKDF2Iterations = 600_000
)

// CryptoEngine handles Zero-Knowledge E2E encryption using AES-256-GCM.
type CryptoEngine struct{}

// NewCryptoEngine creates a new crypto instance.
func NewCryptoEngine() *CryptoEngine {
	return &CryptoEngine{}
}

func (ce *CryptoEngine) DeriveKey(passphrase string, salt []byte) []byte {
	return pbkdf2.Key([]byte(passphrase), salt, PBKDF2Iterations, 32, sha256.New)
}

// Encrypt encrypts a raw byte payload using AES-256-GCM with a passphrase.
// Output contains the salt (8 bytes) + nonce (12 bytes) + ciphertext.
func (ce *CryptoEngine) Encrypt(plaintext []byte, passphrase string) ([]byte, error) {
	// Generate a secure random salt (8 bytes)
	salt := make([]byte, 8)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}

	key := ce.DeriveKey(passphrase, salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// Generate a secure random nonce (12 bytes for GCM)
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	// Seal appends the ciphertext to the nonce, so the output starts with the nonce!
	// We prefix the whole payload with the salt so we can re-derive the key during decryption!
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	// Combine: salt (8) + nonce (12) + ciphertext
	payload := make([]byte, len(salt)+len(nonce)+len(ciphertext))
	copy(payload[0:8], salt)
	copy(payload[8:8+len(nonce)], nonce)
	copy(payload[8+len(nonce):], ciphertext)

	return payload, nil
}

// Decrypt decrypts an AES-256-GCM encrypted payload using the passphrase.
func (ce *CryptoEngine) Decrypt(payload []byte, passphrase string) ([]byte, error) {
	if len(payload) < 20 { // 8 (salt) + 12 (nonce) + minimum ciphertext
		return nil, errors.New("encrypted payload too short")
	}

	// Extract salt and nonce
	salt := payload[0:8]
	nonceSize := 12 // standard GCM nonce
	nonce := payload[8 : 8+nonceSize]
	ciphertext := payload[8+nonceSize:]

	key := ce.DeriveKey(passphrase, salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, errors.New("decryption failed: invalid passphrase or corrupted payload")
	}

	return plaintext, nil
}
