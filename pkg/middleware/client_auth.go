package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/codexylab/alvex-backend/pkg/repository"
	"github.com/codexylab/alvex-backend/pkg/response"
)

type portalCtxKey string

const (
	portalClientIDKey portalCtxKey = "portal_client_id"
	portalRoleKey     portalCtxKey = "portal_role"

	// ClientHeader selects one client when a user has multiple assignments.
	ClientHeader = "X-Client-ID"
)

// RequireClientMembership maps a verified Supabase user to one active client.
// Long-lived shared portal tokens and query-string credentials are not accepted.
func RequireClientMembership(repo repository.ClientMembershipRepository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity, ok := GetAuthenticatedUser(r)
			if !ok || identity.ApplicationUserID == "" {
				response.Unauthorized(w)
				return
			}

			requestedClientID := strings.TrimSpace(r.Header.Get(ClientHeader))
			access, err := repo.ResolveActiveAccess(r.Context(), identity.ApplicationUserID, requestedClientID)
			switch {
			case errors.Is(err, repository.ErrClientSelectionRequired):
				response.BadRequest(w, "Select a client using X-Client-ID")
				return
			case errors.Is(err, repository.ErrClientMembershipNotFound):
				response.Forbidden(w)
				return
			case err != nil:
				response.InternalError(w)
				return
			}

			ctx := context.WithValue(r.Context(), portalClientIDKey, access.ClientID)
			ctx = context.WithValue(ctx, portalRoleKey, access.Role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetPortalClientID retrieves the authorized client ID from request context.
func GetPortalClientID(r *http.Request) string {
	if id, ok := r.Context().Value(portalClientIDKey).(string); ok {
		return id
	}
	return ""
}

// GetPortalRole retrieves the user's client-scoped role.
func GetPortalRole(r *http.Request) string {
	if role, ok := r.Context().Value(portalRoleKey).(string); ok {
		return role
	}
	return ""
}
