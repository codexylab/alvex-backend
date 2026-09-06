package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/codexylab/alvex-backend/pkg/crypto"
	"github.com/codexylab/alvex-backend/pkg/models"
	"github.com/codexylab/alvex-backend/pkg/repository"
	"github.com/codexylab/alvex-backend/pkg/response"
	"github.com/codexylab/alvex-backend/pkg/tenant"
)

const APIKeyHeader = "X-API-Key"

type apiKeyContextKey struct{}

type APIKeyRequestLimiter interface {
	Allow(key string) bool
}

// APIKeyAuthenticator verifies one-way hashed machine credentials and attaches
// their trusted organization scope. Revoked, expired, and disabled-tenant keys
// are rejected by the repository lookup.
type APIKeyAuthenticator struct {
	Repo repository.APIKeyRepository
	Now  func() time.Time
}

func NewAPIKeyAuthenticator(repo repository.APIKeyRepository) *APIKeyAuthenticator {
	return &APIKeyAuthenticator{
		Repo: repo,
		Now:  func() time.Time { return time.Now().UTC() },
	}
}

func (a *APIKeyAuthenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		rawKey := strings.TrimSpace(r.Header.Get(APIKeyHeader))
		if len(rawKey) < 20 || len(rawKey) > 256 || !strings.HasPrefix(rawKey, "alvx_sk_") {
			response.AuthenticationFailed(w, "Invalid API key")
			return
		}

		now := a.Now()
		key, err := a.Repo.ResolveActive(r.Context(), crypto.HashAPIKey(rawKey), now)
		if err != nil {
			response.AuthenticationFailed(w, "Invalid API key")
			return
		}
		if err := a.Repo.MarkUsed(r.Context(), key.ID, now); err != nil {
			response.AuthenticationFailed(w, "Invalid API key")
			return
		}

		scope := tenant.Scope{
			OrganizationID: key.OrganizationID,
			UserID:         "api-key:" + key.ID,
			Role:           "api_key",
		}
		ctx := context.WithValue(r.Context(), apiKeyContextKey{}, *key)
		ctx = tenant.WithScope(ctx, scope)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAPIKeyScopes authorizes only when every requested scope is present.
func RequireAPIKeyScopes(required ...models.APIKeyScope) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key, ok := GetAPIKey(r)
			if !ok {
				response.AuthenticationFailed(w, "Invalid API key")
				return
			}
			granted := make(map[string]struct{}, len(key.Scopes))
			for _, scope := range key.Scopes {
				granted[scope] = struct{}{}
			}
			for _, scope := range required {
				if _, ok := granted[string(scope)]; !ok {
					response.Forbidden(w)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// LimitAPIKeyRequests applies a per-credential sliding-window limit after
// authentication, without retaining or logging the raw credential.
func LimitAPIKeyRequests(limiter APIKeyRequestLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key, ok := GetAPIKey(r)
			if !ok {
				response.AuthenticationFailed(w, "Invalid API key")
				return
			}
			if limiter != nil && !limiter.Allow(key.ID) {
				w.Header().Set("Retry-After", "60")
				response.JSON(w, http.StatusTooManyRequests, response.APIResponse{
					Success: false,
					Error:   "API key rate limit exceeded",
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAPIKeyClientAccess prevents a client-bound key from accessing a
// sibling client inside the same organization. Organization-wide keys pass.
func RequireAPIKeyClientAccess(paramName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key, ok := GetAPIKey(r)
			if !ok {
				response.AuthenticationFailed(w, "Invalid API key")
				return
			}
			if key.ClientID != nil && *key.ClientID != chi.URLParam(r, paramName) {
				response.Forbidden(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func GetAPIKey(r *http.Request) (models.APIKey, bool) {
	key, ok := r.Context().Value(apiKeyContextKey{}).(models.APIKey)
	return key, ok
}
