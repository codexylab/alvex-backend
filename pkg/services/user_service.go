package services

import (
	"context"
	"database/sql"
	"errors"

	"github.com/codexylab/alvex-backend/pkg/apierr"
	"github.com/codexylab/alvex-backend/pkg/repository"
)

// UserService manages the business logic for users.
type UserService struct {
	Repo repository.UserRepository
}

// NewUserService creates a new UserService instance.
func NewUserService(repo repository.UserRepository) *UserService {
	return &UserService{Repo: repo}
}

// GetByID retrieves a user profile by ID. Maps db errors to sentinels.
func (s *UserService) GetByID(ctx context.Context, id string) (*repository.User, error) {
	u, err := s.Repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &apierr.NotFoundError{Resource: "User"}
		}
		return nil, err
	}
	return u, nil
}

// Upsert inserts or updates a user record from a verified Supabase identity.
func (s *UserService) Upsert(ctx context.Context, id, name, email string) error {
	return s.Repo.Upsert(ctx, id, name, email)
}
