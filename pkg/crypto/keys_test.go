package crypto

import (
	"strings"
	"testing"
)

func TestGenerateMachineAPIKeyReturnsHashOnlyPersistenceMaterial(t *testing.T) {
	rawOne, prefixOne, hashOne, err := GenerateMachineAPIKey()
	if err != nil {
		t.Fatalf("generate first key: %v", err)
	}
	rawTwo, _, hashTwo, err := GenerateMachineAPIKey()
	if err != nil {
		t.Fatalf("generate second key: %v", err)
	}
	if !strings.HasPrefix(rawOne, "alvx_sk_") || !strings.HasPrefix(rawOne, prefixOne) {
		t.Fatalf("unexpected API key format: %q / %q", rawOne, prefixOne)
	}
	if len(hashOne) != 64 || hashOne != HashAPIKey(rawOne) {
		t.Fatalf("unexpected API key digest: %q", hashOne)
	}
	if rawOne == rawTwo || hashOne == hashTwo {
		t.Fatal("independently generated API keys must be unique")
	}
}
