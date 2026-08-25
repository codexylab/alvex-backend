package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/codexylab/alvex-backend/pkg/middleware"
	"github.com/codexylab/alvex-backend/pkg/services"
	"github.com/codexylab/alvex-backend/pkg/response"
)

// OnboardingHandler manages 1-click automated onboarding workflows.
type OnboardingHandler struct {
	Service *services.OnboardingService
}

// NewOnboardingHandler creates a new OnboardingHandler instance.
func NewOnboardingHandler(service *services.OnboardingService) *OnboardingHandler {
	return &OnboardingHandler{Service: service}
}

// Start launches automated client onboarding.
//
// POST /api/v1/onboarding/start
func (h *OnboardingHandler) Start(w http.ResponseWriter, r *http.Request) {
	var req services.OnboardingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "Invalid request body")
		return
	}

	if req.Name == "" {
		response.BadRequest(w, "Client name is required")
		return
	}

	ownerID := middleware.GetUserID(r)
	result, err := h.Service.StartOnboarding(r.Context(), req, ownerID)
	if err != nil {
		response.BadRequest(w, err.Error())
		return
	}

	response.Created(w, result)
}

// Status checks the onboarding status of a client.
//
// GET /api/v1/onboarding/:id/status
func (h *OnboardingHandler) Status(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	client, err := h.Service.ClientSvc.GetByID(r.Context(), id)
	if err != nil {
		response.NotFound(w, "Client")
		return
	}

	response.Success(w, map[string]interface{}{
		"client_id":         client.ID,
		"onboarding_status": client.OnboardingStatus,
		"scrape_synced_at":  client.ScrapeSyncedAt,
	})
}
