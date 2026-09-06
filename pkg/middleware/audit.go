package middleware

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/codexylab/alvex-backend/pkg/repository"
	"github.com/codexylab/alvex-backend/pkg/tenant"
)

// AuditMutations records successful tenant-owned write requests without
// persisting request bodies or secrets.
func AuditMutations(repo repository.AuditRepository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isMutationMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}

			wrapped := &auditResponseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(wrapped, r)
			if wrapped.status < 200 || wrapped.status >= 300 {
				return
			}
			scope, ok := tenant.FromContext(r.Context())
			if !ok {
				return
			}

			routePattern := chi.RouteContext(r.Context()).RoutePattern()
			entry := repository.AuditEntry{
				ID:             uuid.NewString(),
				OrganizationID: scope.OrganizationID,
				ActorUserID:    scope.UserID,
				Action:         r.Method + " " + routePattern,
				ResourceType:   resourceType(routePattern),
				ResourceID:     resourceID(r),
				Metadata: map[string]interface{}{
					"request_id": GetRequestID(r),
					"status":     wrapped.status,
				},
				IPAddress: remoteIP(r.RemoteAddr),
				CreatedAt: time.Now().UTC(),
			}
			if err := repo.InsertAuditEntry(r.Context(), entry); err != nil {
				slog.Error("failed to persist audit entry", "error", err, "action", entry.Action)
			}
		})
	}
}

type auditResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *auditResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func isMutationMethod(method string) bool {
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch || method == http.MethodDelete
}

func resourceID(r *http.Request) string {
	if id := chi.URLParam(r, "docId"); id != "" {
		return id
	}
	return chi.URLParam(r, "id")
}

func resourceType(routePattern string) string {
	parts := strings.Split(strings.Trim(routePattern, "/"), "/")
	for index, part := range parts {
		if part == "v1" && index+1 < len(parts) {
			return parts[index+1]
		}
	}
	return "unknown"
}

func remoteIP(remoteAddress string) string {
	host, _, err := net.SplitHostPort(remoteAddress)
	if err == nil {
		return host
	}
	return remoteAddress
}
