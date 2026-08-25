package handlers

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/codexylab/alvex-backend/pkg/middleware"
	"github.com/codexylab/alvex-backend/pkg/services"
	"github.com/codexylab/alvex-backend/pkg/response"
)

// DocumentHandler handles document uploads and management for knowledge base enhancement.
type DocumentHandler struct {
	DocService *services.DocumentService
}

// NewDocumentHandler creates a new DocumentHandler instance.
func NewDocumentHandler(docService *services.DocumentService) *DocumentHandler {
	return &DocumentHandler{DocService: docService}
}

// UploadAdmin handles document upload from the admin dashboard.
//
// POST /api/v1/clients/:id/documents
func (h *DocumentHandler) UploadAdmin(w http.ResponseWriter, r *http.Request) {
	clientID := chi.URLParam(r, "id")
	h.handleUpload(w, r, clientID)
}

// UploadPortal handles document upload from the client self-service portal.
//
// POST /api/v1/client-portal/documents
func (h *DocumentHandler) UploadPortal(w http.ResponseWriter, r *http.Request) {
	clientID := middleware.GetPortalClientID(r)
	if clientID == "" {
		response.Unauthorized(w)
		return
	}
	h.handleUpload(w, r, clientID)
}

func (h *DocumentHandler) handleUpload(w http.ResponseWriter, r *http.Request, clientID string) {
	// Limit upload size to 15 MB
	if err := r.ParseMultipartForm(15 << 20); err != nil {
		response.BadRequest(w, "File too large (max 15MB)")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		response.BadRequest(w, "File parameter 'file' is required")
		return
	}
	defer file.Close()

	doc, err := h.DocService.ProcessUpload(r.Context(), clientID, header.Filename, header.Size, file)
	if err != nil {
		response.BadRequest(w, fmt.Sprintf("Failed to process document: %v", err))
		return
	}

	response.Created(w, doc)
}

// ListAdmin lists documents for a client in the admin dashboard.
//
// GET /api/v1/clients/:id/documents
func (h *DocumentHandler) ListAdmin(w http.ResponseWriter, r *http.Request) {
	clientID := chi.URLParam(r, "id")
	docs, err := h.DocService.ListDocuments(r.Context(), clientID)
	if err != nil {
		response.InternalError(w)
		return
	}
	response.Success(w, docs)
}

// ListPortal lists documents for the authenticated portal client.
//
// GET /api/v1/client-portal/documents
func (h *DocumentHandler) ListPortal(w http.ResponseWriter, r *http.Request) {
	clientID := middleware.GetPortalClientID(r)
	if clientID == "" {
		response.Unauthorized(w)
		return
	}
	docs, err := h.DocService.ListDocuments(r.Context(), clientID)
	if err != nil {
		response.InternalError(w)
		return
	}
	response.Success(w, docs)
}

// DeleteAdmin removes a document via admin API.
//
// DELETE /api/v1/clients/:id/documents/:docId
func (h *DocumentHandler) DeleteAdmin(w http.ResponseWriter, r *http.Request) {
	clientID := chi.URLParam(r, "id")
	docID := chi.URLParam(r, "docId")

	if err := h.DocService.DeleteDocument(r.Context(), docID, clientID); err != nil {
		response.InternalError(w)
		return
	}
	response.NoContent(w)
}

// DeletePortal removes a document via portal API.
//
// DELETE /api/v1/client-portal/documents/:docId
func (h *DocumentHandler) DeletePortal(w http.ResponseWriter, r *http.Request) {
	clientID := middleware.GetPortalClientID(r)
	if clientID == "" {
		response.Unauthorized(w)
		return
	}
	docID := chi.URLParam(r, "docId")

	if err := h.DocService.DeleteDocument(r.Context(), docID, clientID); err != nil {
		response.InternalError(w)
		return
	}
	response.NoContent(w)
}
