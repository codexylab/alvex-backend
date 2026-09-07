package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/lib/pq"

	"github.com/codexylab/alvex-backend/pkg/database"
	"github.com/codexylab/alvex-backend/pkg/models"
)

// APIKeyRepository stores machine credentials without ever persisting their
// raw secrets.
type APIKeyRepository interface {
	Create(ctx context.Context, key models.APIKey, secretHash string) error
	List(ctx context.Context, organizationID string) ([]models.APIKey, error)
	Revoke(ctx context.Context, organizationID, keyID string, revokedAt time.Time) error
	ClientBelongsToOrganization(ctx context.Context, organizationID, clientID string) (bool, error)
	ResolveActive(ctx context.Context, secretHash string, now time.Time) (*models.APIKey, error)
	MarkUsed(ctx context.Context, keyID string, usedAt time.Time) error
}

// SQLAPIKeyRepository is the SQL-backed credential store.
type SQLAPIKeyRepository struct {
	DB *database.DB
}

func NewSQLAPIKeyRepository(db *database.DB) *SQLAPIKeyRepository {
	return &SQLAPIKeyRepository{DB: db}
}

func (r *SQLAPIKeyRepository) Create(ctx context.Context, key models.APIKey, secretHash string) error {
	scopes, err := r.scopeValue(key.Scopes)
	if err != nil {
		return err
	}
	_, err = r.DB.ExecContext(ctx, r.DB.Adapt(`
		INSERT INTO api_keys
		  (id, organization_id, client_id, name, key_prefix, secret_hash, scopes,
		   expires_at, created_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`),
		key.ID, key.OrganizationID, key.ClientID, key.Name, key.KeyPrefix,
		secretHash, scopes, key.ExpiresAt, key.CreatedBy, key.CreatedAt,
	)
	return err
}

func (r *SQLAPIKeyRepository) List(ctx context.Context, organizationID string) ([]models.APIKey, error) {
	rows, err := r.DB.QueryContext(ctx, r.DB.Adapt(`
		SELECT id, organization_id, client_id, name, key_prefix, `+r.scopesSelect()+`,
		       expires_at, revoked_at, last_used_at, created_by, created_at
		FROM api_keys
		WHERE organization_id = $1
		ORDER BY created_at DESC, id DESC`), organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	keys := make([]models.APIKey, 0)
	for rows.Next() {
		key, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		keys = append(keys, *key)
	}
	return keys, rows.Err()
}

func (r *SQLAPIKeyRepository) Revoke(
	ctx context.Context,
	organizationID string,
	keyID string,
	revokedAt time.Time,
) error {
	result, err := r.DB.ExecContext(ctx, r.DB.Adapt(`
		UPDATE api_keys
		SET revoked_at = COALESCE(revoked_at, $1)
		WHERE id = $2 AND organization_id = $3`), revokedAt, keyID, organizationID)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *SQLAPIKeyRepository) ClientBelongsToOrganization(
	ctx context.Context,
	organizationID string,
	clientID string,
) (bool, error) {
	var exists bool
	err := r.DB.QueryRowContext(ctx, r.DB.Adapt(`
		SELECT EXISTS(
			SELECT 1 FROM clients WHERE id = $1 AND organization_id = $2
		)`), clientID, organizationID).Scan(&exists)
	return exists, err
}

func (r *SQLAPIKeyRepository) ResolveActive(
	ctx context.Context,
	secretHash string,
	now time.Time,
) (*models.APIKey, error) {
	row := r.DB.QueryRowContext(ctx, r.DB.Adapt(`
		SELECT k.id, k.organization_id, k.client_id, k.name, k.key_prefix, `+r.scopesSelect()+`,
		       k.expires_at, k.revoked_at, k.last_used_at, k.created_by, k.created_at
		FROM api_keys k
		JOIN organizations o ON o.id = k.organization_id AND o.status = 'active'
		LEFT JOIN clients c ON c.id = k.client_id
		WHERE k.secret_hash = $1
		  AND k.revoked_at IS NULL
		  AND (k.expires_at IS NULL OR k.expires_at > $2)
		  AND (k.client_id IS NULL OR c.status = 'Active')`), secretHash, now)
	return scanAPIKey(row)
}

func (r *SQLAPIKeyRepository) MarkUsed(ctx context.Context, keyID string, usedAt time.Time) error {
	_, err := r.DB.ExecContext(ctx, r.DB.Adapt(`
		UPDATE api_keys SET last_used_at = $1
		WHERE id = $2 AND (last_used_at IS NULL OR last_used_at < $3)`),
		usedAt, keyID, usedAt.Add(-time.Minute))
	return err
}

func (r *SQLAPIKeyRepository) scopesSelect() string {
	if r.DB.IsSQLite() {
		return "scopes"
	}
	return "array_to_json(scopes)::text"
}

func (r *SQLAPIKeyRepository) scopeValue(scopes []string) (interface{}, error) {
	if !r.DB.IsSQLite() {
		return pq.Array(scopes), nil
	}
	value, err := json.Marshal(scopes)
	if err != nil {
		return nil, err
	}
	return string(value), nil
}

type apiKeyScanner interface {
	Scan(dest ...interface{}) error
}

func scanAPIKey(scanner apiKeyScanner) (*models.APIKey, error) {
	var key models.APIKey
	var scopesJSON string
	if err := scanner.Scan(
		&key.ID,
		&key.OrganizationID,
		&key.ClientID,
		&key.Name,
		&key.KeyPrefix,
		&scopesJSON,
		&key.ExpiresAt,
		&key.RevokedAt,
		&key.LastUsedAt,
		&key.CreatedBy,
		&key.CreatedAt,
	); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(scopesJSON), &key.Scopes); err != nil {
		return nil, err
	}
	if key.Scopes == nil {
		key.Scopes = []string{}
	}
	return &key, nil
}
