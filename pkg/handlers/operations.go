package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/codexylab/alvex-backend/pkg/repository"
	"github.com/codexylab/alvex-backend/pkg/response"
)

type OperationsHandler struct {
	Jobs     repository.BackgroundJobOperationsRepository
	AIUsage  repository.AIUsageOperationsRepository
	Database DatabaseStatsProvider
}

type DatabaseStatsProvider interface {
	Stats() sql.DBStats
}

func NewOperationsHandler(
	jobs repository.BackgroundJobOperationsRepository,
	aiUsage repository.AIUsageOperationsRepository,
	database DatabaseStatsProvider,
) *OperationsHandler {
	return &OperationsHandler{Jobs: jobs, AIUsage: aiUsage, Database: database}
}

// ListJobs provides payload-free queue visibility to platform operators.
func (h *OperationsHandler) ListJobs(w http.ResponseWriter, r *http.Request) {
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status == "" {
		status = "dead"
	}
	if !isOperationalJobStatus(status) {
		response.BadRequest(w, "Invalid job status")
		return
	}
	jobType := strings.TrimSpace(r.URL.Query().Get("job_type"))
	if len(jobType) > 100 {
		response.BadRequest(w, "Invalid job type")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	jobs, err := h.Jobs.ListJobs(r.Context(), status, jobType, limit)
	if err != nil {
		response.InternalError(w)
		return
	}
	response.Success(w, jobs)
}

// RetryJob explicitly returns one dead-letter job to the durable queue.
func (h *OperationsHandler) RetryJob(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		response.BadRequest(w, "Job ID is required")
		return
	}
	if err := h.Jobs.RetryDeadJob(r.Context(), id, time.Now().UTC()); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.Conflict(w, "Job is not in dead-letter state")
			return
		}
		response.InternalError(w)
		return
	}
	response.Success(w, map[string]string{"id": id, "status": "queued"})
}

// Snapshot returns payload-free queue depth and database pool metrics.
func (h *OperationsHandler) Snapshot(w http.ResponseWriter, r *http.Request) {
	jobCounts, err := h.Jobs.GetJobCounts(r.Context())
	if err != nil {
		response.InternalError(w)
		return
	}
	capturedAt := time.Now().UTC()
	usage, err := h.AIUsage.Summarize(r.Context(), capturedAt.Add(-24*time.Hour))
	if err != nil {
		response.InternalError(w)
		return
	}
	databaseStats := h.Database.Stats()
	response.Success(w, map[string]interface{}{
		"queue":        jobCounts,
		"ai_usage_24h": usage,
		"database": map[string]interface{}{
			"open_connections":     databaseStats.OpenConnections,
			"in_use":               databaseStats.InUse,
			"idle":                 databaseStats.Idle,
			"wait_count":           databaseStats.WaitCount,
			"wait_duration_ms":     databaseStats.WaitDuration.Milliseconds(),
			"max_open_connections": databaseStats.MaxOpenConnections,
		},
		"captured_at": capturedAt,
	})
}

func isOperationalJobStatus(status string) bool {
	switch status {
	case "queued", "processing", "retry", "succeeded", "dead":
		return true
	default:
		return false
	}
}
