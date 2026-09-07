package queue

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/codexylab/alvex-backend/pkg/repository"
)

type DurableJobProcessor func(ctx context.Context, job repository.BackgroundJob) error
type DeadJobAlert func(ctx context.Context, job repository.BackgroundJob, cause error) error

// DurableWorker polls database-backed jobs. Claimed jobs survive restarts and
// failures are retried with bounded exponential backoff before becoming dead.
type DurableWorker struct {
	repository repository.BackgroundJobRepository
	jobType    string
	processor  DurableJobProcessor
	workers    int
	pollEvery  time.Duration
	deadAlert  DeadJobAlert
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

// WithDeadJobAlert registers a best-effort notification for terminal failures.
func (w *DurableWorker) WithDeadJobAlert(alert DeadJobAlert) *DurableWorker {
	w.deadAlert = alert
	return w
}

func NewDurableWorker(
	repository repository.BackgroundJobRepository,
	jobType string,
	workers int,
	processor DurableJobProcessor,
) *DurableWorker {
	if workers <= 0 {
		workers = 1
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &DurableWorker{
		repository: repository,
		jobType:    jobType,
		processor:  processor,
		workers:    workers,
		pollEvery:  time.Second,
		ctx:        ctx,
		cancel:     cancel,
	}
}

func (w *DurableWorker) Start() {
	for workerID := 1; workerID <= w.workers; workerID++ {
		w.wg.Add(1)
		go w.run(workerID)
	}
	slog.Info("durable worker started", "job_type", w.jobType, "workers", w.workers)
}

func (w *DurableWorker) Stop() {
	w.cancel()
	w.wg.Wait()
	slog.Info("durable worker stopped", "job_type", w.jobType)
}

func (w *DurableWorker) run(workerID int) {
	defer w.wg.Done()
	for {
		if err := w.ctx.Err(); err != nil {
			return
		}

		job, err := w.repository.ClaimNext(w.ctx, w.jobType, time.Now().UTC())
		if errors.Is(err, sql.ErrNoRows) {
			if !waitForContext(w.ctx, w.pollEvery) {
				return
			}
			continue
		}
		if err != nil {
			slog.Error("durable worker claim failed", "job_type", w.jobType, "worker_id", workerID, "error", err)
			if !waitForContext(w.ctx, w.pollEvery) {
				return
			}
			continue
		}

		if err := w.process(job); err != nil {
			retryAt := time.Now().UTC().Add(jobRetryDelay(job.Attempts))
			if failErr := w.repository.Fail(w.ctx, *job, err.Error(), retryAt); failErr != nil {
				slog.Error("durable worker could not record failure", "job_id", job.ID, "error", failErr)
			}
			if job.Attempts >= job.MaxAttempts {
				slog.Error("durable job moved to dead-letter state", "job_id", job.ID, "job_type", job.JobType, "error", err)
				if w.deadAlert != nil {
					if alertErr := w.deadAlert(w.ctx, *job, err); alertErr != nil {
						slog.Error("dead-letter alert delivery failed", "job_id", job.ID, "job_type", job.JobType, "error", alertErr)
					}
				}
			}
			continue
		}
		if err := w.repository.Complete(w.ctx, job.ID, time.Now().UTC()); err != nil {
			slog.Error("durable worker could not complete job", "job_id", job.ID, "error", err)
		}
	}
}

func (w *DurableWorker) process(job *repository.BackgroundJob) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Error("durable worker recovered from panic", "job_id", job.ID, "panic", recovered)
			err = errors.New("job processor panicked")
		}
	}()
	if w.processor == nil {
		return errors.New("job processor is not configured")
	}
	return w.processor(w.ctx, *job)
}

func jobRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := time.Second * time.Duration(1<<min(attempt-1, 8))
	if delay > 5*time.Minute {
		return 5 * time.Minute
	}
	return delay
}

func waitForContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
