package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/codexylab/alvex-backend/pkg/database"
)

func TestBackgroundJobLifecycleAndIdempotency(t *testing.T) {
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	_, err = sqlDB.Exec(`
		CREATE TABLE background_jobs (
			id TEXT PRIMARY KEY,
			job_type TEXT NOT NULL,
			idempotency_key TEXT NOT NULL,
			payload TEXT NOT NULL,
			payload_sha256 TEXT NOT NULL,
			status TEXT NOT NULL,
			attempts INTEGER NOT NULL,
			max_attempts INTEGER NOT NULL,
			available_at DATETIME NOT NULL,
			locked_at DATETIME,
			last_error TEXT,
			completed_at DATETIME,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			UNIQUE (job_type, idempotency_key)
		);`)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}

	repo := NewSQLBackgroundJobRepository(database.NewDB(sqlDB, "sqlite"))
	created, err := repo.Enqueue(context.Background(), "test.job", "event_one", []byte(`{"value":1}`), 2)
	if err != nil || !created {
		t.Fatalf("enqueue created=%v err=%v", created, err)
	}
	created, err = repo.Enqueue(context.Background(), "test.job", "event_one", []byte(`{"value":1}`), 2)
	if err != nil || created {
		t.Fatalf("duplicate enqueue created=%v err=%v", created, err)
	}
	if _, err := repo.Enqueue(context.Background(), "test.job", "event_one", []byte(`{"value":2}`), 2); !errors.Is(err, ErrBackgroundJobPayloadMismatch) {
		t.Fatalf("expected payload mismatch, got %v", err)
	}

	now := time.Now().UTC().Add(time.Second)
	job, err := repo.ClaimNext(context.Background(), "test.job", now)
	if err != nil || job.Attempts != 1 {
		t.Fatalf("first claim job=%#v err=%v", job, err)
	}
	if err := repo.UpdatePayload(context.Background(), job.ID, []byte(`{"value":1,"result":"saved"}`)); err != nil {
		t.Fatalf("update payload: %v", err)
	}
	retryAt := now.Add(time.Minute)
	if err := repo.Fail(context.Background(), *job, "temporary", retryAt); err != nil {
		t.Fatalf("fail job: %v", err)
	}
	if _, err := repo.ClaimNext(context.Background(), "test.job", now); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected retry delay to be respected, got %v", err)
	}

	job, err = repo.ClaimNext(context.Background(), "test.job", retryAt.Add(time.Second))
	if err != nil || job.Attempts != 2 {
		t.Fatalf("second claim job=%#v err=%v", job, err)
	}
	if err := repo.Complete(context.Background(), job.ID, retryAt.Add(time.Second)); err != nil {
		t.Fatalf("complete job: %v", err)
	}
	if _, err := repo.ClaimNext(context.Background(), "test.job", retryAt.Add(2*time.Second)); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected completed job not to be claimed, got %v", err)
	}
}
