package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/codexylab/alvex-backend/pkg/apierr"
	"github.com/codexylab/alvex-backend/pkg/models"
	"github.com/codexylab/alvex-backend/pkg/repository"
)

// HandoffService contains human-support workflow rules.
type HandoffService struct {
	Repo repository.HandoffRepository
	Now  func() time.Time
}

func NewHandoffService(repo repository.HandoffRepository) *HandoffService {
	return &HandoffService{Repo: repo, Now: time.Now}
}

func (s *HandoffService) ListNeedsAttention(ctx context.Context, limit int) ([]models.ActivityLog, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	return s.Repo.ListNeedsAttention(ctx, limit)
}

func (s *HandoffService) Reply(ctx context.Context, id, reply string) (time.Time, error) {
	reply = strings.TrimSpace(reply)
	if reply == "" {
		return time.Time{}, &apierr.ValidationError{Message: "reply text is required"}
	}
	now := s.Now().UTC()
	rows, err := s.Repo.Reply(ctx, id, reply, now)
	if err != nil {
		return time.Time{}, err
	}
	if rows == 0 {
		return time.Time{}, fmt.Errorf("%w: conversation not found", apierr.ErrNotFound)
	}
	return now, nil
}

func (s *HandoffService) Resolve(ctx context.Context, id string) error {
	rows, err := s.Repo.Resolve(ctx, id)
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("%w: conversation not found", apierr.ErrNotFound)
	}
	return nil
}
