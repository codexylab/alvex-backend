package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/codexylab/alvex-backend/pkg/database"
)

type BackgroundJobSummary struct {
	ID             string     `json:"id"`
	JobType        string     `json:"job_type"`
	IdempotencyKey string     `json:"idempotency_key"`
	Status         string     `json:"status"`
	Attempts       int        `json:"attempts"`
	MaxAttempts    int        `json:"max_attempts"`
	AvailableAt    time.Time  `json:"available_at"`
	LockedAt       *time.Time `json:"locked_at,omitempty"`
	LastError      string     `json:"last_error,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type BackgroundJobOperationsRepository interface {
	ListJobs(ctx context.Context, status, jobType string, limit int) ([]BackgroundJobSummary, error)
	GetJobCounts(ctx context.Context) ([]BackgroundJobCount, error)
	RetryDeadJob(ctx context.Context, id string, now time.Time) error
}

type BackgroundJobCount struct {
	JobType string `json:"job_type"`
	Status  string `json:"status"`
	Count   int64  `json:"count"`
}

type SQLBackgroundJobOperationsRepository struct {
	DB *database.DB
}

func NewSQLBackgroundJobOperationsRepository(db *database.DB) *SQLBackgroundJobOperationsRepository {
	return &SQLBackgroundJobOperationsRepository{DB: db}
}

// ListJobs intentionally excludes payloads because provider events and chat
// messages can contain customer data or billing metadata.
func (r *SQLBackgroundJobOperationsRepository) ListJobs(
	ctx context.Context,
	status string,
	jobType string,
	limit int,
) ([]BackgroundJobSummary, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	conditions := make([]string, 0, 2)
	args := make([]interface{}, 0, 3)
	if status != "" {
		args = append(args, status)
		conditions = append(conditions, fmt.Sprintf("status = $%d", len(args)))
	}
	if jobType != "" {
		args = append(args, jobType)
		conditions = append(conditions, fmt.Sprintf("job_type = $%d", len(args)))
	}
	query := `
		SELECT id, job_type, idempotency_key, status, attempts, max_attempts,
		       available_at, locked_at, COALESCE(last_error, ''), completed_at,
		       created_at, updated_at
		FROM background_jobs`
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	args = append(args, limit)
	query += fmt.Sprintf(" ORDER BY updated_at DESC LIMIT $%d", len(args))

	rows, err := r.DB.QueryContext(ctx, r.DB.Adapt(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]BackgroundJobSummary, 0)
	for rows.Next() {
		var item BackgroundJobSummary
		var lockedAt, completedAt sql.NullTime
		if err := rows.Scan(
			&item.ID,
			&item.JobType,
			&item.IdempotencyKey,
			&item.Status,
			&item.Attempts,
			&item.MaxAttempts,
			&item.AvailableAt,
			&lockedAt,
			&item.LastError,
			&completedAt,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if lockedAt.Valid {
			item.LockedAt = &lockedAt.Time
		}
		if completedAt.Valid {
			item.CompletedAt = &completedAt.Time
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *SQLBackgroundJobOperationsRepository) GetJobCounts(ctx context.Context) ([]BackgroundJobCount, error) {
	rows, err := r.DB.QueryContext(ctx, `
		SELECT job_type, status, COUNT(*)
		FROM background_jobs
		GROUP BY job_type, status
		ORDER BY job_type, status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make([]BackgroundJobCount, 0)
	for rows.Next() {
		var count BackgroundJobCount
		if err := rows.Scan(&count.JobType, &count.Status, &count.Count); err != nil {
			return nil, err
		}
		counts = append(counts, count)
	}
	return counts, rows.Err()
}

func (r *SQLBackgroundJobOperationsRepository) RetryDeadJob(
	ctx context.Context,
	id string,
	now time.Time,
) error {
	result, err := r.DB.ExecContext(ctx, r.DB.Adapt(`
		UPDATE background_jobs
		SET status = 'queued', attempts = 0, available_at = $1, locked_at = NULL,
		    last_error = NULL, completed_at = NULL, updated_at = $2
		WHERE id = $3 AND status = 'dead'`), now.UTC(), now.UTC(), id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}
