package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/google/uuid"

	"github.com/codexylab/alvex-backend/pkg/models"
	"github.com/codexylab/alvex-backend/pkg/queue"
	"github.com/codexylab/alvex-backend/pkg/ratelimit"
	"github.com/codexylab/alvex-backend/pkg/repository"
	"github.com/codexylab/alvex-backend/pkg/services"
	"github.com/codexylab/alvex-backend/pkg/response"
)

// WebhookHandler processes incoming messages from WhatsApp and web chat widgets.
type WebhookHandler struct {
	Service             *services.ChatService
	WhatsAppVerifyToken string
	WhatsAppAppSecret   string // Used to verify X-Hub-Signature-256 from Meta
	Limiter             *ratelimit.Limiter // Per-client rate limiter
	DailyLimiter        *ratelimit.DailyLimiter
	WorkerPool          *queue.WorkerPool
	LeadRepo            repository.LeadRepository
}

// VerifyWhatsApp handles the GET challenge verification for WhatsApp Business API.
//
// GET /webhook/wa/v2/:clientId
func (h *WebhookHandler) VerifyWhatsApp(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("hub.mode")
	token := r.URL.Query().Get("hub.verify_token")
	challenge := r.URL.Query().Get("hub.challenge")

	if mode == "subscribe" && token == h.WhatsAppVerifyToken {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(challenge))
		return
	}
	w.WriteHeader(http.StatusForbidden)
}

// whatsAppPayload matches the Meta WhatsApp Cloud API webhook body structure.
type whatsAppPayload struct {
	Object string `json:"object"`
	Entry  []struct {
		Changes []struct {
			Value struct {
				Messages []struct {
					From string `json:"from"`
					Text struct {
						Body string `json:"body"`
					} `json:"text"`
				} `json:"messages"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

// verifyWhatsAppSignature cryptographically validates the X-Hub-Signature-256
// header using the WhatsApp App Secret. Returns true only if the signature matches.
func (h *WebhookHandler) verifyWhatsAppSignature(r *http.Request, body []byte) bool {
	if h.WhatsAppAppSecret == "" {
		return true // no secret configured â€” skip verification (dev mode)
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

	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if !h.verifyWhatsAppSignature(r, body) {
		slog.Warn("whatsapp webhook: invalid signature", "client_id", clientID)
		w.WriteHeader(http.StatusForbidden)
		return
	}

	var payload whatsAppPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if len(payload.Entry) == 0 || len(payload.Entry[0].Changes) == 0 {
		w.WriteHeader(http.StatusOK) // WhatsApp expects 200 even for empty payloads
		return
	}

	messages := payload.Entry[0].Changes[0].Value.Messages
	if len(messages) == 0 {
		w.WriteHeader(http.StatusOK)
		return
	}

	from := messages[0].From
	msgBody := messages[0].Text.Body

	if h.Limiter != nil && !h.Limiter.Allow(clientID) {
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}

	// Process asynchronously via worker pool â€” Meta WhatsApp expects a 200 response within 200ms.
	if h.WorkerPool != nil {
		h.WorkerPool.Submit(queue.Job{
			ClientID:  clientID,
			UserRef:   from,
			SessionID: from,
			Message:   msgBody,
			Channel:   string(models.ChannelWhatsApp),
			Process: func(ctx context.Context, cID, uRef, sID, msg, img, ch string) string {
				return h.Service.ProcessMessage(ctx, cID, uRef, sID, msg, img, ch)
			},
		})
	} else {
		go h.Service.ProcessMessage(context.Background(), clientID, from, from, msgBody, "", string(models.ChannelWhatsApp))
	}

	w.WriteHeader(http.StatusOK)
}

// webChatPayload is the body shape for web widget chat messages.
type webChatPayload struct {
	Message   string `json:"message"`
	UserRef   string `json:"user_ref"`   // e.g. "Visitor #8291" (session-scoped)
	TicketRef string `json:"ticket_ref"` // persistent user ID for ticket lookup
	SessionID string `json:"session_id,omitempty"`
	IsTicket  bool   `json:"is_ticket,omitempty"`
	Image     string `json:"image,omitempty"`
}

// ReceiveWebChat processes an inbound web chat message and returns the AI reply.
//
// POST /webhook/chat/:clientId
func (h *WebhookHandler) ReceiveWebChat(w http.ResponseWriter, r *http.Request) {
	clientID := chi.URLParam(r, "clientId")

	var payload webChatPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		response.BadRequest(w, "Invalid JSON body")
		return
	}

	if payload.Message == "" && payload.Image == "" {
		response.BadRequest(w, "Message or image cannot be empty")
		return
	}

	if payload.UserRef == "" {
		payload.UserRef = fmt.Sprintf("Visitor #%d", time.Now().UnixMilli()%10000)
	}

	if payload.SessionID == "" {
		payload.SessionID = payload.UserRef
	}

	if h.Limiter != nil && !h.Limiter.Allow(clientID) {
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}

	if payload.IsTicket {
		ticketRef := payload.TicketRef
		if ticketRef == "" {
			ticketRef = payload.UserRef
		}

		logID, _, err := h.Service.RegisterTicket(r.Context(), clientID, payload.Message, ticketRef, payload.SessionID)
		if err != nil {
			response.InternalError(w)
			return
		}

		response.Success(w, map[string]interface{}{
			"reply":      "Thank you! Your ticket has been registered. You will receive replies right here.",
			"ticket_id":  logID,
			"ticket_ref": ticketRef,
		})
		return
	}

	aiResponse := h.Service.ProcessMessage(r.Context(), clientID, payload.UserRef, payload.SessionID, payload.Message, payload.Image, string(models.ChannelWeb))
	response.Success(w, map[string]string{"reply": aiResponse})
}

// GetWebChatHistory fetches conversation history for a given session.
//
// GET /webhook/chat/{clientId}/history
func (h *WebhookHandler) GetWebChatHistory(w http.ResponseWriter, r *http.Request) {
	clientID := chi.URLParam(r, "clientId")
	historyType := r.URL.Query().Get("type") // "chat" (default) or "tickets"

	var history []repository.HistoryItem
	var err error

	if historyType == "tickets" {
		ticketRef := r.URL.Query().Get("ticket_ref")
		if ticketRef == "" {
			response.BadRequest(w, "ticket_ref query parameter is required for type=tickets")
			return
		}
		history, err = h.Service.GetChatHistory(r.Context(), clientID, "tickets", ticketRef)
	} else {
		sessionID := r.URL.Query().Get("session_id")
		if sessionID == "" {
			response.BadRequest(w, "session_id query parameter is required")
			return
		}
		history, err = h.Service.GetChatHistory(r.Context(), clientID, "chat", sessionID)
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
	id := chi.URLParam(r, "id")

	var payload struct {
		Reaction string `json:"reaction"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		response.BadRequest(w, "Invalid JSON body")
		return
	}

	if err := h.Service.UpdateMessageReaction(r.Context(), id, payload.Reaction); err != nil {
		response.InternalError(w)
		return
	}

	response.Success(w, map[string]string{"message": "Reaction updated"})
}

// CaptureLead handles customer contact information submitted through the widget lead generation form.
//
// POST /webhook/chat/:clientId/lead
func (h *WebhookHandler) CaptureLead(w http.ResponseWriter, r *http.Request) {
	clientID := chi.URLParam(r, "clientId")

	var req models.CreateLeadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		response.BadRequest(w, "Name is required")
		return
	}

	lead := &models.Lead{
		ID:        uuid.New().String(),
		ClientID:  clientID,
		Name:      req.Name,
		Email:     req.Email,
		Phone:     req.Phone,
		SessionID: req.SessionID,
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
	clientID := chi.URLParam(r, "clientId")

	var payload struct {
		SessionID string `json:"session_id"`
		IsTyping  bool   `json:"is_typing"`
	}
	_ = json.NewDecoder(r.Body).Decode(&payload)

	if h.Service.Hub != nil {
		event, _ := json.Marshal(map[string]interface{}{
			"type":       "typing",
			"client_id":  clientID,
			"session_id": payload.SessionID,
			"is_typing":  payload.IsTyping,
		})
		h.Service.Hub.Broadcast(event)
	}

	response.Success(w, map[string]bool{"ok": true})
}

