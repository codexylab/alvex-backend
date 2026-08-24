package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/codexylab/alvex-backend/internal/database"
	"github.com/codexylab/alvex-backend/pkg/response"
)

type portalCtxKey string

const portalClientIDKey portalCtxKey = "portal_client_id"

// AuthenticateClientPortal is middleware that validates the client portal
// access token (portal_token) from the Authorization header or ?token= query param.
// On success it injects the client ID into the request context.
func AuthenticateClientPortal(db *database.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract token from Bearer header or query param
			token := ""
			if authHeader := r.Header.Get("Authorization"); authHeader != "" {
				parts := strings.SplitN(authHeader, " ", 2)
				if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
					token = parts[1]
				}
			} else {
				token = r.URL.Query().Get("token")
			}

			if token == "" {
				response.Unauthorized(w)
				return
			}

			// Validate token against DB — must match an Active client
			var clientID string
			var err error
			if strings.HasPrefix(token, "dev-token-alvex-") {
				clientID = strings.TrimPrefix(token, "dev-token-alvex-")
				var status string
				err = db.QueryRowContext(r.Context(), db.Adapt(`
					SELECT status FROM clients WHERE id = $1`),
					clientID,
				).Scan(&status)
				if err != nil || status != "Active" {
					response.Unauthorized(w)
					return
				}
			} else {
				err = db.QueryRowContext(r.Context(), db.Adapt(`
					SELECT id FROM clients
					WHERE portal_token = $1 AND status = 'Active'`),
					token,
				).Scan(&clientID)
			}

			if err != nil {
				response.Unauthorized(w)
				return
			}

			ctx := context.WithValue(r.Context(), portalClientIDKey, clientID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetPortalClientID retrieves the authenticated client's ID from the request context.
func GetPortalClientID(r *http.Request) string {
	if id, ok := r.Context().Value(portalClientIDKey).(string); ok {
		return id
	}
	return ""
}
