package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/codexylab/alvex-backend/pkg/models"
	"github.com/codexylab/alvex-backend/pkg/tenant"
)

type membershipRepositoryStub struct {
	memberships []models.OrganizationMembership
}

func (s membershipRepositoryStub) ListActiveMemberships(context.Context, string) ([]models.OrganizationMembership, error) {
	return s.memberships, nil
}

func TestRequireOrganizationAttachesVerifiedMembership(t *testing.T) {
	repo := membershipRepositoryStub{memberships: []models.OrganizationMembership{{
		Organization: models.Organization{ID: "org_one", Type: models.OrganizationDirectCustomer},
		Role:         models.OrganizationOwner,
		Status:       "active",
	}}}
	handler := RequireOrganization(repo)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scope, ok := tenant.FromContext(r.Context())
		if !ok || scope.OrganizationID != "org_one" || scope.Role != "owner" {
			t.Fatalf("unexpected organization scope: %#v", scope)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/clients", nil)
	request = request.WithContext(context.WithValue(request.Context(), UserIDKey, "user_one"))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected %d, got %d", http.StatusNoContent, recorder.Code)
	}
}

func TestRequireOrganizationRejectsUnownedSelection(t *testing.T) {
	repo := membershipRepositoryStub{memberships: []models.OrganizationMembership{{
		Organization: models.Organization{ID: "org_one"},
		Role:         models.OrganizationViewer,
		Status:       "active",
	}}}
	handler := RequireOrganization(repo)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/clients", nil)
	request.Header.Set(OrganizationHeader, "org_other")
	request = request.WithContext(context.WithValue(request.Context(), UserIDKey, "user_one"))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected %d, got %d", http.StatusForbidden, recorder.Code)
	}
}

func TestRequireOrganizationDemandsSelectionForMultipleMemberships(t *testing.T) {
	repo := membershipRepositoryStub{memberships: []models.OrganizationMembership{
		{Organization: models.Organization{ID: "org_one"}, Status: "active"},
		{Organization: models.Organization{ID: "org_two"}, Status: "active"},
	}}
	handler := RequireOrganization(repo)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/clients", nil)
	request = request.WithContext(context.WithValue(request.Context(), UserIDKey, "user_one"))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d", http.StatusBadRequest, recorder.Code)
	}
}
