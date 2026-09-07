package config

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all environment-based configuration for ALVEX backend.
type Config struct {
	// Server
	Port string
	Env  string

	// Database
	DatabaseURL string

	// Encryption key for API key storage (AES-256, exactly 32 chars)
	EncryptionKey string
	// PreviousEncryptionKeys remain decrypt-only during a controlled key rotation.
	PreviousEncryptionKeys []string

	// Supabase Auth
	SupabaseURL       string
	SupabaseAnonKey   string
	SupabaseSecretKey string
	DevToken          string
	FrontendURL       string
	PublicAPIURL      string

	// AI Providers
	GeminiAPIKey string
	OpenAIAPIKey string
	GroqAPIKey   string
	// FallbackGeminiKey is the platform-level Gemini API key used to automatically
	// handle requests when the client's primary provider (Groq/OpenAI) fails.
	FallbackGeminiKey string

	// Stripe Billing (optional)
	StripeSecretKey       string
	StripeWebhookSecret   string
	StripePriceBasic      string
	StripePricePro        string
	StripePriceEnterprise string

	// WhatsApp Business API
	WhatsAppVerifyToken     string
	WhatsAppAppSecret       string
	WhatsAppAccessToken     string
	WhatsAppGraphAPIBaseURL string

	// Sentry Error Monitoring (optional)
	SentryDSN       string
	AlertWebhookURL string

	// CORS allowed origins
	AllowedOrigins []string
}

// Load reads .env file (if present) and returns a populated Config.
// In production, use real environment variables instead of .env file.
func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️  No .env file found, using system environment variables")
	}

	cfg := &Config{
		Port:                    getEnv("PORT", "8080"),
		Env:                     getEnv("ENV", "development"),
		DatabaseURL:             mustGetEnv("DATABASE_URL"),
		EncryptionKey:           getEnv("ENCRYPTION_KEY", ""),
		PreviousEncryptionKeys:  parseList(getEnv("ENCRYPTION_PREVIOUS_KEYS", "")),
		SupabaseURL:             getEnv("SUPABASE_URL", ""),
		SupabaseAnonKey:         getEnv("SUPABASE_ANON_KEY", ""),
		SupabaseSecretKey:       getEnv("SUPABASE_SECRET_KEY", ""),
		DevToken:                getEnv("DEV_TOKEN", ""),
		FrontendURL:             getEnv("FRONTEND_URL", "http://localhost:3000"),
		PublicAPIURL:            getEnv("PUBLIC_API_URL", "http://localhost:8080"),
		GeminiAPIKey:            getEnv("GEMINI_API_KEY", ""),
		OpenAIAPIKey:            getEnv("OPENAI_API_KEY", ""),
		GroqAPIKey:              getEnv("GROQ_API_KEY", ""),
		FallbackGeminiKey:       getEnv("FALLBACK_GEMINI_KEY", getEnv("GEMINI_API_KEY", "")),
		StripeSecretKey:         getEnv("STRIPE_SECRET_KEY", ""),
		StripeWebhookSecret:     getEnv("STRIPE_WEBHOOK_SECRET", ""),
		StripePriceBasic:        getEnv("STRIPE_PRICE_BASIC", ""),
		StripePricePro:          getEnv("STRIPE_PRICE_PRO", ""),
		StripePriceEnterprise:   getEnv("STRIPE_PRICE_ENTERPRISE", ""),
		WhatsAppVerifyToken:     getEnv("WHATSAPP_VERIFY_TOKEN", ""),
		WhatsAppAppSecret:       getEnv("WHATSAPP_APP_SECRET", ""),
		WhatsAppAccessToken:     getEnv("WHATSAPP_ACCESS_TOKEN", ""),
		WhatsAppGraphAPIBaseURL: getEnv("WHATSAPP_GRAPH_API_BASE_URL", ""),
		SentryDSN:               getEnv("SENTRY_DSN", ""),
		AlertWebhookURL:         getEnv("ALERT_WEBHOOK_URL", ""),
		AllowedOrigins:          parseList(getEnv("ALLOWED_ORIGINS", "http://localhost:3000,http://127.0.0.1:5500,http://localhost:5500")),
	}

	return cfg
}

// ValidateRuntime rejects unsafe or incomplete application configuration
// before the HTTP server starts accepting requests.
func (c *Config) ValidateRuntime() error {
	var problems []string
	environment := strings.ToLower(strings.TrimSpace(c.Env))
	switch environment {
	case "development", "test", "staging", "production":
	default:
		problems = append(problems, "ENV must be development, test, staging, or production")
	}

	if c.EncryptionKey != "" && len([]byte(c.EncryptionKey)) != 32 {
		problems = append(problems, "ENCRYPTION_KEY must be exactly 32 bytes")
	}
	for _, previousKey := range c.PreviousEncryptionKeys {
		if len([]byte(previousKey)) != 32 {
			problems = append(problems, "every ENCRYPTION_PREVIOUS_KEYS entry must be exactly 32 bytes")
		}
		if previousKey == c.EncryptionKey {
			problems = append(problems, "ENCRYPTION_PREVIOUS_KEYS must not include the current ENCRYPTION_KEY")
		}
	}
	if len(c.PreviousEncryptionKeys) > 0 && c.EncryptionKey == "" {
		problems = append(problems, "ENCRYPTION_KEY is required when ENCRYPTION_PREVIOUS_KEYS is configured")
	}

	if (c.SupabaseURL == "") != (c.SupabaseAnonKey == "") {
		problems = append(problems, "SUPABASE_URL and SUPABASE_ANON_KEY must be configured together")
	}
	if c.SupabaseURL != "" {
		parsed, err := url.Parse(c.SupabaseURL)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			problems = append(problems, "SUPABASE_URL must be a valid HTTPS URL")
		}
	}
	if c.FrontendURL != "" {
		parsed, err := url.Parse(c.FrontendURL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			problems = append(problems, "FRONTEND_URL must be a valid HTTP or HTTPS URL")
		}
	}
	if c.PublicAPIURL != "" {
		parsed, err := url.Parse(c.PublicAPIURL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			problems = append(problems, "PUBLIC_API_URL must be a valid HTTP or HTTPS URL")
		}
	}
	if c.AlertWebhookURL != "" {
		parsed, err := url.Parse(c.AlertWebhookURL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			problems = append(problems, "ALERT_WEBHOOK_URL must be a valid HTTP or HTTPS URL")
		} else if (environment == "staging" || environment == "production") && parsed.Scheme != "https" {
			problems = append(problems, "ALERT_WEBHOOK_URL must use HTTPS in staging and production")
		}
	}

	if (c.WhatsAppVerifyToken == "") != (c.WhatsAppAppSecret == "") {
		problems = append(problems, "WHATSAPP_VERIFY_TOKEN and WHATSAPP_APP_SECRET must be configured together")
	}
	if c.WhatsAppVerifyToken != "" && (c.WhatsAppAccessToken == "" || c.WhatsAppGraphAPIBaseURL == "") {
		problems = append(problems, "WHATSAPP_ACCESS_TOKEN and WHATSAPP_GRAPH_API_BASE_URL are required when WhatsApp webhooks are enabled")
	}
	if c.WhatsAppGraphAPIBaseURL != "" {
		parsed, err := url.Parse(c.WhatsAppGraphAPIBaseURL)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			problems = append(problems, "WHATSAPP_GRAPH_API_BASE_URL must be a valid HTTPS URL")
		}
	}
	stripeConfigured := c.StripeSecretKey != "" || c.StripeWebhookSecret != "" ||
		c.StripePriceBasic != "" || c.StripePricePro != "" || c.StripePriceEnterprise != ""
	if stripeConfigured && (c.StripeSecretKey == "" || c.StripeWebhookSecret == "" ||
		c.StripePriceBasic == "" || c.StripePricePro == "" || c.StripePriceEnterprise == "") {
		problems = append(problems, "Stripe configuration must include the secret, webhook secret, and all subscription Price IDs")
	}

	for _, origin := range c.AllowedOrigins {
		if origin == "*" {
			problems = append(problems, "ALLOWED_ORIGINS cannot contain * when credentials are enabled")
			continue
		}
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			problems = append(problems, fmt.Sprintf("invalid CORS origin %q", origin))
		}
	}

	if environment == "staging" || environment == "production" {
		if len([]byte(c.EncryptionKey)) != 32 {
			problems = append(problems, "ENCRYPTION_KEY is required in staging and production")
		}
		if c.SupabaseURL == "" || c.SupabaseAnonKey == "" {
			problems = append(problems, "Supabase Auth is required in staging and production")
		}
		if len(c.AllowedOrigins) == 0 {
			problems = append(problems, "ALLOWED_ORIGINS must contain at least one trusted origin")
		}
		if c.PublicAPIURL == "" {
			problems = append(problems, "PUBLIC_API_URL is required in staging and production")
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("invalid runtime configuration: %s", strings.Join(problems, "; "))
	}
	return nil
}

// IsDevelopment returns true when running in development mode.
func (c *Config) IsDevelopment() bool {
	return strings.EqualFold(c.Env, "development")
}

// --- Private helpers ---

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func mustGetEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		log.Fatalf("❌ Required environment variable %q is not set", key)
	}
	return val
}

func parseList(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
