package middleware

import (
	"context"
	"net/http"
	"os"
	"strings"

	"github.com/clerkinc/clerk-sdk-go/clerk"
	"github.com/codexylab/alvex-backend/pkg/response"
)

type contextKey string

const (
	UserIDKey contextKey = "user_id"
)

var (
	clerkClient clerk.Client
	devToken    string
)

func init() {
	devToken = os.Getenv("DEV_TOKEN")

	// Initialize Clerk client if secret key is available
	if secretKey := os.Getenv("CLERK_SECRET_KEY"); secretKey != "" {
		var err error
		clerkClient, err = clerk.NewClient(secretKey)
		if err != nil {
			// Log warning but don't crash — dev mode can still work
		}
	}
}

// Authenticate validates the Bearer token on every protected request.
// In development mode, a hardcoded DEV_TOKEN bypasses Clerk verification.
// In production, the token is verified using the Clerk SDK.
func Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		if token == "" {
			response.Unauthorized(w)
			return
		}

		isDev := os.Getenv("ENV") == "development"

		// Development bypass: accept hardcoded dev token
		if isDev && devToken != "" && token == devToken {
			ctx := context.WithValue(r.Context(), UserIDKey, "dev-user-001")
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Production: verify token with Clerk
		if clerkClient == nil {
			// Clerk not configured and not in dev mode
			response.Unauthorized(w)
			return
		}

		sessClaims, err := clerkClient.VerifyToken(token)
		if err != nil {
			response.Unauthorized(w)
			return
		}

		userID := sessClaims.Claims.Subject
		if userID == "" {
			response.Unauthorized(w)
			return
		}

		ctx := context.WithValue(r.Context(), UserIDKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
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
