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

type CheckoutHandler struct {
	Service *services.SelfServiceCheckoutService
}

// Create creates a server-controlled Stripe subscription Checkout session.
// POST /api/v1/self-service/checkout
func (h *CheckoutHandler) Create(w http.ResponseWriter, r *http.Request) {
	body, ok := readRequestBody(w, r, 64<<10)
	if !ok {
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var request services.SelfServiceCheckoutRequest
	if err := decoder.Decode(&request); err != nil {
		response.BadRequest(w, "Invalid request body")
		return
	}

	identity, ok := middleware.GetAuthenticatedUser(r)
	if !ok || identity.ApplicationUserID == "" {
		response.Unauthorized(w)
		return
	}
	session, err := h.Service.Create(
		r.Context(),
		request,
		identity.ApplicationUserID,
		identity.Email,
		identity.Name,
	)
	if errors.Is(err, apierr.ErrValidation) {
		response.BadRequest(w, err.Error())
		return
	}
	if errors.Is(err, services.ErrStripeCheckoutNotConfigured) {
		response.ServiceUnavailable(w, "Stripe Checkout is not configured for this plan")
		return
	}
	if err != nil {
		slog.Error("self-service checkout creation failed", "error", err)
		response.InternalError(w)
		return
	}
	response.Created(w, session)
}
