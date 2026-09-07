package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/codexylab/alvex-backend/pkg/database"
)

var ErrBackgroundJobPayloadMismatch = errors.New("background job payload does not match the original delivery")

type BackgroundJob struct {
	ID             string
	JobType        string
	IdempotencyKey string
	Payload        json.RawMessage
	Attempts       int
	MaxAttempts    int
}

type BackgroundJobRepository interface {
	Enqueue(ctx context.Context, jobType, idempotencyKey string, payload []byte, maxAttempts int) (bool, error)
	ClaimNext(ctx context.Context, jobType string, now time.Time) (*BackgroundJob, error)
	UpdatePayload(ctx context.Context, jobID string, payload []byte) error
	Complete(ctx context.Context, jobID string, completedAt time.Time) error
	Fail(ctx context.Context, job BackgroundJob, reason string, retryAt time.Time) error
}

type SQLBackgroundJobRepository struct {
	DB *database.DB
}

func NewSQLBackgroundJobRepository(db *database.DB) *SQLBackgroundJobRepository {
	return &SQLBackgroundJobRepository{DB: db}
}

func (r *SQLBackgroundJobRepository) Enqueue(
	ctx context.Context,
	jobType string,
	idempotencyKey string,
	payload []byte,
	maxAttempts int,
) (bool, error) {
	if !json.Valid(payload) {
		return false, fmt.Errorf("background job payload must be valid JSON")
	}
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	now := time.Now().UTC()
	payloadDigest := sha256.Sum256(payload)
	payloadHash := hex.EncodeToString(payloadDigest[:])

	result, err := r.DB.ExecContext(ctx, r.DB.Adapt(`
		INSERT INTO background_jobs
		  (id, job_type, idempotency_key, payload, payload_sha256, status, attempts, max_attempts, available_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, 'queued', 0, $6, $7, $8, $9)
		ON CONFLICT (job_type, idempotency_key) DO NOTHING`),
		uuid.NewString(), jobType, idempotencyKey, string(payload), payloadHash, maxAttempts,
		now, now, now,
	)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected > 0 {
		return true, nil
	}

	var storedPayloadHash string
	err = r.DB.QueryRowContext(ctx, r.DB.Adapt(`
		SELECT payload_sha256 FROM background_jobs
		WHERE job_type = $1 AND idempotency_key = $2`), jobType, idempotencyKey).Scan(&storedPayloadHash)
	if err != nil {
		return false, err
	}
	if storedPayloadHash != payloadHash {
		return false, ErrBackgroundJobPayloadMismatch
	}
	return false, nil
}

func (r *SQLBackgroundJobRepository) ClaimNext(
	ctx context.Context,
	jobType string,
	now time.Time,
) (*BackgroundJob, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	staleBefore := now.Add(-10 * time.Minute)
	if _, err := tx.ExecContext(ctx, r.DB.Adapt(`
		UPDATE background_jobs
		SET status = 'dead', locked_at = NULL,
		    last_error = COALESCE(last_error, 'worker lease expired after final attempt'), updated_at = $1
		WHERE job_type = $2 AND status = 'processing' AND locked_at < $3
		  AND attempts >= max_attempts`), now, jobType, staleBefore); err != nil {
		return nil, err
	}

	query := `
		SELECT id, job_type, idempotency_key, payload, attempts, max_attempts
		FROM background_jobs
		WHERE job_type = $1
		  AND attempts < max_attempts
		  AND ((status IN ('queued', 'retry') AND available_at <= $2)
		       OR (status = 'processing' AND locked_at < $3))
		ORDER BY available_at ASC, created_at ASC
		LIMIT 1`
	if !r.DB.IsSQLite() {
		query += " FOR UPDATE SKIP LOCKED"
	}

	var job BackgroundJob
	var payload string
	err = tx.QueryRowContext(ctx, r.DB.Adapt(query), jobType, now, staleBefore).Scan(
		&job.ID,
		&job.JobType,
		&job.IdempotencyKey,
		&payload,
		&job.Attempts,
		&job.MaxAttempts,
	)
	if err != nil {
		return nil, err
	}
	job.Payload = json.RawMessage(payload)

	result, err := tx.ExecContext(ctx, r.DB.Adapt(`
		UPDATE background_jobs
		SET status = 'processing', attempts = attempts + 1, locked_at = $1,
		    last_error = NULL, updated_at = $2
		WHERE id = $3 AND attempts = $4`), now, now, job.ID, job.Attempts)
	if err != nil {
		return nil, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected != 1 {
		return nil, sql.ErrNoRows
	}
	job.Attempts++
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &job, nil
}

func (r *SQLBackgroundJobRepository) UpdatePayload(ctx context.Context, jobID string, payload []byte) error {
	if !json.Valid(payload) {
		return fmt.Errorf("background job payload must be valid JSON")
	}
	_, err := r.DB.ExecContext(ctx, r.DB.Adapt(`
		UPDATE background_jobs SET payload = $1, updated_at = $2
		WHERE id = $3 AND status = 'processing'`), string(payload), time.Now().UTC(), jobID)
	return err
}

func (r *SQLBackgroundJobRepository) Complete(ctx context.Context, jobID string, completedAt time.Time) error {
	_, err := r.DB.ExecContext(ctx, r.DB.Adapt(`
		UPDATE background_jobs
		SET status = 'succeeded', completed_at = $1, locked_at = NULL, updated_at = $2
		WHERE id = $3 AND status = 'processing'`), completedAt, completedAt, jobID)
	return err
}

func (r *SQLBackgroundJobRepository) Fail(
	ctx context.Context,
	job BackgroundJob,
	reason string,
	retryAt time.Time,
) error {
	status := "retry"
	if job.Attempts >= job.MaxAttempts {
		status = "dead"
	}
	_, err := r.DB.ExecContext(ctx, r.DB.Adapt(`
		UPDATE background_jobs
		SET status = $1, available_at = $2, locked_at = NULL, last_error = $3, updated_at = $4
		WHERE id = $5 AND status = 'processing'`), status, retryAt, truncateJobError(reason), time.Now().UTC(), job.ID)
	return err
}

func truncateJobError(reason string) string {
	const maxLength = 2_000
	if len(reason) <= maxLength {
		return reason
	}
	return reason[:maxLength]
}
