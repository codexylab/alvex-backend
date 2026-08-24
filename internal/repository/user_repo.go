package repository

import (
	"context"

	"github.com/codexylab/alvex-backend/internal/database"
)

// User represents a user record in the DB.
type User struct {
	ID    string
	Name  string
	Email string
	Role  string
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
		SELECT id, name, email, role FROM users WHERE id = $1
	`), id).Scan(&u.ID, &u.Name, &u.Email, &u.Role)
	if err != nil {
		return nil, err
	}
	return u, nil
}

// Upsert inserts a new user or updates their name/email if they already exist.
// Called from the Clerk webhook handler on user.created and user.updated events.
// Compatible with both SQLite (development) and PostgreSQL (Supabase production).
func (r *SQLUserRepository) Upsert(ctx context.Context, id, name, email string) error {
	_, err := r.DB.ExecContext(ctx, r.DB.Adapt(`
		INSERT INTO users (id, name, email, role, created_at, updated_at)
		VALUES ($1, $2, $3, 'admin', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (id) DO UPDATE
		  SET name       = EXCLUDED.name,
		      email      = EXCLUDED.email,
		      updated_at = CURRENT_TIMESTAMP
	`), id, name, email)
	return err
}

