package config

import (
	"log"
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

	// Clerk Authentication
	ClerkSecretKey      string
	ClerkPublishableKey string
	ClerkWebhookSecret  string

	// Encryption key for API key storage (AES-256, exactly 32 chars)
	EncryptionKey string

	// AI Providers
	GeminiAPIKey      string
	OpenAIAPIKey      string
	GroqAPIKey        string
	// FallbackGeminiKey is the platform-level Gemini API key used to automatically
	// handle requests when the client's primary provider (Groq/OpenAI) fails.
	FallbackGeminiKey string

	// Stripe Billing (optional)
	StripeSecretKey     string
	StripeWebhookSecret string

	// WhatsApp Business API
	WhatsAppVerifyToken string
	WhatsAppAppSecret   string

	// Sentry Error Monitoring (optional)
	SentryDSN string

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
		Port:                getEnv("PORT", "8080"),
		Env:                 getEnv("ENV", "development"),
		DatabaseURL:         mustGetEnv("DATABASE_URL"),
		ClerkSecretKey:      getEnv("CLERK_SECRET_KEY", ""),      // optional in dev
		ClerkPublishableKey: getEnv("CLERK_PUBLISHABLE_KEY", ""), // optional in dev
		ClerkWebhookSecret:  getEnv("CLERK_WEBHOOK_SECRET", ""),  // for verifying Clerk webhook events
		EncryptionKey:       getEnv("ENCRYPTION_KEY", ""),
		GeminiAPIKey:        getEnv("GEMINI_API_KEY", ""),
		OpenAIAPIKey:        getEnv("OPENAI_API_KEY", ""),
		GroqAPIKey:          getEnv("GROQ_API_KEY", ""),
		FallbackGeminiKey:   getEnv("FALLBACK_GEMINI_KEY", getEnv("GEMINI_API_KEY", "")),
		StripeSecretKey:     getEnv("STRIPE_SECRET_KEY", ""),
		StripeWebhookSecret: getEnv("STRIPE_WEBHOOK_SECRET", ""),
		WhatsAppVerifyToken: getEnv("WHATSAPP_VERIFY_TOKEN", "alvex-verify"),
		WhatsAppAppSecret:   getEnv("WHATSAPP_APP_SECRET", ""),
		SentryDSN:           getEnv("SENTRY_DSN", ""),
		AllowedOrigins:      parseList(getEnv("ALLOWED_ORIGINS", "http://localhost:3000,http://127.0.0.1:5500,http://localhost:5500")),
	}


	return cfg
}

// IsDevelopment returns true when running in development mode.
func (c *Config) IsDevelopment() bool {
	return c.Env == "development"
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
