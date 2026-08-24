package crypto

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"strings"
)

const (
	apiKeyAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	apiKeyLength   = 8
)

// GenerateAPIKey creates a new ALVEX-format API key.
// Format: ALVX-{4-char-prefix}-{4-digit-number}x{8-random-chars}
// Example: ALVX-NEXD-8921xab3c4f2d
func GenerateAPIKey(clientName string) string {
	prefix := strings.ToUpper(clientName)
	if len(prefix) > 4 {
		prefix = prefix[:4]
	} else {
		prefix = strings.ToUpper(fmt.Sprintf("%-4s", prefix))
	}
	prefix = strings.ReplaceAll(prefix, " ", "X")

	numPart := mustRandomInt(1000, 9999)
	randPart := mustRandomString(apiKeyAlphabet, apiKeyLength)

	return fmt.Sprintf("ALVX-%s-%dx%s", prefix, numPart, randPart)
}

// GenerateWebhookURL constructs the WhatsApp webhook endpoint URL for a client.
// The base URL is read from WEBHOOK_BASE_URL env var; falls back to the production default.
func GenerateWebhookURL(clientID string) string {
	baseURL := os.Getenv("WEBHOOK_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.alvex.ai"
	}
	return fmt.Sprintf("%s/webhook/wa/v2/%s", baseURL, clientID)
}

// GeneratePortalToken creates a cryptographically secure 64-character hex token
// for client portal authentication. This is separate from the API key used for webhooks.
func GeneratePortalToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	return fmt.Sprintf("%x", b)
}

// SlugifyClientName converts a client name into a URL-safe ID slug.
// Example: "Nexus Dynamics" → "nexus-dynamics"
func SlugifyClientName(name string) string {
	slug := strings.ToLower(strings.TrimSpace(name))
	var result strings.Builder
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			result.WriteRune(r)
		} else {
			result.WriteRune('-')
		}
	}
	// Collapse consecutive dashes and trim
	s := result.String()
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}

// --- Private helpers ---

func mustRandomInt(min, max int64) int64 {
	n, err := rand.Int(rand.Reader, big.NewInt(max-min+1))
	if err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	return n.Int64() + min
}

func mustRandomString(alphabet string, length int) string {
	result := make([]byte, length)
	for i := range result {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			panic(fmt.Sprintf("crypto/rand failed: %v", err))
		}
		result[i] = alphabet[idx.Int64()]
	}
	return string(result)
}
