package crypto

import (
	"strings"
	"testing"
)

func TestVersionedAPIKeyEncryptionRoundTrip(t *testing.T) {
	const key = "12345678901234567890123456789012"
	encrypted, err := EncryptAPIKey(key, "provider-secret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if !strings.HasPrefix(encrypted, ciphertextVersion) {
		t.Fatalf("ciphertext is not versioned: %q", encrypted)
	}
	plaintext, err := DecryptAPIKeyWithError(key, encrypted)
	if err != nil || plaintext != "provider-secret" {
		t.Fatalf("decrypt: plaintext=%q error=%v", plaintext, err)
	}
}

func TestVersionedAPIKeyEncryptionRejectsTamperingAndInvalidKeys(t *testing.T) {
	const key = "12345678901234567890123456789012"
	encrypted, err := EncryptAPIKey(key, "provider-secret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	tampered := encrypted[:len(encrypted)-1] + "A"
	if _, err := DecryptAPIKeyWithError(key, tampered); err == nil {
		t.Fatal("expected tampered ciphertext to fail closed")
	}
	if _, err := EncryptAPIKey("too-short", "secret"); err == nil {
		t.Fatal("expected invalid encryption key length to be rejected")
	}
}

func TestDecryptAPIKeyWithErrorAcceptsLegacyPlaintext(t *testing.T) {
	const key = "12345678901234567890123456789012"
	plaintext, err := DecryptAPIKeyWithError(key, "legacy-provider-key")
	if err != nil || plaintext != "legacy-provider-key" {
		t.Fatalf("legacy plaintext compatibility failed: plaintext=%q error=%v", plaintext, err)
	}
}

func TestDecryptAPIKeyWithKeyringSupportsSafeRotation(t *testing.T) {
	const previousKey = "12345678901234567890123456789012"
	const currentKey = "abcdefghijklmnopqrstuvwx12345678"
	encrypted, err := EncryptAPIKey(previousKey, "provider-secret")
	if err != nil {
		t.Fatalf("encrypt with previous key: %v", err)
	}

	plaintext, usedPrevious, err := DecryptAPIKeyWithKeyring(
		currentKey,
		[]string{previousKey},
		encrypted,
	)
	if err != nil || plaintext != "provider-secret" || !usedPrevious {
		t.Fatalf("rotation decrypt failed: plaintext=%q previous=%v error=%v", plaintext, usedPrevious, err)
	}
}
