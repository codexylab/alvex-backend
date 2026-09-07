package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

const ciphertextVersion = "v1:"

func IsVersionedCiphertext(value string) bool {
	return strings.HasPrefix(value, ciphertextVersion)
}

// EncryptAPIKey encrypts a plaintext API key using AES-256-GCM.
// The encryptionKey must be exactly 32 bytes. New values include an algorithm
// version prefix so future rotations can distinguish storage formats.
func EncryptAPIKey(encryptionKey, plaintext string) (string, error) {
	key, err := validatedKey(encryptionKey)
	if err != nil {
		return "", err
	}

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
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), []byte(ciphertextVersion))
	return ciphertextVersion + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptAPIKey preserves the legacy string-only API. New code that handles
// secrets should use DecryptAPIKeyWithError and fail closed on versioned data.
func DecryptAPIKey(encryptionKey, ciphertext string) string {
	plaintext, err := DecryptAPIKeyWithError(encryptionKey, ciphertext)
	if err != nil {
		return ciphertext
	}
	return plaintext
}

// DecryptAPIKeyWithError decrypts versioned values strictly while retaining
// compatibility with legacy unversioned ciphertext and plaintext records.
func DecryptAPIKeyWithError(encryptionKey, ciphertext string) (string, error) {
	plaintext, _, err := DecryptAPIKeyWithKeyring(encryptionKey, nil, ciphertext)
	return plaintext, err
}

// DecryptAPIKeyWithKeyring attempts the current key first, followed by
// previous keys retained during a rotation window. The boolean reports when a
// previous key was used so callers can schedule re-encryption.
func DecryptAPIKeyWithKeyring(
	currentKey string,
	previousKeys []string,
	ciphertext string,
) (string, bool, error) {
	keys := append([]string{currentKey}, previousKeys...)
	for _, key := range keys {
		if _, err := validatedKey(key); err != nil {
			return "", false, err
		}
	}

	encoded := ciphertext
	additionalData := []byte(nil)
	versioned := strings.HasPrefix(ciphertext, ciphertextVersion)
	if versioned {
		encoded = strings.TrimPrefix(ciphertext, ciphertextVersion)
		additionalData = []byte(ciphertextVersion)
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		if !versioned {
			return ciphertext, false, nil
		}
		return "", false, fmt.Errorf("decode versioned ciphertext: %w", err)
	}

	var lastError error
	for index, candidateKey := range keys {
		plaintext, decryptErr := decryptCiphertext(candidateKey, data, additionalData)
		if decryptErr == nil {
			return plaintext, index > 0, nil
		}
		lastError = decryptErr
	}
	if !versioned {
		return ciphertext, false, nil
	}
	return "", false, fmt.Errorf("decrypt versioned ciphertext: %w", lastError)
}

func decryptCiphertext(key string, data, additionalData []byte) (string, error) {
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return "", fmt.Errorf("aes cipher init: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("gcm init: %w", err)
	}
	if len(data) < gcm.NonceSize() {
		return "", fmt.Errorf("ciphertext is too short")
	}
	plaintext, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], additionalData)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func validatedKey(key string) ([]byte, error) {
	if len([]byte(key)) != 32 {
		return nil, fmt.Errorf("encryption key must be exactly 32 bytes")
	}
	return []byte(key), nil
}
