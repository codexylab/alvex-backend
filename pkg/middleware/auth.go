package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/codexylab/alvex-backend/pkg/repository"
	"github.com/codexylab/alvex-backend/pkg/response"
)

type contextKey string

const (
	UserIDKey contextKey = "user_id"
	userKey   contextKey = "authenticated_user"
)

// AuthenticatedUser contains claims trusted only after cryptographic JWT validation.
type AuthenticatedUser struct {
	ID                string
	ApplicationUserID string
	Email             string
	Name              string
	PlatformRole      string
}

// Authenticator owns immutable authentication dependencies.
type Authenticator struct {
	verifier    TokenVerifier
	userRepo    repository.UserRepository
	development bool
	devToken    string
}

func NewAuthenticator(
	supabaseURL, supabaseAnonKey, environment, devToken string,
	userRepo repository.UserRepository,
) *Authenticator {
	authenticator := &Authenticator{
		userRepo:    userRepo,
		development: strings.EqualFold(environment, "development"),
		devToken:    devToken,
	}
	if supabaseURL != "" && supabaseAnonKey != "" {
		authenticator.verifier = NewJWKSVerifier(supabaseURL)
	}
	return authenticator
}

// Middleware verifies Supabase JWTs locally. The profile bootstrap endpoint may
// create app_users; all other protected routes require an active database user.
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawToken := extractBearerToken(r)
		if rawToken == "" {
			response.Unauthorized(w)
			return
		}

		if a.verifier != nil {
			user, err := a.verifier.Verify(r.Context(), rawToken)
			if err != nil {
				response.Unauthorized(w)
				return
			}
			if r.URL.Path != "/api/v1/auth/me" && a.userRepo != nil {
				applicationUser, err := a.userRepo.GetByID(r.Context(), user.ID)
				if err != nil || applicationUser.Status != "active" {
					response.Unauthorized(w)
					return
				}
				user.ApplicationUserID = applicationUser.ID
				user.PlatformRole = applicationUser.PlatformRole
			}
			next.ServeHTTP(w, withAuthenticatedUser(r, user))
			return
		}

		if a.development && a.devToken != "" && rawToken == a.devToken {
			user := AuthenticatedUser{
				ID:                "dev-user-001",
				ApplicationUserID: "dev-user-001",
				Email:             "dev@localhost",
				Name:              "Development User",
				PlatformRole:      "super_admin",
			}
			next.ServeHTTP(w, withAuthenticatedUser(r, user))
			return
		}

		response.Unauthorized(w)
	})
}

func withAuthenticatedUser(r *http.Request, user AuthenticatedUser) *http.Request {
	userID := user.ApplicationUserID
	if userID == "" {
		userID = user.ID
	}
	ctx := context.WithValue(r.Context(), UserIDKey, userID)
	ctx = context.WithValue(ctx, userKey, user)
	return r.WithContext(ctx)
}

func GetUserID(r *http.Request) string {
	if id, ok := r.Context().Value(UserIDKey).(string); ok {
		return id
	}
	return ""
}

func GetAuthenticatedUser(r *http.Request) (AuthenticatedUser, bool) {
	user, ok := r.Context().Value(userKey).(AuthenticatedUser)
	return user, ok
}

// extractBearerToken accepts query tokens only for the browser WebSocket API.
func extractBearerToken(r *http.Request) string {
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return parts[1]
		}
	}
	if r.URL.Path == "/ws/activity" {
		return r.URL.Query().Get("token")
	}
	return ""
}
