package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	machineKeyTag  = "alvx_sk_"
	machineKeySize = 32
)

// GenerateMachineAPIKey creates a high-entropy credential, its safe display
// prefix, and the one-way digest that should be persisted.
func GenerateMachineAPIKey() (rawKey, prefix, hash string, err error) {
	secret := make([]byte, machineKeySize)
	if _, err = rand.Read(secret); err != nil {
		return "", "", "", fmt.Errorf("generate API key: %w", err)
	}
	rawKey = machineKeyTag + base64.RawURLEncoding.EncodeToString(secret)
	prefixLength := len(machineKeyTag) + 8
	prefix = rawKey[:prefixLength]
	return rawKey, prefix, HashAPIKey(rawKey), nil
}

// HashAPIKey returns the deterministic one-way digest used for credential
// lookup. The raw credential must never be logged or persisted.
func HashAPIKey(rawKey string) string {
	digest := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(digest[:])
}

// GenerateWebhookURL constructs a WhatsApp callback using the explicit public
// API URL. Runtime configuration, rather than process-global environment reads,
// remains the single source of deployment truth.
func GenerateWebhookURL(publicAPIURL, clientID string) string {
	return fmt.Sprintf("%s/webhook/wa/v2/%s", strings.TrimRight(publicAPIURL, "/"), clientID)
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
