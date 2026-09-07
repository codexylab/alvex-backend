package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/codexylab/alvex-backend/pkg/middleware"
	"github.com/codexylab/alvex-backend/pkg/response"
	"github.com/codexylab/alvex-backend/pkg/services"
)

// ScrapeHandler handles website scraping requests.
type ScrapeHandler struct {
	ClientSvc   *services.ClientService
	WebsiteSync *services.WebsiteIndexScheduler
}

type scrapeResponse struct {
	JobID  string `json:"job_id"`
	Status string `json:"status"`
}

// ScrapeAdmin handles manual scraping triggered by the Admin settings page.
//
// POST /api/v1/clients/{id}/scrape
func (h *ScrapeHandler) ScrapeAdmin(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	client, err := h.ClientSvc.GetByID(r.Context(), id)
	if err != nil {
		response.NotFound(w, "Client")
		return
	}

	if client.Domain == "" {
		response.BadRequest(w, "No website domain set for this client")
		return
	}

	jobID, err := h.WebsiteSync.EnqueueManual(r.Context(), id, client.Domain)
	if err != nil {
		response.ServiceUnavailable(w, "Failed to queue website synchronization")
		return
	}

	response.JSON(w, http.StatusAccepted, response.APIResponse{Success: true, Data: scrapeResponse{
		JobID:  jobID,
		Status: "queued",
	}})
}

// ScrapePortal handles client-triggered manual scraping from the Client Portal.
//
// POST /api/v1/client-portal/sync-knowledge
func (h *ScrapeHandler) ScrapePortal(w http.ResponseWriter, r *http.Request) {
	clientID := middleware.GetPortalClientID(r)
	if clientID == "" {
		response.Unauthorized(w)
		return
	}

	client, err := h.ClientSvc.GetByID(r.Context(), clientID)
	if err != nil {
		response.NotFound(w, "Client")
		return
	}

	if client.Domain == "" {
		response.BadRequest(w, "No website domain set for this client")
		return
	}

	jobID, err := h.WebsiteSync.EnqueueManual(r.Context(), clientID, client.Domain)
	if err != nil {
		response.ServiceUnavailable(w, "Failed to queue website synchronization")
		return
	}

	response.JSON(w, http.StatusAccepted, response.APIResponse{Success: true, Data: scrapeResponse{
		JobID:  jobID,
		Status: "queued",
	}})
}
