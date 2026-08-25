package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/codexylab/alvex-backend/pkg/middleware"
	"github.com/codexylab/alvex-backend/pkg/models"
	"github.com/codexylab/alvex-backend/pkg/services"
	"github.com/codexylab/alvex-backend/pkg/apierr"
	"github.com/codexylab/alvex-backend/pkg/response"
)

// ClientHandler handles all /api/v1/clients HTTP transport routing.
type ClientHandler struct {
	Service   *services.ClientService
	PortalSvc *services.PortalService
}

// List returns a paginated, searchable, filterable list of clients.
//
// GET /api/v1/clients?search=nexus&status=Active&page=1&limit=5
func (h *ClientHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	search := strings.ToLower(strings.TrimSpace(q.Get("search")))
	status := q.Get("status")
	page, _ := strconv.Atoi(q.Get("page"))
	limit, _ := strconv.Atoi(q.Get("limit"))

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}

	clients, total, err := h.Service.List(r.Context(), search, status, page, limit)
	if err != nil {
		log.Printf("[ERROR] clients List query failed: %v", err)
		response.InternalError(w)
		return
	}

	totalPages := (total + limit - 1) / limit
	if totalPages == 0 {
		totalPages = 1
	}

	response.Success(w, models.ClientListResponse{
		Data:       clients,
		Total:      total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
	})
}

// Create adds a new client profile.
//
// POST /api/v1/clients
func (h *ClientHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req models.CreateClientRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "Invalid JSON body")
		return
	}

	ownerID := middleware.GetUserID(r)
	client, err := h.Service.Create(r.Context(), req, ownerID)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "duplicate") {
			response.Conflict(w, errMsg)
			return
		}
		if strings.Contains(errMsg, "required") || strings.Contains(errMsg, "valid") || strings.Contains(errMsg, "not valid") {
			response.BadRequest(w, errMsg)
			return
		}
		log.Printf("[ERROR] clients Create execution failed: %v", err)
		response.InternalError(w)
		return
	}

	// Trigger background website scrape if domain is provided
	if req.Domain != "" && h.PortalSvc != nil {
		go func(cid, dom string) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			_, _, _ = h.Service.ScrapeAndSave(ctx, cid, dom, h.PortalSvc)
		}(client.ID, req.Domain)
	}

	response.Created(w, client)
}

// GetOne returns a single client by ID with full (unmasked) API key.
//
// GET /api/v1/clients/:id
func (h *ClientHandler) GetOne(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	client, err := h.Service.GetByID(r.Context(), id)
	if err != nil {
		response.NotFound(w, "Client")
		return
	}
	response.Success(w, client)
}

// Update replaces a client's configuration fields.
//
// PUT /api/v1/clients/:id
func (h *ClientHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req models.UpdateClientRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "Invalid JSON body")
		return
	}

	client, domainToSync, err := h.Service.Update(r.Context(), id, req)
	if err != nil {
		if errors.Is(err, apierr.ErrNotFound) {
			response.NotFound(w, "Client")
			return
		}
		log.Printf("[ERROR] clients Update failed: %v", err)
		response.InternalError(w)
		return
	}

	if domainToSync != "" && h.PortalSvc != nil {
		go func(cid, dom string) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			_, _, _ = h.Service.ScrapeAndSave(ctx, cid, dom, h.PortalSvc)
		}(id, domainToSync)
	}

	response.Success(w, client)
}

// ToggleStatus switches a client between Active and Suspended states.
//
// PATCH /api/v1/clients/:id/status
func (h *ClientHandler) ToggleStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	newStatus, err := h.Service.ToggleStatus(r.Context(), id)
	if err != nil {
		if errors.Is(err, apierr.ErrNotFound) {
			response.NotFound(w, "Client")
			return
		}
		log.Printf("[ERROR] clients ToggleStatus failed: %v", err)
		response.InternalError(w)
		return
	}

	response.Message(w, fmt.Sprintf("Client status updated to %s", newStatus))
}

// Delete permanently removes a client profile.
//
// DELETE /api/v1/clients/:id
func (h *ClientHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	err := h.Service.Delete(r.Context(), id)
	if err != nil {
		if errors.Is(err, apierr.ErrActiveClient) {
			response.BadRequest(w, "Only suspended clients can be deleted. Please suspend the client first.")
			return
		}
		if errors.Is(err, apierr.ErrNotFound) {
			response.NotFound(w, "Client")
			return
		}
		log.Printf("[ERROR] clients Delete failed: %v", err)
		response.InternalError(w)
		return
	}

	response.NoContent(w)
}

// RotateToken generates a new portal token for a client, invalidating the old one.
//
// POST /api/v1/clients/:id/rotate-token
func (h *ClientHandler) RotateToken(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	client, err := h.Service.RotateToken(r.Context(), id)
	if err != nil {
		if errors.Is(err, apierr.ErrNotFound) {
			response.NotFound(w, "Client")
			return
		}
		log.Printf("[ERROR] clients RotateToken failed: %v", err)
		response.InternalError(w)
		return
	}

	response.Success(w, client)
}

// RotateKey generates a new platform API key for a client, invalidating the previous key.
//
// POST /api/v1/clients/:id/rotate-key
func (h *ClientHandler) RotateKey(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	userID := middleware.GetUserID(r)

	client, err := h.Service.RotateAPIKey(r.Context(), id, userID)
	if err != nil {
		if errors.Is(err, apierr.ErrNotFound) {
			response.NotFound(w, "Client")
			return
		}
		log.Printf("[ERROR] clients RotateKey failed: %v", err)
		response.InternalError(w)
		return
	}

	response.Success(w, client)
}
