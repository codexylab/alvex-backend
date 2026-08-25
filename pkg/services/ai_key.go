package services

import (
	"strings"

	"github.com/codexylab/alvex-backend/pkg/models"
	"github.com/codexylab/alvex-backend/pkg/crypto"
)

// platformKeys holds the platform-level fallback API keys for each provider.
type platformKeys struct {
	EncryptionKey  string
	Gemini         string
	OpenAI         string
	Groq           string
	FallbackGemini string
}

// resolveProviderAPIKey determines the correct API key to use for a client's AI provider.
// Priority: client-specific key â†’ platform key.
func resolveProviderAPIKey(c *models.Client, keys platformKeys) string {
	apiKey := c.APIKey

	switch c.Provider {
	case "Gemini":
		if c.GeminiAPIKey != "" {
			if keys.EncryptionKey != "" {
				return crypto.DecryptAPIKey(keys.EncryptionKey, c.GeminiAPIKey)
			}
			return c.GeminiAPIKey
		}
		decrypted := decryptIfNeeded(apiKey, keys.EncryptionKey)
		if isAlvexKey(decrypted) {
			return keys.Gemini
		}
		return decrypted

	case "Groq":
		if c.GroqAPIKey != "" {
			if keys.EncryptionKey != "" {
				return crypto.DecryptAPIKey(keys.EncryptionKey, c.GroqAPIKey)
			}
			return c.GroqAPIKey
		}
		decrypted := decryptIfNeeded(apiKey, keys.EncryptionKey)
		if isAlvexKey(decrypted) {
			return keys.Groq
		}
		return decrypted

	default: // OpenAI
		decrypted := decryptIfNeeded(apiKey, keys.EncryptionKey)
		if isAlvexKey(decrypted) {
			return keys.OpenAI
		}
		return decrypted
	}
}

// decryptIfNeeded decrypts the key if an encryption key is set and the value is non-empty.
func decryptIfNeeded(key, encKey string) string {
	if encKey != "" && key != "" {
		return crypto.DecryptAPIKey(encKey, key)
	}
	return key
}

// isAlvexKey returns true if the key is an internal Alvex platform key (not a real provider key).
func isAlvexKey(key string) bool {
	return strings.HasPrefix(key, "ALVX-")
}
