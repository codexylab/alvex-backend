package handlers

import (
	"database/sql"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/codexylab/alvex-backend/pkg/repository"
	"github.com/codexylab/alvex-backend/pkg/response"
	"github.com/codexylab/alvex-backend/pkg/services"
)

type requestLimiter interface {
	Allow(key string) bool
}

// WidgetHandler owns the public bootstrap contract. Conversation operations are
// handled separately after RequireWidgetSession attaches a trusted session.
type WidgetHandler struct {
	Sessions *services.WidgetSessionService
	Limiter  requestLimiter
}

func NewWidgetHandler(sessions *services.WidgetSessionService, limiter requestLimiter) *WidgetHandler {
	return &WidgetHandler{Sessions: sessions, Limiter: limiter}
}

func (h *WidgetHandler) Bootstrap(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	clientID := strings.TrimSpace(chi.URLParam(r, "clientId"))
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if clientID == "" || origin == "" {
		response.Forbidden(w)
		return
	}
	if h.Limiter != nil && !h.Limiter.Allow(clientID+":"+requestIP(r)) {
		response.JSON(w, http.StatusTooManyRequests, response.APIResponse{
			Success: false,
			Error:   "Too many widget session requests",
		})
		return
	}

	bootstrap, err := h.Sessions.Bootstrap(r.Context(), clientID, origin)
	switch {
	case errors.Is(err, repository.ErrWidgetOriginForbidden), errors.Is(err, sql.ErrNoRows):
		response.Forbidden(w)
		return
	case err != nil:
		response.InternalError(w)
		return
	}

	response.Created(w, bootstrap)
}

func (h *WidgetHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	session, ok := requireWidgetSession(w, r)
	if !ok {
		return
	}
	if err := h.Sessions.Revoke(r.Context(), session.ID, session.ClientID); err != nil {
		response.InternalError(w)
		return
	}
	response.NoContent(w)
}

func requestIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	if value := strings.TrimSpace(r.RemoteAddr); value != "" {
		return value
	}
	return "unknown"
}
