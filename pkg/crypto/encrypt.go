package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
)

// EncryptAPIKey encrypts a plaintext API key using AES-256-GCM.
// The encryptionKey is padded/truncated to exactly 32 bytes.
// Returns a base64-encoded ciphertext string safe for DB storage.
func EncryptAPIKey(encryptionKey, plaintext string) (string, error) {
	key := padKey(encryptionKey)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("aes cipher init: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("gcm init: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("nonce generation: %w", err)
	}

	// Seal prepends the nonce so we can recover it during decryption.
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptAPIKey decrypts a base64-encoded AES-256-GCM ciphertext.
// If decryption fails for any reason (e.g. legacy plaintext key stored before
// encryption was enabled), the input is returned unchanged for backward compatibility.
func DecryptAPIKey(encryptionKey, ciphertext string) string {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		// Not valid base64 — treat as a legacy plaintext key.
		return ciphertext
	}

	key := padKey(encryptionKey)

	block, err := aes.NewCipher(key)
	if err != nil {
		return ciphertext
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return ciphertext
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return ciphertext
	}

	plaintext, err := gcm.Open(nil, data[:nonceSize], data[nonceSize:], nil)
	if err != nil {
		// Decryption failed — return raw value (legacy plaintext key).
		return ciphertext
	}

	return string(plaintext)
}

// padKey pads or truncates the key to exactly 32 bytes for AES-256.
func padKey(key string) []byte {
	b := make([]byte, 32)
	copy(b, []byte(key))
	return b
}
