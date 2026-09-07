package repository

import (
	"context"

	"github.com/codexylab/alvex-backend/pkg/database"
)

// User represents a user record in the DB.
type User struct {
	ID             string
	SupabaseUserID string
	Name           string
	Email          string
	PlatformRole   string
	Status         string
}

// UserRepository defines interface for user database operations.
type UserRepository interface {
	GetByID(ctx context.Context, id string) (*User, error)
	Upsert(ctx context.Context, id, name, email string) error
}

// SQLUserRepository implements UserRepository for SQL databases.
type SQLUserRepository struct {
	DB *database.DB
}

// NewSQLUserRepository creates a new SQLUserRepository instance.
func NewSQLUserRepository(db *database.DB) *SQLUserRepository {
	return &SQLUserRepository{DB: db}
}

// GetByID retrieves a user by their ID.
func (r *SQLUserRepository) GetByID(ctx context.Context, id string) (*User, error) {
	u := &User{}
	err := r.DB.QueryRowContext(ctx, r.DB.Adapt(`
		SELECT id, supabase_user_id, name, email, platform_role, status
		FROM app_users
		WHERE supabase_user_id = $1
	`), id).Scan(&u.ID, &u.SupabaseUserID, &u.Name, &u.Email, &u.PlatformRole, &u.Status)
	if err != nil {
		return nil, err
	}
	return u, nil
}

// Upsert inserts a new user or updates their name/email if they already exist.
// Called after Supabase returns a verified user identity.
// Compatible with both SQLite (development) and PostgreSQL (Supabase production).
func (r *SQLUserRepository) Upsert(ctx context.Context, id, name, email string) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, r.DB.Adapt(`
		INSERT INTO app_users
		  (id, supabase_user_id, name, email, platform_role, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'platform_user', 'active', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (supabase_user_id) DO UPDATE
		  SET name       = EXCLUDED.name,
		      email      = EXCLUDED.email,
		      updated_at = CURRENT_TIMESTAMP
	`), id, id, name, email)
	if err != nil {
		return err
	}

	// Accept any pending invitation for this verified Supabase identity and
	// activate only the memberships connected to that invitation.
	if _, err = tx.ExecContext(ctx, r.DB.Adapt(`
		UPDATE client_memberships
		SET status = 'active', updated_at = CURRENT_TIMESTAMP
		WHERE user_id = $1 AND status = 'invited'
		  AND client_id IN (
		    SELECT client_id FROM identity_invitations
		    WHERE supabase_user_id = $2 AND status = 'sent'
		  )`), id, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, r.DB.Adapt(`
		UPDATE organization_memberships
		SET status = 'active', updated_at = CURRENT_TIMESTAMP
		WHERE user_id = $1 AND status = 'invited'
		  AND organization_id IN (
		    SELECT c.organization_id
		    FROM identity_invitations i
		    JOIN clients c ON c.id = i.client_id
		    WHERE i.supabase_user_id = $2 AND i.status = 'sent'
		  )`), id, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, r.DB.Adapt(`
		UPDATE identity_invitations
		SET status = 'accepted', updated_at = CURRENT_TIMESTAMP
		WHERE supabase_user_id = $1 AND status = 'sent'`), id); err != nil {
		return err
	}
	return tx.Commit()
}
