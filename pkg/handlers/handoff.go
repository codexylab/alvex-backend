package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/codexylab/alvex-backend/pkg/apierr"
	"github.com/codexylab/alvex-backend/pkg/response"
	"github.com/codexylab/alvex-backend/pkg/services"
	"github.com/codexylab/alvex-backend/pkg/tenant"
)

// HandoffHandler transports human-intervention requests to the service layer.
type HandoffHandler struct {
	Service *services.HandoffService
	Hub     *WSHub
}

func NewHandoffHandler(service *services.HandoffService, hub *WSHub) *HandoffHandler {
	return &HandoffHandler{Service: service, Hub: hub}
}

func (h *HandoffHandler) ListNeedsAttention(w http.ResponseWriter, r *http.Request) {
	logs, err := h.Service.ListNeedsAttention(r.Context(), 50)
	if err != nil {
		response.InternalError(w)
		return
	}
	response.Success(w, logs)
}

func (h *HandoffHandler) HumanReply(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var payload struct {
		Reply string `json:"reply"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || strings.TrimSpace(payload.Reply) == "" {
		response.BadRequest(w, "Reply text is required")
		return
	}

	repliedAt, err := h.Service.Reply(r.Context(), id, payload.Reply)
	if err != nil {
		if errors.Is(err, apierr.ErrNotFound) {
			response.NotFound(w, "Conversation")
			return
		}
		response.InternalError(w)
		return
	}

	scope, _ := tenant.FromContext(r.Context())
	h.broadcastReply(scope.OrganizationID, id, payload.Reply, repliedAt)
	response.Success(w, map[string]interface{}{
		"message":    "Reply sent and conversation marked as resolved",
		"id":         id,
		"replied_at": repliedAt,
	})
}

func (h *HandoffHandler) ResolveConversation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Service.Resolve(r.Context(), id); err != nil {
		if errors.Is(err, apierr.ErrNotFound) {
			response.NotFound(w, "Conversation")
			return
		}
		response.InternalError(w)
		return
	}
	response.Success(w, map[string]string{"message": "Conversation marked as resolved"})
}

func (h *HandoffHandler) broadcastReply(organizationID, id, reply string, repliedAt time.Time) {
	if h.Hub == nil {
		return
	}
	event, _ := json.Marshal(map[string]interface{}{
		"id":          id,
		"human_reply": reply,
		"status":      "Resolved",
		"replied_at":  repliedAt.Format(time.RFC3339),
	})
	h.Hub.BroadcastToOrganization(organizationID, event)
}
