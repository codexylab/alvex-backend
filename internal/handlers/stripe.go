package handlers

import (
	"io"
	"log/slog"
	"net/http"

	"github.com/codexylab/alvex-backend/internal/services"
	"github.com/codexylab/alvex-backend/pkg/response"
)

// StripeHandler handles incoming Stripe webhooks.
type StripeHandler struct {
	Service       *services.StripeService
	WebhookSecret string
}

// NewStripeHandler creates a new StripeHandler instance.
func NewStripeHandler(service *services.StripeService, secret string) *StripeHandler {
	return &StripeHandler{
		Service:       service,
		WebhookSecret: secret,
	}
}

// HandleWebhook processes inbound webhooks from Stripe.
//
// POST /webhooks/stripe
func (h *StripeHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := h.Service.HandleWebhook(r.Context(), body); err != nil {
		slog.Error("stripe webhook processing error", "error", err)
		response.BadRequest(w, "Webhook processing error")
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"received":true}`))
}
