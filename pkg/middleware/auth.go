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

var (
	supabaseURL string
	supabaseKey string
	devToken    string
)

func init() {
	devToken = os.Getenv("DEV_TOKEN")
	supabaseURL = os.Getenv("SUPABASE_URL")
	supabaseKey = os.Getenv("SUPABASE_ANON_KEY")
}

// Authenticate validates the Bearer token on every protected request.
// In development mode, a hardcoded DEV_TOKEN bypasses verification.
// In production, the token is verified via Supabase Auth API.
func Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		if token == "" {
			response.Unauthorized(w)
			return
		}

		isDev := os.Getenv("ENV") == "development"

		// Development bypass: accept hardcoded dev token
		expectedToken := os.Getenv("DEV_TOKEN")
		if expectedToken == "" {
			expectedToken = devToken
		}
		if expectedToken == "" {
			expectedToken = "dev-token-alvex-2024"
		}

		if isDev && token == expectedToken {
			ctx := context.WithValue(r.Context(), UserIDKey, "dev-user-001")
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}


		// Production: verify token via Supabase Auth API
		userID, err := verifySupabaseToken(token)
		if err != nil {
			response.Unauthorized(w)
			return
		}

		ctx := context.WithValue(r.Context(), UserIDKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// verifySupabaseToken verifies a Supabase token by calling the Supabase Auth API.
// This works with any JWT signing algorithm (ECC, RSA, HS256).
func verifySupabaseToken(token string) (string, error) {
	if supabaseURL == "" || supabaseKey == "" {
		return "", fmt.Errorf("supabase not configured")
	}

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

