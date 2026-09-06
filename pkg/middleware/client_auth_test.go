package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/codexylab/alvex-backend/pkg/repository"
)

type clientMembershipRepositoryStub struct {
	access *repository.ClientAccess
	err    error
}

func (s clientMembershipRepositoryStub) ResolveActiveAccess(context.Context, string, string) (*repository.ClientAccess, error) {
	return s.access, s.err
}

func TestRequireClientMembershipAttachesVerifiedClient(t *testing.T) {
	repo := clientMembershipRepositoryStub{access: &repository.ClientAccess{
		ClientID: "client_one",
		UserID:   "app_user_one",
		Role:     "client_admin",
	}}
	handler := RequireClientMembership(repo)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := GetPortalClientID(r); got != "client_one" {
			t.Fatalf("expected client_one, got %q", got)
		}
		if got := GetPortalRole(r); got != "client_admin" {
			t.Fatalf("expected client_admin role, got %q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/client-portal/me", nil)
	request = withAuthenticatedUser(request, AuthenticatedUser{
		ID:                "supabase_user_one",
		ApplicationUserID: "app_user_one",
	})
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected %d, got %d", http.StatusNoContent, recorder.Code)
	}
}

func TestRequireClientMembershipRejectsUnassignedClient(t *testing.T) {
	repo := clientMembershipRepositoryStub{err: repository.ErrClientMembershipNotFound}
	handler := RequireClientMembership(repo)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/client-portal/me", nil)
	request.Header.Set(ClientHeader, "client_other")
	request = withAuthenticatedUser(request, AuthenticatedUser{
		ID:                "supabase_user_one",
		ApplicationUserID: "app_user_one",
	})
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected %d, got %d", http.StatusForbidden, recorder.Code)
	}
}

func TestRequireClientMembershipRejectsLegacyQueryToken(t *testing.T) {
	handler := RequireClientMembership(clientMembershipRepositoryStub{})(http.HandlerFunc(func(
		w http.ResponseWriter,
		_ *http.Request,
	) {
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/client-portal/me?token=dev-token-alvex-client_one", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d", http.StatusUnauthorized, recorder.Code)
	}
}
