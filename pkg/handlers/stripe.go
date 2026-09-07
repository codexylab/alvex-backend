package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/codexylab/alvex-backend/pkg/response"
	"github.com/codexylab/alvex-backend/pkg/services"
)

// StripeHandler handles incoming Stripe webhooks.
type StripeHandler struct {
	Service       *services.StripeService
	WebhookSecret string
	Now           func() time.Time
}

// NewStripeHandler creates a new StripeHandler instance.
func NewStripeHandler(service *services.StripeService, secret string) *StripeHandler {
	return &StripeHandler{
		Service:       service,
		WebhookSecret: secret,
		Now:           time.Now,
	}
}

// HandleWebhook processes inbound webhooks from Stripe.
//
// POST /webhooks/stripe
func (h *StripeHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	if h.WebhookSecret == "" {
		slog.Error("stripe webhook secret is not configured")
		response.ServiceUnavailable(w, "Stripe webhook is not configured")
		return
	}

	body, ok := readRequestBody(w, r, signedWebhookBodyLimit)
	if !ok {
		return
	}
	if !verifyStripeSignature(r.Header.Get("Stripe-Signature"), body, h.WebhookSecret, h.now(), 5*time.Minute) {
		slog.Warn("stripe webhook: invalid signature")
		response.AuthenticationFailed(w, "Invalid Stripe webhook signature")
		return
	}

	if err := h.Service.AcceptWebhook(r.Context(), body); err != nil {
		slog.Error("stripe webhook acceptance error", "error", err)
		if errors.Is(err, services.ErrInvalidStripeWebhook) {
			response.BadRequest(w, "Invalid webhook payload")
			return
		}
		response.InternalError(w)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"received":true}`))
}

func (h *StripeHandler) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

// verifyStripeSignature implements Stripe's signed-payload verification with
// replay protection. Multiple v1 values are accepted to support key rotation.
func verifyStripeSignature(header string, payload []byte, secret string, now time.Time, tolerance time.Duration) bool {
	var timestamp string
	var signatures []string
	for _, part := range strings.Split(header, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch key {
		case "t":
			timestamp = value
		case "v1":
			signatures = append(signatures, value)
		}
	}

	unixTime, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || len(signatures) == 0 {
		return false
	}
	eventTime := time.Unix(unixTime, 0)
	age := now.Sub(eventTime)
	if age < -tolerance || age > tolerance {
		return false
	}

	signedPayload := fmt.Sprintf("%s.%s", timestamp, payload)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(signedPayload))
	expected := mac.Sum(nil)
	for _, signature := range signatures {
		decoded, err := hex.DecodeString(signature)
		if err == nil && hmac.Equal(expected, decoded) {
			return true
		}
	}
	return false
}
