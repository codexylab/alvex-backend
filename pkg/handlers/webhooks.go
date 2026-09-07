package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/google/uuid"

	"github.com/codexylab/alvex-backend/pkg/models"
	"github.com/codexylab/alvex-backend/pkg/ratelimit"
	"github.com/codexylab/alvex-backend/pkg/repository"
	"github.com/codexylab/alvex-backend/pkg/response"
	"github.com/codexylab/alvex-backend/pkg/services"
)

// WebhookHandler processes incoming messages from WhatsApp and web chat widgets.
type WebhookHandler struct {
	Service             *services.ChatService
	WhatsAppVerifyToken string
	WhatsAppAppSecret   string             // Used to verify X-Hub-Signature-256 from Meta
	Limiter             *ratelimit.Limiter // Per-client rate limiter
	LeadRepo            repository.LeadRepository
	WhatsApp            *services.WhatsAppWebhookService
}

// VerifyWhatsApp handles the GET challenge verification for WhatsApp Business API.
//
// GET /webhook/wa/v2/:clientId
func (h *WebhookHandler) VerifyWhatsApp(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("hub.mode")
	token := r.URL.Query().Get("hub.verify_token")
	challenge := r.URL.Query().Get("hub.challenge")

	if h.WhatsAppVerifyToken != "" && mode == "subscribe" && token == h.WhatsAppVerifyToken {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(challenge))
		return
	}
	w.WriteHeader(http.StatusForbidden)
}

// verifyWhatsAppSignature cryptographically validates the X-Hub-Signature-256
// header using the WhatsApp App Secret. Returns true only if the signature matches.
func (h *WebhookHandler) verifyWhatsAppSignature(r *http.Request, body []byte) bool {
	if h.WhatsAppAppSecret == "" {
		return false
	}
	sig := r.Header.Get("X-Hub-Signature-256")
	if sig == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(h.WhatsAppAppSecret))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(sig), []byte(expected))
}

// ReceiveWhatsApp processes an inbound WhatsApp message, routes it to the
// client's configured AI, saves the activity, and broadcasts to the live feed.
//
// POST /webhook/wa/v2/:clientId
func (h *WebhookHandler) ReceiveWhatsApp(w http.ResponseWriter, r *http.Request) {
	clientID := chi.URLParam(r, "clientId")

	body, ok := readRequestBody(w, r, signedWebhookBodyLimit)
	if !ok {
		return
	}

	if !h.verifyWhatsAppSignature(r, body) {
		slog.Warn("whatsapp webhook: invalid signature", "client_id", clientID)
		response.AuthenticationFailed(w, "Invalid WhatsApp webhook signature")
		return
	}

	if h.WhatsApp == nil {
		response.ServiceUnavailable(w, "WhatsApp processing is not configured")
		return
	}
	if _, err := h.WhatsApp.Accept(r.Context(), clientID, body); err != nil {
		slog.Error("whatsapp webhook: durable acceptance failed", "client_id", clientID, "error", err)
		if errors.Is(err, services.ErrInvalidWhatsAppWebhook) {
			response.BadRequest(w, "Invalid WhatsApp webhook payload")
			return
		}
		response.InternalError(w)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// webChatPayload is the body shape for web widget chat messages.
type webChatPayload struct {
	Message      string `json:"message"`
	TicketType   string `json:"ticket_type,omitempty"`
	ContactEmail string `json:"contact_email,omitempty"`
	IsTicket     bool   `json:"is_ticket,omitempty"`
	Image        string `json:"image,omitempty"`
}

// ReceiveWebChat processes an inbound web chat message and returns the AI reply.
//
// POST /webhook/chat/:clientId
func (h *WebhookHandler) ReceiveWebChat(w http.ResponseWriter, r *http.Request) {
	session, ok := requireWidgetSession(w, r)
	if !ok {
		return
	}
	clientID := session.ClientID

	var payload webChatPayload
	if !decodeWidgetJSON(w, r, &payload) {
		return
	}
	payload.Message = strings.TrimSpace(payload.Message)
	payload.ContactEmail = strings.TrimSpace(payload.ContactEmail)
	if err := validateWidgetMessage(payload.Message, payload.Image); err != nil {
		response.BadRequest(w, err.Error())
		return
	}
	userRef := widgetVisitorReference(session.ID)

	if h.Limiter != nil && !h.Limiter.Allow(clientID+":"+requestIP(r)) {
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}

	if payload.IsTicket {
		if payload.ContactEmail != "" {
			address, err := mail.ParseAddress(payload.ContactEmail)
			if err != nil || !strings.EqualFold(address.Address, payload.ContactEmail) || len(payload.ContactEmail) > 254 {
				response.BadRequest(w, "A valid contact_email is required")
				return
			}
		}
		var ticketLabel string
		switch payload.TicketType {
		case "complaint":
			if !session.TicketingEnabled {
				response.Forbidden(w)
				return
			}
			ticketLabel = "Register a Complaint"
		case "admin_message":
			if !session.AdminMsgEnabled {
				response.Forbidden(w)
				return
			}
			ticketLabel = "Message Administration"
		default:
			response.BadRequest(w, "A valid ticket_type is required")
			return
		}

		ticketMessage := "[" + ticketLabel + "] " + payload.Message
		if payload.ContactEmail != "" {
			ticketMessage += "\nContact email: " + payload.ContactEmail
		}
		logID, _, err := h.Service.RegisterTicket(r.Context(), clientID, ticketMessage, session.ID, session.ID)
		if err != nil {
			response.InternalError(w)
			return
		}

		response.Success(w, map[string]interface{}{
			"reply":      "Thank you! Your ticket has been registered. You will receive replies right here.",
			"ticket_id":  logID,
			"ticket_ref": session.ID,
		})
		return
	}
	if payload.Image != "" && !session.ImageSearchEnabled {
		response.Forbidden(w)
		return
	}

	aiResponse := h.Service.ProcessMessage(r.Context(), clientID, userRef, session.ID, payload.Message, payload.Image, string(models.ChannelWeb))
	response.Success(w, map[string]string{"reply": aiResponse})
}

// GetWebChatHistory fetches conversation history for a given session.
//
// GET /webhook/chat/{clientId}/history
func (h *WebhookHandler) GetWebChatHistory(w http.ResponseWriter, r *http.Request) {
	session, ok := requireWidgetSession(w, r)
	if !ok {
		return
	}
	clientID := session.ClientID
	historyType := r.URL.Query().Get("type") // "chat" (default) or "tickets"

	var history []repository.HistoryItem
	var err error

	if historyType == "tickets" {
		history, err = h.Service.GetChatHistory(r.Context(), clientID, "tickets", session.ID)
	} else {
		history, err = h.Service.GetChatHistory(r.Context(), clientID, "chat", session.ID)
	}

	if err != nil {
		response.InternalError(w)
		return
	}

	response.Success(w, history)
}

// SaveMessageReaction updates the reaction for a specific chat message.
//
// PATCH /webhook/chat/message/{id}/reaction
func (h *WebhookHandler) SaveMessageReaction(w http.ResponseWriter, r *http.Request) {
	session, ok := requireWidgetSession(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")

	var payload struct {
		Reaction string `json:"reaction"`
	}
	if !decodeWidgetJSON(w, r, &payload) {
		return
	}
	if !isAllowedReaction(payload.Reaction) {
		response.BadRequest(w, "Reaction is not allowed")
		return
	}

	if err := h.Service.UpdateMessageReaction(r.Context(), id, session.ClientID, session.ID, payload.Reaction); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.NotFound(w, "Message")
			return
		}
		response.InternalError(w)
		return
	}

	response.Success(w, map[string]string{"message": "Reaction updated"})
}

// CaptureLead handles customer contact information submitted through the widget lead generation form.
//
// POST /webhook/chat/:clientId/lead
func (h *WebhookHandler) CaptureLead(w http.ResponseWriter, r *http.Request) {
	session, ok := requireWidgetSession(w, r)
	if !ok {
		return
	}
	clientID := session.ClientID

	var req models.CreateLeadRequest
	if !decodeWidgetJSON(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.TrimSpace(req.Email)
	req.Phone = strings.TrimSpace(req.Phone)
	if req.Name == "" || len(req.Name) > 200 || len(req.Email) > 254 || len(req.Phone) > 50 {
		response.BadRequest(w, "Name is required")
		return
	}

	lead := &models.Lead{
		ID:        uuid.New().String(),
		ClientID:  clientID,
		Name:      req.Name,
		Email:     req.Email,
		Phone:     req.Phone,
		SessionID: session.ID,
		Source:    "widget",
		CreatedAt: time.Now(),
	}

	if h.LeadRepo != nil {
		if err := h.LeadRepo.InsertLead(r.Context(), lead); err != nil {
			response.InternalError(w)
			return
		}
	}

	response.Created(w, lead)
}

// TypingIndicator broadcasts typing status to client portal via WebSocket.
//
// POST /webhook/chat/:clientId/typing
func (h *WebhookHandler) TypingIndicator(w http.ResponseWriter, r *http.Request) {
	session, ok := requireWidgetSession(w, r)
	if !ok {
		return
	}
	clientID := session.ClientID

	var payload struct {
		IsTyping bool `json:"is_typing"`
	}
	if !decodeWidgetJSON(w, r, &payload) {
		return
	}

	if err := h.Service.BroadcastTyping(r.Context(), clientID, session.ID, payload.IsTyping); err != nil {
		response.NotFound(w, "Client")
		return
	}

	response.Success(w, map[string]bool{"ok": true})
}

func isAllowedReaction(reaction string) bool {
	switch reaction {
	case "", "👍", "❤️", "😊":
		return true
	default:
		return false
	}
}
