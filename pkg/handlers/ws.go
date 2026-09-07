package handlers

import (
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"

	"github.com/codexylab/alvex-backend/pkg/response"
	"github.com/codexylab/alvex-backend/pkg/tenant"
)

// WSHub manages tenant-partitioned WebSocket connections.
type WSHub struct {
	mu             sync.RWMutex
	clients        map[*websocket.Conn]string
	allowedOrigins map[string]struct{}
	upgrader       websocket.Upgrader
}

func NewWSHub(allowedOrigins []string) *WSHub {
	hub := &WSHub{
		clients:        make(map[*websocket.Conn]string),
		allowedOrigins: make(map[string]struct{}, len(allowedOrigins)),
	}
	for _, origin := range allowedOrigins {
		hub.allowedOrigins[strings.TrimRight(origin, "/")] = struct{}{}
	}
	hub.upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     hub.checkOrigin,
	}
	return hub
}

func (h *WSHub) checkOrigin(r *http.Request) bool {
	origin := strings.TrimRight(r.Header.Get("Origin"), "/")
	_, allowed := h.allowedOrigins[origin]
	return origin != "" && allowed
}

// ServeWS registers a connection under the verified organization scope.
func (h *WSHub) ServeWS(w http.ResponseWriter, r *http.Request) {
	scope, ok := tenant.FromContext(r.Context())
	if !ok {
		response.Forbidden(w)
		return
	}
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("websocket upgrade rejected", "error", err)
		return
	}

	h.mu.Lock()
	h.clients[conn] = scope.OrganizationID
	h.mu.Unlock()
	slog.Info("websocket client connected", "organization_id", scope.OrganizationID, "remote", conn.RemoteAddr())

	defer func() {
		h.mu.Lock()
		delete(h.clients, conn)
		h.mu.Unlock()
		_ = conn.Close()
		slog.Info("websocket client disconnected", "organization_id", scope.OrganizationID, "remote", conn.RemoteAddr())
	}()
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

// BroadcastToOrganization sends an event only to connections in one tenant.
func (h *WSHub) BroadcastToOrganization(organizationID string, message []byte) {
	if organizationID == "" {
		slog.Warn("websocket event dropped because organization scope is missing")
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for conn, connectedOrganizationID := range h.clients {
		if connectedOrganizationID != organizationID {
			continue
		}
		if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
			slog.Warn("websocket write failed, removing connection", "error", err)
			_ = conn.Close()
			delete(h.clients, conn)
		}
	}
}

func (h *WSHub) ConnectedCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
