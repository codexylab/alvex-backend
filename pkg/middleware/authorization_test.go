package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/codexylab/alvex-backend/pkg/models"
	"github.com/codexylab/alvex-backend/pkg/tenant"
)

func TestRequirePlatformRolesUsesDatabaseRole(t *testing.T) {
	handler := RequirePlatformRoles(PlatformSuperAdmin)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	allowedRequest := httptest.NewRequest(http.MethodPost, "/api/v1/platform/clients", nil)
	allowedRequest = withAuthenticatedUser(allowedRequest, AuthenticatedUser{
		ID:                "supabase_admin",
		ApplicationUserID: "admin_one",
		PlatformRole:      PlatformSuperAdmin,
	})
	allowedRecorder := httptest.NewRecorder()
	handler.ServeHTTP(allowedRecorder, allowedRequest)
	if allowedRecorder.Code != http.StatusNoContent {
		t.Fatalf("expected super admin access, got %d", allowedRecorder.Code)
	}

	deniedRequest := httptest.NewRequest(http.MethodPost, "/api/v1/platform/clients", nil)
	deniedRequest = withAuthenticatedUser(deniedRequest, AuthenticatedUser{
		ID:                "supabase_user",
		ApplicationUserID: "user_one",
		PlatformRole:      "platform_user",
	})
	deniedRecorder := httptest.NewRecorder()
	handler.ServeHTTP(deniedRecorder, deniedRequest)
	if deniedRecorder.Code != http.StatusForbidden {
		t.Fatalf("expected platform user rejection, got %d", deniedRecorder.Code)
	}
}

func TestRequireOrganizationRoles(t *testing.T) {
	handler := RequireOrganizationRoles(models.OrganizationOwner, models.OrganizationAdmin)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
	)

	tests := []struct {
		name       string
		role       string
		wantStatus int
	}{
		{name: "owner allowed", role: "owner", wantStatus: http.StatusNoContent},
		{name: "admin allowed", role: "admin", wantStatus: http.StatusNoContent},
		{name: "viewer rejected", role: "viewer", wantStatus: http.StatusForbidden},
		{name: "missing scope rejected", role: "", wantStatus: http.StatusForbidden},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/v1/clients", nil)
			if test.role != "" {
				request = request.WithContext(tenant.WithScope(request.Context(), tenant.Scope{
					OrganizationID: "org_test",
					Role:           test.role,
				}))
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != test.wantStatus {
				t.Fatalf("expected %d, got %d", test.wantStatus, recorder.Code)
			}
		})
	}
}
