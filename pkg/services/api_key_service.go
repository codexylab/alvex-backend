package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/codexylab/alvex-backend/pkg/apierr"
	"github.com/codexylab/alvex-backend/pkg/crypto"
	"github.com/codexylab/alvex-backend/pkg/models"
	"github.com/codexylab/alvex-backend/pkg/repository"
	"github.com/codexylab/alvex-backend/pkg/tenant"
)

var allowedAPIKeyScopes = map[string]struct{}{
	string(models.APIKeyScopeClientsRead):        {},
	string(models.APIKeyScopeClientsWrite):       {},
	string(models.APIKeyScopeConversationsRead):  {},
	string(models.APIKeyScopeConversationsWrite): {},
	string(models.APIKeyScopeAnalyticsRead):      {},
}

type machineKeyGenerator func() (rawKey, prefix, hash string, err error)

// APIKeyService owns validation and lifecycle rules for machine credentials.
type APIKeyService struct {
	Repo        repository.APIKeyRepository
	now         func() time.Time
	generateKey machineKeyGenerator
}

func NewAPIKeyService(repo repository.APIKeyRepository) *APIKeyService {
	return &APIKeyService{
		Repo:        repo,
		now:         func() time.Time { return time.Now().UTC() },
		generateKey: crypto.GenerateMachineAPIKey,
	}
}

// Create validates tenant ownership and returns the raw credential exactly
// once. Only its SHA-256 digest is sent to the repository.
func (s *APIKeyService) Create(
	ctx context.Context,
	request models.CreateAPIKeyRequest,
	createdBy string,
) (*models.CreatedAPIKey, error) {
	organizationID, err := tenant.RequireOrganizationID(ctx)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(request.Name)
	if name == "" || len(name) > 120 {
		return nil, &apierr.ValidationError{Message: "name must contain between 1 and 120 characters"}
	}
	scopes, err := normalizeAPIKeyScopes(request.Scopes)
	if err != nil {
		return nil, err
	}
	now := s.now()
	if request.ExpiresAt != nil && !request.ExpiresAt.After(now) {
		return nil, &apierr.ValidationError{Message: "expires_at must be in the future"}
	}

	clientID := normalizeOptionalString(request.ClientID)
	if clientID != nil {
		belongs, err := s.Repo.ClientBelongsToOrganization(ctx, organizationID, *clientID)
		if err != nil {
			return nil, fmt.Errorf("verify API key client: %w", err)
		}
		if !belongs {
			return nil, &apierr.ValidationError{Message: "client_id does not belong to the active organization"}
		}
	}

	rawKey, prefix, secretHash, err := s.generateKey()
	if err != nil {
		return nil, err
	}
	creator := normalizeOptionalString(&createdBy)
	key := models.APIKey{
		ID:             uuid.NewString(),
		OrganizationID: organizationID,
		ClientID:       clientID,
		Name:           name,
		KeyPrefix:      prefix,
		Scopes:         scopes,
		ExpiresAt:      request.ExpiresAt,
		CreatedBy:      creator,
		CreatedAt:      now,
	}
	if err := s.Repo.Create(ctx, key, secretHash); err != nil {
		return nil, fmt.Errorf("create API key: %w", err)
	}
	return &models.CreatedAPIKey{APIKey: key, Secret: rawKey}, nil
}

func (s *APIKeyService) List(ctx context.Context) ([]models.APIKey, error) {
	organizationID, err := tenant.RequireOrganizationID(ctx)
	if err != nil {
		return nil, err
	}
	return s.Repo.List(ctx, organizationID)
}

// Revoke is tenant-scoped and idempotent for an already-revoked key.
func (s *APIKeyService) Revoke(ctx context.Context, keyID string) error {
	organizationID, err := tenant.RequireOrganizationID(ctx)
	if err != nil {
		return err
	}
	if _, err := uuid.Parse(strings.TrimSpace(keyID)); err != nil {
		return &apierr.ValidationError{Message: "API key ID is invalid"}
	}
	if err := s.Repo.Revoke(ctx, organizationID, keyID, s.now()); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &apierr.NotFoundError{Resource: "API key"}
		}
		return fmt.Errorf("revoke API key: %w", err)
	}
	return nil
}

func normalizeAPIKeyScopes(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, &apierr.ValidationError{Message: "at least one API key scope is required"}
	}
	unique := make(map[string]struct{}, len(values))
	for _, value := range values {
		scope := strings.ToLower(strings.TrimSpace(value))
		if _, ok := allowedAPIKeyScopes[scope]; !ok {
			return nil, &apierr.ValidationError{Message: fmt.Sprintf("unsupported API key scope %q", value)}
		}
		unique[scope] = struct{}{}
	}
	scopes := make([]string, 0, len(unique))
	for scope := range unique {
		scopes = append(scopes, scope)
	}
	sort.Strings(scopes)
	return scopes, nil
}

func normalizeOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
