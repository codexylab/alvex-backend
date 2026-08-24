package handlers

import (
	"encoding/csv"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/codexylab/alvex-backend/internal/services"
	"github.com/codexylab/alvex-backend/pkg/response"
)

// AnalyticsHandler handles all /api/v1/analytics HTTP endpoints.
type AnalyticsHandler struct {
	Service *services.AnalyticsService
}

// Overview returns aggregated dashboard statistics.
//
// GET /api/v1/analytics/overview
func (h *AnalyticsHandler) Overview(w http.ResponseWriter, r *http.Request) {
	stats, err := h.Service.GetOverview(r.Context())
	if err != nil {
		log.Printf("[ERROR] analytics Overview failed: %v", err)
		response.InternalError(w)
		return
	}
	response.Success(w, stats)
}

// Trends returns conversation volume data for the chart.
//
// GET /api/v1/analytics/trends?period=7d   (options: 7d, 30d)
func (h *AnalyticsHandler) Trends(w http.ResponseWriter, r *http.Request) {
	period := r.URL.Query().Get("period")
	clientID := r.URL.Query().Get("client_id")

	points, err := h.Service.GetTrends(r.Context(), period, clientID)
	if err != nil {
		log.Printf("[ERROR] analytics Trends failed: %v", err)
		response.InternalError(w)
		return
	}
	response.Success(w, points)
}

// Activity returns the recent activity log feed.
//
// GET /api/v1/analytics/activity?limit=20
func (h *AnalyticsHandler) Activity(w http.ResponseWriter, r *http.Request) {
	logs, err := h.Service.GetRecentActivity(r.Context())
	if err != nil {
		log.Printf("[ERROR] analytics Activity failed: %v", err)
		response.InternalError(w)
		return
	}
	response.Success(w, logs)
}

// TopQuestions returns frequently asked questions.
//
// GET /api/v1/analytics/top-questions?client_id=...&limit=10
func (h *AnalyticsHandler) TopQuestions(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	questions, err := h.Service.GetTopQuestions(r.Context(), clientID, 10)
	if err != nil {
		response.InternalError(w)
		return
	}
	response.Success(w, questions)
}

// FailedQueries returns unanswered or failed customer queries.
//
// GET /api/v1/analytics/failed-queries?client_id=...&limit=20
func (h *AnalyticsHandler) FailedQueries(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	failed, err := h.Service.GetFailedQueries(r.Context(), clientID, 20)
	if err != nil {
		response.InternalError(w)
		return
	}
	response.Success(w, failed)
}

// Satisfaction returns positive vs negative feedback analytics.
//
// GET /api/v1/analytics/satisfaction?client_id=...
func (h *AnalyticsHandler) Satisfaction(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	stats, err := h.Service.GetSatisfaction(r.Context(), clientID)
	if err != nil {
		response.InternalError(w)
		return
	}
	response.Success(w, stats)
}

// ExportCSV exports data as a downloadable CSV file.
//
// GET /api/v1/analytics/export?type=clients|invoices|activity
func (h *AnalyticsHandler) ExportCSV(w http.ResponseWriter, r *http.Request) {
	exportType := r.URL.Query().Get("type")
	if exportType == "" {
		exportType = "activity"
	}

	filename := fmt.Sprintf("alvex_%s_%s.csv", exportType, time.Now().Format("2006-01-02"))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	cw := csv.NewWriter(w)
	defer cw.Flush()

	rows, err := h.Service.QueryExportRows(r.Context(), exportType)
	if err != nil {
		log.Printf("[ERROR] analytics ExportCSV failed to fetch rows: %v", err)
		return
	}
	defer rows.Close()

	switch exportType {
	case "clients":
		cw.Write([]string{"ID", "Name", "Domain", "Status", "Provider", "Model", "Billing Plan", "Created At"})
		for rows.Next() {
			var id, name, domain, status, provider, model, plan string
			var createdAt time.Time
			if err := rows.Scan(&id, &name, &domain, &status, &provider, &model, &plan, &createdAt); err == nil {
				cw.Write([]string{id, name, domain, status, provider, model, plan, createdAt.Format("2006-01-02")})
			}
		}

	case "invoices":
		cw.Write([]string{"ID", "Client", "Amount ($)", "Status", "Due Date", "Created At"})
		for rows.Next() {
			var id, client, status, dueDate string
			var amount float64
			var createdAt time.Time
			if err := rows.Scan(&id, &client, &amount, &status, &dueDate, &createdAt); err == nil {
				cw.Write([]string{id, client, fmt.Sprintf("%.2f", amount), status, dueDate, createdAt.Format("2006-01-02")})
			}
		}

	default: // "activity"
		cw.Write([]string{"ID", "Client", "Channel", "User", "Message", "Status", "Latency (ms)", "Created At"})
		for rows.Next() {
			var id, client, channel, user, message, status string
			var latency int64
			var createdAt time.Time
			if err := rows.Scan(&id, &client, &channel, &user, &message, &status, &latency, &createdAt); err == nil {
				cw.Write([]string{id, client, channel, user, message, status,
					fmt.Sprintf("%d", latency), createdAt.Format(time.RFC3339)})
			}
		}
	}
}
