package repository

import (
	"context"
	"testing"
	"time"
)

func TestBackgroundJobOperationsListsMetadataAndRetriesDeadJob(t *testing.T) {
	db := newTenantTestDB(t)
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	if _, err := db.Exec(`
		INSERT INTO background_jobs
		  (id, job_type, idempotency_key, payload, payload_sha256, status, attempts,
		   max_attempts, available_at, last_error, created_at, updated_at)
		VALUES
		  ('job_dead', 'knowledge.document_index', 'document_one', '{"secret":"hidden"}',
		   'hash', 'dead', 6, 6, ?, 'provider offline', ?, ?)
	`, now, now, now); err != nil {
		t.Fatalf("seed dead job: %v", err)
	}
	repo := NewSQLBackgroundJobOperationsRepository(db)

	jobs, err := repo.ListJobs(context.Background(), "dead", "knowledge.document_index", 10)
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ID != "job_dead" || jobs[0].LastError != "provider offline" {
		t.Fatalf("unexpected job summaries: %#v", jobs)
	}
	if err := repo.RetryDeadJob(context.Background(), "job_dead", now.Add(time.Minute)); err != nil {
		t.Fatalf("retry dead job: %v", err)
	}
	var status string
	var attempts int
	if err := db.QueryRow(`SELECT status, attempts FROM background_jobs WHERE id = 'job_dead'`).Scan(&status, &attempts); err != nil {
		t.Fatalf("load retried job: %v", err)
	}
	if status != "queued" || attempts != 0 {
		t.Fatalf("unexpected retried state: status=%q attempts=%d", status, attempts)
	}
}

func TestBackgroundJobOperationsCountsQueueDepth(t *testing.T) {
	db := newTenantTestDB(t)
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	if _, err := db.Exec(`
		INSERT INTO background_jobs
		  (id, job_type, idempotency_key, payload, payload_sha256, status, attempts,
		   max_attempts, available_at, created_at, updated_at)
		VALUES
		  ('job_one', 'knowledge.document_index', 'one', '{}', 'hash-one', 'queued', 0, 6, ?, ?, ?),
		  ('job_two', 'knowledge.document_index', 'two', '{}', 'hash-two', 'queued', 0, 6, ?, ?, ?),
		  ('job_three', 'website.index', 'three', '{}', 'hash-three', 'dead', 6, 6, ?, ?, ?)
	`, now, now, now, now, now, now, now, now, now); err != nil {
		t.Fatalf("seed jobs: %v", err)
	}

	counts, err := NewSQLBackgroundJobOperationsRepository(db).GetJobCounts(context.Background())
	if err != nil {
		t.Fatalf("count queue depth: %v", err)
	}
	if len(counts) != 2 || counts[0].Count != 2 || counts[1].Count != 1 {
		t.Fatalf("unexpected queue counts: %#v", counts)
	}
}
