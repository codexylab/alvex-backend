package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/codexylab/alvex-backend/pkg/apierr"
	"github.com/codexylab/alvex-backend/pkg/middleware"
	"github.com/codexylab/alvex-backend/pkg/response"
	"github.com/codexylab/alvex-backend/pkg/services"
)

type PlatformClientHandler struct {
	Service *services.ManualOnboardingService
}

// Create invites a direct customer and atomically provisions its first client.
// POST /api/v1/platform/clients
func (h *PlatformClientHandler) Create(w http.ResponseWriter, r *http.Request) {
	body, ok := readRequestBody(w, r, 64<<10)
	if !ok {
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()

	var request services.ManualClientRequest
	if err := decoder.Decode(&request); err != nil {
		response.BadRequest(w, "Invalid request body")
		return
	}

	result, err := h.Service.InviteAndProvision(r.Context(), request, middleware.GetUserID(r))
	if errors.Is(err, apierr.ErrValidation) {
		response.BadRequest(w, err.Error())
		return
	}
	if errors.Is(err, services.ErrIdentityInvitationNotConfigured) {
		response.ServiceUnavailable(w, "Client invitations are not configured")
		return
	}
	if err != nil {
		slog.Error("manual client provisioning failed", "error", err)
		response.InternalError(w)
		return
	}
	response.Created(w, result)
}
