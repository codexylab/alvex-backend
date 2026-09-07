package config

import (
	"strings"
	"testing"
)

func TestValidateRuntimeAcceptsSafeProductionConfig(t *testing.T) {
	cfg := productionConfig()

	if err := cfg.ValidateRuntime(); err != nil {
		t.Fatalf("expected valid configuration, got %v", err)
	}
}

func TestValidateRuntimeAcceptsProductionWithoutOptionalStripe(t *testing.T) {
	cfg := productionConfig()
	cfg.StripeSecretKey = ""
	cfg.StripeWebhookSecret = ""
	cfg.StripePriceBasic = ""
	cfg.StripePricePro = ""
	cfg.StripePriceEnterprise = ""

	if err := cfg.ValidateRuntime(); err != nil {
		t.Fatalf("expected Stripe-free web-chat configuration to be valid, got %v", err)
	}
}

func TestValidateRuntimeRejectsPartialStripeConfiguration(t *testing.T) {
	cfg := productionConfig()
	cfg.StripeWebhookSecret = ""

	err := cfg.ValidateRuntime()
	if err == nil || !strings.Contains(err.Error(), "Stripe configuration must include") {
		t.Fatalf("expected partial Stripe configuration error, got %v", err)
	}
}

func TestValidateRuntimeRejectsIncompleteProductionSecrets(t *testing.T) {
	cfg := productionConfig()
	cfg.EncryptionKey = "short"
	cfg.SupabaseAnonKey = ""
	cfg.AllowedOrigins = []string{"*"}

	err := cfg.ValidateRuntime()
	if err == nil {
		t.Fatal("expected invalid configuration")
	}
	for _, message := range []string{
		"ENCRYPTION_KEY must be exactly 32 bytes",
		"SUPABASE_URL and SUPABASE_ANON_KEY must be configured together",
		"ALLOWED_ORIGINS cannot contain *",
	} {
		if !strings.Contains(err.Error(), message) {
			t.Errorf("expected error to contain %q, got %q", message, err)
		}
	}
}

func TestValidateRuntimeRequiresPairedWhatsAppSecrets(t *testing.T) {
	cfg := productionConfig()
	cfg.WhatsAppVerifyToken = "verify-token"

	err := cfg.ValidateRuntime()
	if err == nil || !strings.Contains(err.Error(), "WHATSAPP_VERIFY_TOKEN and WHATSAPP_APP_SECRET") {
		t.Fatalf("expected WhatsApp pair validation error, got %v", err)
	}
}

func TestValidateRuntimeRequiresWhatsAppOutboundConfiguration(t *testing.T) {
	cfg := productionConfig()
	cfg.WhatsAppVerifyToken = "verify-token"
	cfg.WhatsAppAppSecret = "app-secret"

	err := cfg.ValidateRuntime()
	if err == nil || !strings.Contains(err.Error(), "WHATSAPP_ACCESS_TOKEN and WHATSAPP_GRAPH_API_BASE_URL") {
		t.Fatalf("expected WhatsApp outbound configuration error, got %v", err)
	}
}

func TestValidateRuntimeValidatesPreviousEncryptionKeys(t *testing.T) {
	cfg := productionConfig()
	cfg.PreviousEncryptionKeys = []string{"too-short", cfg.EncryptionKey}

	err := cfg.ValidateRuntime()
	if err == nil {
		t.Fatal("expected invalid key rotation configuration")
	}
	for _, message := range []string{
		"every ENCRYPTION_PREVIOUS_KEYS entry must be exactly 32 bytes",
		"ENCRYPTION_PREVIOUS_KEYS must not include the current ENCRYPTION_KEY",
	} {
		if !strings.Contains(err.Error(), message) {
			t.Errorf("expected error to contain %q, got %q", message, err)
		}
	}
}

func productionConfig() *Config {
	return &Config{
		Env:                   "production",
		EncryptionKey:         "0123456789abcdef0123456789abcdef",
		SupabaseURL:           "https://project.supabase.co",
		SupabaseAnonKey:       "public-anon-key",
		PublicAPIURL:          "https://api.alvex.example",
		AllowedOrigins:        []string{"https://app.alvex.example"},
		StripeSecretKey:       "sk_test_example",
		StripeWebhookSecret:   "whsec_example",
		StripePriceBasic:      "price_basic",
		StripePricePro:        "price_pro",
		StripePriceEnterprise: "price_enterprise",
	}
}
