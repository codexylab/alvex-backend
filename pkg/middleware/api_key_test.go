package middleware

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/codexylab/alvex-backend/pkg/crypto"
	"github.com/codexylab/alvex-backend/pkg/models"
	"github.com/codexylab/alvex-backend/pkg/tenant"
)

type middlewareAPIKeyRepositoryStub struct {
	key          *models.APIKey
	resolvedHash string
	usedKeyID    string
}

func (s *middlewareAPIKeyRepositoryStub) Create(context.Context, models.APIKey, string) error {
	return nil
}

func (s *middlewareAPIKeyRepositoryStub) List(context.Context, string) ([]models.APIKey, error) {
	return nil, nil
}

func (s *middlewareAPIKeyRepositoryStub) Revoke(context.Context, string, string, time.Time) error {
	return nil
}

func (s *middlewareAPIKeyRepositoryStub) ClientBelongsToOrganization(context.Context, string, string) (bool, error) {
	return true, nil
}

func (s *middlewareAPIKeyRepositoryStub) ResolveActive(_ context.Context, hash string, _ time.Time) (*models.APIKey, error) {
	s.resolvedHash = hash
	if s.key == nil {
		return nil, sql.ErrNoRows
	}
	return s.key, nil
}

func (s *middlewareAPIKeyRepositoryStub) MarkUsed(_ context.Context, keyID string, _ time.Time) error {
	s.usedKeyID = keyID
	return nil
}

func TestAPIKeyMiddlewareAttachesTenantAndEnforcesScopeAndClient(t *testing.T) {
	clientID := "client_one"
	repo := &middlewareAPIKeyRepositoryStub{key: &models.APIKey{
		ID:             "key_one",
		OrganizationID: "org_one",
		ClientID:       &clientID,
		Scopes:         []string{"clients:read"},
	}}
	authenticator := NewAPIKeyAuthenticator(repo)
	router := chi.NewRouter()
	router.With(
		authenticator.Middleware,
		RequireAPIKeyScopes(models.APIKeyScopeClientsRead),
		RequireAPIKeyClientAccess("id"),
	).Get("/clients/{id}", func(w http.ResponseWriter, r *http.Request) {
		scope, ok := tenant.FromContext(r.Context())
		if !ok || scope.OrganizationID != "org_one" || scope.UserID != "api-key:key_one" {
			t.Fatalf("unexpected tenant scope: %#v", scope)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/clients/client_one", nil)
	request.Header.Set(APIKeyHeader, "alvx_sk_valid-machine-credential")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected authorized request, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if repo.resolvedHash != crypto.HashAPIKey("alvx_sk_valid-machine-credential") || repo.usedKeyID != "key_one" {
		t.Fatalf("credential lookup/use tracking mismatch: %q / %q", repo.resolvedHash, repo.usedKeyID)
	}
}

func TestAPIKeyMiddlewareRejectsMissingScopeAndSiblingClient(t *testing.T) {
	clientID := "client_one"
	repo := &middlewareAPIKeyRepositoryStub{key: &models.APIKey{
		ID:             "key_one",
		OrganizationID: "org_one",
		ClientID:       &clientID,
		Scopes:         []string{"analytics:read"},
	}}
	authenticator := NewAPIKeyAuthenticator(repo)
	router := chi.NewRouter()
	router.With(
		authenticator.Middleware,
		RequireAPIKeyScopes(models.APIKeyScopeClientsRead),
		RequireAPIKeyClientAccess("id"),
	).Get("/clients/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/clients/client_two", nil)
	request.Header.Set(APIKeyHeader, "alvx_sk_valid-machine-credential")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected missing scope rejection, got %d", recorder.Code)
	}

	repo.key.Scopes = []string{"clients:read"}
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected sibling client rejection, got %d", recorder.Code)
	}
}
