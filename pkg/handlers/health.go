package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

const readinessTimeout = 2 * time.Second

// DatabaseHealthChecker defines the database capability required by readiness checks.
type DatabaseHealthChecker interface {
	PingContext(ctx context.Context) error
}

// HealthHandler exposes process liveness and dependency readiness independently.
type HealthHandler struct {
	database  DatabaseHealthChecker
	startedAt time.Time
}

// NewHealthHandler creates health endpoints with a stable process start time.
func NewHealthHandler(database DatabaseHealthChecker, startedAt time.Time) *HealthHandler {
	return &HealthHandler{database: database, startedAt: startedAt}
}

// Live reports whether the HTTP process is running.
func (h *HealthHandler) Live(w http.ResponseWriter, _ *http.Request) {
	writeHealthResponse(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"service": "alvex-backend",
		"uptime":  time.Since(h.startedAt).Round(time.Second).String(),
	})
}

// Ready reports whether required dependencies can serve application traffic.
func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
	defer cancel()

	if err := h.database.PingContext(ctx); err != nil {
		writeHealthResponse(w, http.StatusServiceUnavailable, map[string]any{
			"status":  "unavailable",
			"service": "alvex-backend",
		})
		return
	}

	writeHealthResponse(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"service": "alvex-backend",
	})
}

func writeHealthResponse(w http.ResponseWriter, status int, payload map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
