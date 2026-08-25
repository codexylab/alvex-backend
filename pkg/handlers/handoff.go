package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/codexylab/alvex-backend/pkg/database"
	"github.com/codexylab/alvex-backend/pkg/models"
	"github.com/codexylab/alvex-backend/pkg/response"
)

// HandoffHandler manages human intervention and ticket resolution for escalated AI conversations.
type HandoffHandler struct {
	DB  *database.DB
	Hub *WSHub
}

// NewHandoffHandler creates a new HandoffHandler.
func NewHandoffHandler(db *database.DB, hub *WSHub) *HandoffHandler {
	return &HandoffHandler{DB: db, Hub: hub}
}

// ListNeedsAttention returns conversations flagged for human support.
//
// GET /api/v1/conversations/needs-attention
func (h *HandoffHandler) ListNeedsAttention(w http.ResponseWriter, r *http.Request) {
	query := `
		SELECT id, client_id, client_name, channel, user_ref, session_id, message,
		       COALESCE(ai_response,''), status, latency_ms, COALESCE(needs_human,0),
		       COALESCE(human_reply,''), replied_at, COALESCE(handoff_reason,''), created_at
		FROM activity_logs
		WHERE (needs_human = 1 OR status = 'Needs Human') AND status != 'Resolved'
		ORDER BY created_at DESC
		LIMIT 50
	`

	rows, err := h.DB.QueryContext(r.Context(), h.DB.Adapt(query))
	if err != nil {
		response.InternalError(w)
		return
	}
	defer rows.Close()

	var logs []models.ActivityLog
	for rows.Next() {
		var log models.ActivityLog
		var clientID, aiResp, humanReply, handoffReason string
		var repliedAt *time.Time
		var needsHumanVal interface{}

		if err := rows.Scan(
			&log.ID, &clientID, &log.ClientName, &log.Channel, &log.UserRef, &log.SessionID, &log.Message,
			&aiResp, &log.Status, &log.LatencyMs, &needsHumanVal,
			&humanReply, &repliedAt, &handoffReason, &log.CreatedAt,
		); err != nil {
			response.InternalError(w)
			return
		}

		if clientID != "" {
			log.ClientID = &clientID
		}
		log.AIResponse = aiResp
		log.HumanReply = humanReply
		log.RepliedAt = repliedAt
		log.HandoffReason = handoffReason
		log.NeedsHuman = true

		logs = append(logs, log)
	}

	response.Success(w, logs)
}

// HumanReply sends an agent's manual response to a customer conversation and marks it resolved.
//
// POST /api/v1/conversations/:id/reply
func (h *HandoffHandler) HumanReply(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var payload struct {
		Reply string `json:"reply"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.Reply == "" {
		response.BadRequest(w, "Reply text is required")
		return
	}

	now := time.Now()
	query := `
		UPDATE activity_logs
		SET human_reply = $1, replied_at = $2, status = 'Resolved', needs_human = 0
		WHERE id = $3
	`
	res, err := h.DB.ExecContext(r.Context(), h.DB.Adapt(query), payload.Reply, now, id)
	if err != nil {
		response.InternalError(w)
		return
	}

	rows, _ := res.RowsAffected()
	if rows == 0 {
		response.NotFound(w, "Conversation")
		return
	}

	if h.Hub != nil {
		event, _ := json.Marshal(map[string]interface{}{
			"id":          id,
			"human_reply": payload.Reply,
			"status":      "Resolved",
			"replied_at":  now.Format(time.RFC3339),
		})
		h.Hub.Broadcast(event)
	}

	response.Success(w, map[string]interface{}{
		"message":    "Reply sent and conversation marked as resolved",
		"id":         id,
		"replied_at": now,
	})
}

// ResolveConversation marks a conversation resolved without sending a reply.
//
// POST /api/v1/conversations/:id/resolve
func (h *HandoffHandler) ResolveConversation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	query := `
		UPDATE activity_logs
		SET status = 'Resolved', needs_human = 0
		WHERE id = $1
	`
	res, err := h.DB.ExecContext(r.Context(), h.DB.Adapt(query), id)
	if err != nil {
		response.InternalError(w)
		return
	}

	rows, _ := res.RowsAffected()
	if rows == 0 {
		response.NotFound(w, "Conversation")
		return
	}

	response.Success(w, map[string]string{"message": "Conversation marked as resolved"})
}
