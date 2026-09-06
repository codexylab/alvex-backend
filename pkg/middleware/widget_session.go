package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/codexylab/alvex-backend/pkg/repository"
	"github.com/codexylab/alvex-backend/pkg/response"
)

type widgetSessionContextKey string

const widgetSessionKey widgetSessionContextKey = "widget_session"

type WidgetSessionResolver interface {
	Resolve(ctx context.Context, clientID, origin, token string) (*repository.WidgetSession, error)
}

// RequireWidgetSession authenticates an origin-bound, short-lived widget bearer
// token. Client-controlled session identifiers are never trusted.
func RequireWidgetSession(resolver WidgetSessionResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			clientID := strings.TrimSpace(chi.URLParam(r, "clientId"))
			origin := strings.TrimSpace(r.Header.Get("Origin"))
			token := bearerToken(r.Header.Get("Authorization"))
			if clientID == "" || origin == "" || token == "" {
				widgetUnauthorized(w)
				return
			}

			session, err := resolver.Resolve(r.Context(), clientID, origin, token)
			if err != nil {
				// Authentication failures deliberately use one response so callers
				// cannot distinguish expired tokens, client IDs, or dependency state.
				widgetUnauthorized(w)
				return
			}

			ctx := context.WithValue(r.Context(), widgetSessionKey, session)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetWidgetSession(r *http.Request) (*repository.WidgetSession, bool) {
	session, ok := r.Context().Value(widgetSessionKey).(*repository.WidgetSession)
	return session, ok && session != nil
}

func bearerToken(header string) string {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

func widgetUnauthorized(w http.ResponseWriter) {
	response.JSON(w, http.StatusUnauthorized, response.APIResponse{
		Success: false,
		Error:   "Invalid or expired widget session",
	})
}
