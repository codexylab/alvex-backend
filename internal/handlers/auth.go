package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/codexylab/alvex-backend/internal/middleware"
	"github.com/codexylab/alvex-backend/internal/services"
	"github.com/codexylab/alvex-backend/pkg/apierr"
	"github.com/codexylab/alvex-backend/pkg/response"
)

// AuthHandler handles authentication-related endpoints.
// Delegates database queries to the UserService.
type AuthHandler struct {
	Service *services.UserService
}

// meResponse is the shape returned by GET /api/v1/auth/me.
type meResponse struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Role   string `json:"role"`
}

// Me returns the currently authenticated user's profile.
//
// GET /api/v1/auth/me
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	if userID == "" {
		response.Unauthorized(w)
		return
	}

	// In development mode, return a mock profile
	if userID == "dev-user-001" {
		response.Success(w, meResponse{
			UserID: userID,
			Name:   "Dev Administrator",
			Email:  "dev@alvex.ai",
			Role:   "admin",
		})
		return
	}

	// Fetch from user service
	u, err := h.Service.GetByID(r.Context(), userID)
	if err != nil {
		var notFoundErr *apierr.NotFoundError
		if errors.As(err, &notFoundErr) {
			response.NotFound(w, "User")
			return
		}
		response.InternalError(w)
		return
	}

	response.Success(w, meResponse{
		UserID: u.ID,
		Name:   u.Name,
		Email:  u.Email,
		Role:   u.Role,
	})
}

// -------------------------------------------------------------------
// Clerk Webhook
// -------------------------------------------------------------------

// clerkWebhookPayload represents the Clerk webhook event structure.
type clerkWebhookPayload struct {
	Type string `json:"type"`
	Data struct {
		ID             string `json:"id"`
		EmailAddresses []struct {
			EmailAddress string `json:"email_address"`
		} `json:"email_addresses"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
	} `json:"data"`
}

// ClerkWebhook handles Clerk user lifecycle events (user.created, user.updated).
// Keeps the local users table in sync with Clerk's user registry.
//
// POST /webhooks/clerk
func (h *AuthHandler) ClerkWebhook(w http.ResponseWriter, r *http.Request) {
	webhookSecret := os.Getenv("CLERK_WEBHOOK_SECRET")

	// Read body first (needed for signature verification)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Warn("clerk webhook: failed to read body", "error", err)
		response.BadRequest(w, "Failed to read request body")
		return
	}

	// Verify Svix signature if webhook secret is configured
	if webhookSecret != "" {
		if !verifyClerkSignature(r, body, webhookSecret) {
			slog.Warn("clerk webhook: invalid signature")
			response.Unauthorized(w)
			return
		}
	}

	var payload clerkWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		slog.Warn("clerk webhook: invalid JSON", "error", err)
		response.BadRequest(w, "Invalid JSON body")
		return
	}

	switch payload.Type {
	case "user.created", "user.updated":
		userID := payload.Data.ID
		if userID == "" {
			response.BadRequest(w, "Missing user ID in webhook payload")
			return
		}

		// Build display name from first + last name
		name := strings.TrimSpace(payload.Data.FirstName + " " + payload.Data.LastName)
		if name == "" {
			name = "Alvex User"
		}

		// Extract primary email
		email := ""
		if len(payload.Data.EmailAddresses) > 0 {
			email = payload.Data.EmailAddresses[0].EmailAddress
		}

		// Upsert user into local database
		if err := h.Service.Upsert(r.Context(), userID, name, email); err != nil {
			slog.Error("clerk webhook: failed to upsert user",
				"user_id", userID,
				"event", payload.Type,
				"error", err,
			)
			response.InternalError(w)
			return
		}

		slog.Info("clerk webhook: user synced",
			"user_id", userID,
			"event", payload.Type,
			"email", email,
		)

	default:
		// Acknowledge unhandled events gracefully
		slog.Debug("clerk webhook: unhandled event type", "type", payload.Type)
	}

	w.WriteHeader(http.StatusOK)
}

// verifyClerkSignature verifies the Svix webhook signature from Clerk.
// Clerk uses the Svix signing standard: sha256 HMAC of the raw body.
func verifyClerkSignature(r *http.Request, body []byte, secret string) bool {
	// Svix-Signature header format: "v1,<base64_signature>"
	svixSig := r.Header.Get("Svix-Signature")
	if svixSig == "" {
		return false
	}

	// Extract the raw secret (strip "whsec_" prefix if present)
	rawSecret := strings.TrimPrefix(secret, "whsec_")

	// Svix signs: "<svix-id>.<svix-timestamp>.<body>"
	svixID := r.Header.Get("Svix-Id")
	svixTimestamp := r.Header.Get("Svix-Timestamp")
	if svixID == "" || svixTimestamp == "" {
		return false
	}

	toSign := svixID + "." + svixTimestamp + "." + string(body)

	secretBytes, err := hex.DecodeString(rawSecret)
	if err != nil {
		// Try raw bytes if not hex-encoded
		secretBytes = []byte(rawSecret)
	}

	mac := hmac.New(sha256.New, secretBytes)
	mac.Write([]byte(toSign))
	computed := "v1," + hex.EncodeToString(mac.Sum(nil))

	// Compare against each signature in the header (multiple may be present)
	for _, sig := range strings.Split(svixSig, " ") {
		if hmac.Equal([]byte(computed), []byte(strings.TrimSpace(sig))) {
			return true
		}
	}
	return false
}
