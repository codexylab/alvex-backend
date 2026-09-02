package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/codexylab/alvex-backend/pkg/response"
)

type contextKey string

const (
	UserIDKey contextKey = "user_id"
)

// Authenticate validates the Bearer token on every protected request.
//
// Priority order:
//  1. If SUPABASE_URL + SUPABASE_ANON_KEY are set → always verify via Supabase JWT
//     (works for both local dev and production when Supabase auth is configured)
//  2. If Supabase is NOT configured → fall back to DEV_TOKEN for local testing only
//
// This fixes the init() timing bug where env vars were captured before godotenv loaded.
// All env vars are now read dynamically per-request via os.Getenv().
func Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		if token == "" {
			response.Unauthorized(w)
			return
		}

		// Read Supabase config dynamically (after godotenv has loaded .env)
		supabaseURL := os.Getenv("SUPABASE_URL")
		supabaseKey := os.Getenv("SUPABASE_ANON_KEY")

		// --- Priority 1: Supabase JWT verification (local + production) ---
		// When Supabase is configured, always verify via Supabase regardless of ENV.
		if supabaseURL != "" && supabaseKey != "" {
			userID, err := verifySupabaseToken(token, supabaseURL, supabaseKey)
			if err != nil {
				response.Unauthorized(w)
				return
			}
			ctx := context.WithValue(r.Context(), UserIDKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// --- Priority 2: Dev token fallback (only when Supabase is NOT configured) ---
		// Useful for pure local testing without any Supabase project.
		isDev := os.Getenv("ENV") == "development"
		expectedToken := os.Getenv("DEV_TOKEN")
		if expectedToken == "" {
			expectedToken = "dev-token-alvex-2024"
		}

		if isDev && token == expectedToken {
			ctx := context.WithValue(r.Context(), UserIDKey, "dev-user-001")
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Neither Supabase nor dev token matched — reject
		response.Unauthorized(w)
	})
}

// verifySupabaseToken verifies a Supabase JWT by calling the Supabase Auth REST API.
// supabaseURL and supabaseKey are passed in (not read from globals) to avoid the
// init() timing bug where package-level vars captured empty strings before godotenv loaded.
func verifySupabaseToken(token, supabaseURL, supabaseKey string) (string, error) {
	req, err := http.NewRequest("GET", supabaseURL+"/auth/v1/user", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("apikey", supabaseKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("invalid token: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var user struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &user); err != nil || user.ID == "" {
		return "", fmt.Errorf("invalid user response")
	}

	return user.ID, nil
}

// GetUserID retrieves the authenticated user's ID from the request context.
func GetUserID(r *http.Request) string {
	if id, ok := r.Context().Value(UserIDKey).(string); ok {
		return id
	}
	return ""
}

// extractBearerToken extracts the token from the Authorization header or ?token= query param.
func extractBearerToken(r *http.Request) string {
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return parts[1]
		}
	}
	return r.URL.Query().Get("token")
}

