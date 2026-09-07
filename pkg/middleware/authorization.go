package middleware

import (
	"net/http"

	"github.com/codexylab/alvex-backend/pkg/models"
	"github.com/codexylab/alvex-backend/pkg/response"
	"github.com/codexylab/alvex-backend/pkg/tenant"
)

const PlatformSuperAdmin = "super_admin"

// RequirePlatformRoles authorizes platform-wide operations from the active
// database user loaded by Authenticator, never from mutable JWT metadata.
func RequirePlatformRoles(roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		allowed[role] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := GetAuthenticatedUser(r)
			if !ok || user.ApplicationUserID == "" {
				response.Unauthorized(w)
				return
			}
			if _, ok := allowed[user.PlatformRole]; !ok {
				response.Forbidden(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireOrganizationRoles authorizes a request using the verified membership
// role already attached by RequireOrganization.
func RequireOrganizationRoles(roles ...models.OrganizationRole) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		allowed[string(role)] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scope, ok := tenant.FromContext(r.Context())
			if !ok {
				response.Forbidden(w)
				return
			}
			if _, ok := allowed[scope.Role]; !ok {
				response.Forbidden(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
