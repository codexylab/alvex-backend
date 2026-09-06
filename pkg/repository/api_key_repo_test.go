package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/codexylab/alvex-backend/pkg/database"
	"github.com/codexylab/alvex-backend/pkg/models"
)

func TestAPIKeyRepositoryResolveExpiryRevocationAndSecretBoundary(t *testing.T) {
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer sqlDB.Close()
	db := database.NewDB(sqlDB, "sqlite")
	createAPIKeyTestSchema(t, db)

	now := time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC)
	_, err = db.Exec(`
		INSERT INTO organizations (id, status) VALUES ('org_one', 'active');
		INSERT INTO app_users (id) VALUES ('user_one');
		INSERT INTO clients (id, organization_id, status) VALUES ('client_one', 'org_one', 'Active')`)
	if err != nil {
		t.Fatalf("seed API key test: %v", err)
	}

	repo := NewSQLAPIKeyRepository(db)
	clientID := "client_one"
	createdBy := "user_one"
	expiresAt := now.Add(time.Hour)
	key := models.APIKey{
		ID:             "key_one",
		OrganizationID: "org_one",
		ClientID:       &clientID,
		Name:           "Automation",
		KeyPrefix:      "alvx_sk_prefix",
		Scopes:         []string{"clients:read"},
		ExpiresAt:      &expiresAt,
		CreatedBy:      &createdBy,
		CreatedAt:      now,
	}
	const rawSecret = "alvx_sk_raw-secret-never-store"
	const secretHash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if err := repo.Create(context.Background(), key, secretHash); err != nil {
		t.Fatalf("create API key: %v", err)
	}

	var storedHash string
	if err := db.QueryRow(`SELECT secret_hash FROM api_keys WHERE id = 'key_one'`).Scan(&storedHash); err != nil {
		t.Fatalf("read persisted digest: %v", err)
	}
	if storedHash != secretHash || storedHash == rawSecret {
		t.Fatalf("expected digest-only persistence, got %q", storedHash)
	}

	resolved, err := repo.ResolveActive(context.Background(), secretHash, now)
	if err != nil {
		t.Fatalf("resolve active API key: %v", err)
	}
	if resolved.ID != "key_one" || len(resolved.Scopes) != 1 || resolved.Scopes[0] != "clients:read" {
		t.Fatalf("unexpected resolved key: %#v", resolved)
	}
	if _, err := repo.ResolveActive(context.Background(), secretHash, expiresAt); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected key to expire at its deadline, got %v", err)
	}

	if err := repo.Revoke(context.Background(), "org_one", "key_one", now.Add(time.Minute)); err != nil {
		t.Fatalf("revoke API key: %v", err)
	}
	if _, err := repo.ResolveActive(context.Background(), secretHash, now); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected revoked key rejection, got %v", err)
	}
	keys, err := repo.List(context.Background(), "org_one")
	if err != nil || len(keys) != 1 || keys[0].RevokedAt == nil {
		t.Fatalf("expected revoked metadata in tenant list, got %#v / %v", keys, err)
	}
}

func createAPIKeyTestSchema(t *testing.T, db *database.DB) {
	t.Helper()
	_, err := db.Exec(`
		PRAGMA foreign_keys = ON;
		CREATE TABLE organizations (id TEXT PRIMARY KEY, status TEXT NOT NULL);
		CREATE TABLE app_users (id TEXT PRIMARY KEY);
		CREATE TABLE clients (
			id TEXT PRIMARY KEY,
			organization_id TEXT NOT NULL REFERENCES organizations(id),
			status TEXT NOT NULL
		);
		CREATE TABLE api_keys (
			id TEXT PRIMARY KEY,
			organization_id TEXT NOT NULL REFERENCES organizations(id),
			client_id TEXT REFERENCES clients(id),
			name TEXT NOT NULL,
			key_prefix TEXT NOT NULL,
			secret_hash TEXT NOT NULL UNIQUE,
			scopes TEXT NOT NULL,
			expires_at DATETIME,
			revoked_at DATETIME,
			last_used_at DATETIME,
			created_by TEXT REFERENCES app_users(id),
			created_at DATETIME NOT NULL
		)`)
	if err != nil {
		t.Fatalf("create API key schema: %v", err)
	}
}
