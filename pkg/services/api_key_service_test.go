package services

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/codexylab/alvex-backend/pkg/models"
	"github.com/codexylab/alvex-backend/pkg/tenant"
)

type apiKeyRepositoryStub struct {
	created         models.APIKey
	createdHash     string
	clientBelongs   bool
	revokedOrgID    string
	revokedKeyID    string
	revokeErr       error
	listedOrgID     string
	resolved        *models.APIKey
	markedUsedKeyID string
}

func (s *apiKeyRepositoryStub) Create(_ context.Context, key models.APIKey, hash string) error {
	s.created = key
	s.createdHash = hash
	return nil
}

func (s *apiKeyRepositoryStub) List(_ context.Context, organizationID string) ([]models.APIKey, error) {
	s.listedOrgID = organizationID
	return []models.APIKey{}, nil
}

func (s *apiKeyRepositoryStub) Revoke(_ context.Context, organizationID, keyID string, _ time.Time) error {
	s.revokedOrgID = organizationID
	s.revokedKeyID = keyID
	return s.revokeErr
}

func (s *apiKeyRepositoryStub) ClientBelongsToOrganization(_ context.Context, _, _ string) (bool, error) {
	return s.clientBelongs, nil
}

func (s *apiKeyRepositoryStub) ResolveActive(_ context.Context, _ string, _ time.Time) (*models.APIKey, error) {
	if s.resolved == nil {
		return nil, sql.ErrNoRows
	}
	return s.resolved, nil
}

func (s *apiKeyRepositoryStub) MarkUsed(_ context.Context, keyID string, _ time.Time) error {
	s.markedUsedKeyID = keyID
	return nil
}

func TestAPIKeyServiceCreatePersistsDigestAndReturnsSecretOnce(t *testing.T) {
	repo := &apiKeyRepositoryStub{clientBelongs: true}
	service := NewAPIKeyService(repo)
	now := time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	service.generateKey = func() (string, string, string, error) {
		return "alvx_sk_raw-secret", "alvx_sk_raw-se", "stored-digest", nil
	}
	ctx := tenant.WithScope(context.Background(), tenant.Scope{OrganizationID: "org_one"})
	clientID := "client_one"
	expiresAt := now.Add(24 * time.Hour)

	created, err := service.Create(ctx, models.CreateAPIKeyRequest{
		Name:      " Production automation ",
		ClientID:  &clientID,
		Scopes:    []string{"clients:read", "CLIENTS:READ", "analytics:read"},
		ExpiresAt: &expiresAt,
	}, "user_one")
	if err != nil {
		t.Fatalf("create API key: %v", err)
	}
	if created.Secret != "alvx_sk_raw-secret" || repo.createdHash != "stored-digest" {
		t.Fatalf("secret/digest boundary was not preserved: %#v / %q", created, repo.createdHash)
	}
	if repo.created.Name != "Production automation" || repo.created.OrganizationID != "org_one" {
		t.Fatalf("unexpected stored metadata: %#v", repo.created)
	}
	if len(repo.created.Scopes) != 2 || repo.created.Scopes[0] != "analytics:read" || repo.created.Scopes[1] != "clients:read" {
		t.Fatalf("expected normalized deterministic scopes, got %#v", repo.created.Scopes)
	}
}

func TestAPIKeyServiceRejectsCrossTenantClientAndPastExpiry(t *testing.T) {
	repo := &apiKeyRepositoryStub{clientBelongs: false}
	service := NewAPIKeyService(repo)
	now := time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	ctx := tenant.WithScope(context.Background(), tenant.Scope{OrganizationID: "org_one"})
	clientID := "client_two"

	_, err := service.Create(ctx, models.CreateAPIKeyRequest{
		Name:     "Cross tenant",
		ClientID: &clientID,
		Scopes:   []string{"clients:read"},
	}, "user_one")
	if err == nil {
		t.Fatal("expected a cross-tenant client binding to be rejected")
	}

	past := now.Add(-time.Minute)
	_, err = service.Create(ctx, models.CreateAPIKeyRequest{
		Name:      "Expired",
		Scopes:    []string{"clients:read"},
		ExpiresAt: &past,
	}, "user_one")
	if err == nil {
		t.Fatal("expected a past expiration to be rejected")
	}
}
