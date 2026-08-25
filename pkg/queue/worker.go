package queue

import (
	"context"
	"log/slog"
	"sync"
)

// Job represents an asynchronous chat or webhook processing task.
type Job struct {
	ClientID  string
	UserRef   string
	SessionID string
	Message   string
	Image     string
	Channel   string
	Process   func(ctx context.Context, clientID, userRef, sessionID, message, image, channel string) string
}

// WorkerPool manages a pool of concurrent goroutine workers processing chat jobs.
type WorkerPool struct {
	jobs    chan Job
	workers int
	wg      sync.WaitGroup
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewWorkerPool creates a new WorkerPool with the specified worker count and buffer size.
func NewWorkerPool(workers, bufferSize int) *WorkerPool {
	if workers <= 0 {
		workers = 5
	}
	if bufferSize <= 0 {
		bufferSize = 200
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &WorkerPool{
		jobs:    make(chan Job, bufferSize),
		workers: workers,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start launches worker goroutines.
func (wp *WorkerPool) Start() {
	for i := 0; i < wp.workers; i++ {
		wp.wg.Add(1)
		go func(workerID int) {
			defer wp.wg.Done()
			for {
				select {
				case <-wp.ctx.Done():
					return
				case job, ok := <-wp.jobs:
					if !ok {
						return
					}
					wp.processJob(workerID, job)
				}
			}
		}(i + 1)
	}
	slog.Info("async chat worker pool started", "workers", wp.workers)
}

func (wp *WorkerPool) processJob(workerID int, job Job) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("worker recovered from panic", "worker_id", workerID, "panic", r, "client_id", job.ClientID)
		}
	}()

	if job.Process != nil {
		job.Process(wp.ctx, job.ClientID, job.UserRef, job.SessionID, job.Message, job.Image, job.Channel)
	}
}

// Submit adds a job to the queue without blocking.
// Returns true if the job was enqueued, false if queue is saturated.
func (wp *WorkerPool) Submit(job Job) bool {
	select {
	case wp.jobs <- job:
		return true
	default:
		slog.Warn("worker pool queue full, falling back to background goroutine", "client_id", job.ClientID)
		// Fallback so messages are never dropped
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("fallback goroutine recovered from panic", "panic", r)
				}
			}()
			if job.Process != nil {
				job.Process(context.Background(), job.ClientID, job.UserRef, job.SessionID, job.Message, job.Image, job.Channel)
			}
		}()
		return false
	}
}

// Stop gracefully shuts down the worker pool.
func (wp *WorkerPool) Stop() {
	wp.cancel()
	close(wp.jobs)
	wp.wg.Wait()
	slog.Info("async chat worker pool stopped cleanly")
}
