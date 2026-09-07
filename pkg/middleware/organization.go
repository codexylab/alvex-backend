package middleware

import (
	"net/http"
	"strings"

	"github.com/codexylab/alvex-backend/pkg/repository"
	"github.com/codexylab/alvex-backend/pkg/response"
	"github.com/codexylab/alvex-backend/pkg/tenant"
)

const OrganizationHeader = "X-Organization-ID"

// RequireOrganization resolves an active membership and attaches the trusted
// tenant scope. Users with multiple memberships must select one explicitly.
func RequireOrganization(repo repository.OrganizationRepository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := GetUserID(r)
			if userID == "" {
				response.Unauthorized(w)
				return
			}

			memberships, err := repo.ListActiveMemberships(r.Context(), userID)
			if err != nil {
				response.InternalError(w)
				return
			}
			requestedID := strings.TrimSpace(r.Header.Get(OrganizationHeader))
			if requestedID == "" && r.URL.Path == "/ws/activity" {
				requestedID = strings.TrimSpace(r.URL.Query().Get("organization_id"))
			}
			if requestedID == "" && len(memberships) > 1 {
				response.BadRequest(w, OrganizationHeader+" is required when the user belongs to multiple organizations")
				return
			}

			for _, membership := range memberships {
				if requestedID == "" || membership.Organization.ID == requestedID {
					scope := tenant.Scope{
						OrganizationID:   membership.Organization.ID,
						OrganizationType: string(membership.Organization.Type),
						UserID:           userID,
						Role:             string(membership.Role),
					}
					next.ServeHTTP(w, r.WithContext(tenant.WithScope(r.Context(), scope)))
					return
				}
			}

			response.Forbidden(w)
		})
	}
}
