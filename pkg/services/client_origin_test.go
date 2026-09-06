package services

import (
	"errors"
	"testing"

	"github.com/codexylab/alvex-backend/pkg/apierr"
)

func TestValidateAllowedOriginsNormalizesAndDeduplicates(t *testing.T) {
	origins, err := validateAllowedOrigins([]string{
		"HTTPS://Example.COM",
		"https://example.com",
		"http://localhost:3000",
	})
	if err != nil {
		t.Fatalf("validate origins: %v", err)
	}
	if len(origins) != 2 || origins[0] != "https://example.com" || origins[1] != "http://localhost:3000" {
		t.Fatalf("unexpected origins: %#v", origins)
	}
}

func TestValidateAllowedOriginsRejectsPathsAndUnsafeSchemes(t *testing.T) {
	for _, origin := range []string{"https://example.com/path", "javascript:alert(1)", "example.com"} {
		_, err := validateAllowedOrigins([]string{origin})
		if !errors.Is(err, apierr.ErrValidation) {
			t.Fatalf("expected validation error for %q, got %v", origin, err)
		}
	}
}
