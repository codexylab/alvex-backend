package handlers

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/codexylab/alvex-backend/pkg/middleware"
	"github.com/codexylab/alvex-backend/pkg/services"
	"github.com/codexylab/alvex-backend/pkg/response"
)

// ScrapeHandler handles website scraping requests.
type ScrapeHandler struct {
	ClientSvc *services.ClientService
	PortalSvc *services.PortalService
}

type scrapeResponse struct {
	SyncedAt      string `json:"synced_at"`
	ContentLength int    `json:"content_length"`
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

	scrapedText, syncedAt, err := h.ClientSvc.ScrapeAndSave(r.Context(), id, client.Domain, h.PortalSvc)
	if err != nil {
		response.BadRequest(w, "Failed to scrape website: "+err.Error())
		return
	}

	response.Success(w, scrapeResponse{
		SyncedAt:      syncedAt.Format(time.RFC3339),
		ContentLength: len(scrapedText),
	})
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

	scrapedText, syncedAt, err := h.ClientSvc.ScrapeAndSave(r.Context(), clientID, client.Domain, h.PortalSvc)
	if err != nil {
		response.BadRequest(w, "Failed to scrape website: "+err.Error())
		return
	}

	response.Success(w, scrapeResponse{
		SyncedAt:      syncedAt.Format(time.RFC3339),
		ContentLength: len(scrapedText),
	})
}
