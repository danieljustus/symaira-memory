package security

import (
	"bytes"
	"testing"
)

func TestE2EEncryptionLoop(t *testing.T) {
	crypto := NewCryptoEngine()
	passphrase := "my_ultra_secure_master_password_2026"
	plaintext := []byte("Strict secret: User preferences and API credentials")

	// Test Encryption
	ciphertext, err := crypto.Encrypt(plaintext, passphrase)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	if len(ciphertext) <= len(plaintext) {
		t.Errorf("ciphertext is too short, should be longer due to salt and nonce overhead")
	}

	// Verify ciphertext doesn't contain plaintext directly
	if bytes.Contains(ciphertext, plaintext) {
		t.Errorf("ciphertext leaked plaintext directly")
	}

	// Test Decryption (Success)
	decrypted, err := crypto.Decrypt(ciphertext, passphrase)
	if err != nil {
		t.Fatalf("decryption failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("decrypted payload mismatch:\n  expected: '%s'\n  got:      '%s'", string(plaintext), string(decrypted))
	}

	// Test Decryption with invalid passphrase
	_, err = crypto.Decrypt(ciphertext, "wrong_password_attempt")
	if err == nil {
		t.Errorf("decryption should fail with incorrect passphrase")
	}
}
