package handlers

import (
	"log/slog"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// WSHub manages all active WebSocket connections and broadcasts messages.
type WSHub struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]bool
}

// NewWSHub creates and returns a new WebSocket hub.
func NewWSHub() *WSHub {
	return &WSHub{
		clients: make(map[*websocket.Conn]bool),
	}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// Allow connections from any origin (tighten in production via config)
	CheckOrigin: func(r *http.Request) bool { return true },
}

// ServeWS upgrades an HTTP connection to WebSocket and registers the client.
//
// WS /ws/activity
func (h *WSHub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("websocket upgrade failed", "error", err)
		return
	}

	h.mu.Lock()
	h.clients[conn] = true
	h.mu.Unlock()

	slog.Info("websocket client connected", "remote", conn.RemoteAddr())

	// Block here — read loop keeps the connection alive and detects disconnects
	defer func() {
		h.mu.Lock()
		delete(h.clients, conn)
		h.mu.Unlock()
		conn.Close()
		slog.Info("websocket client disconnected", "remote", conn.RemoteAddr())
	}()

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break // Client disconnected or sent invalid frame
		}
	}
}

// Broadcast sends a JSON message to every connected WebSocket client.
// Failed connections are removed from the hub to prevent double-close on next broadcast.
func (h *WSHub) Broadcast(message []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for conn := range h.clients {
		if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
			slog.Warn("websocket write failed, removing connection", "error", err)
			conn.Close()
			delete(h.clients, conn)
		}
	}
}

// ConnectedCount returns the number of active WebSocket connections.
func (h *WSHub) ConnectedCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
