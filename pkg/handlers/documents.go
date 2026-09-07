package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/codexylab/alvex-backend/pkg/middleware"
	"github.com/codexylab/alvex-backend/pkg/repository"
	"github.com/codexylab/alvex-backend/pkg/response"
	"github.com/codexylab/alvex-backend/pkg/services"
)

// DocumentHandler handles document uploads and management for knowledge base enhancement.
type DocumentHandler struct {
	AdminService  *services.DocumentService
	PortalService *services.DocumentService
}

// NewDocumentHandler creates a new DocumentHandler instance.
func NewDocumentHandler(adminService, portalService *services.DocumentService) *DocumentHandler {
	return &DocumentHandler{AdminService: adminService, PortalService: portalService}
}

// UploadAdmin handles document upload from the admin dashboard.
//
// POST /api/v1/clients/:id/documents
func (h *DocumentHandler) UploadAdmin(w http.ResponseWriter, r *http.Request) {
	clientID := chi.URLParam(r, "id")
	h.handleUpload(w, r, h.AdminService, clientID)
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
	h.handleUpload(w, r, h.PortalService, clientID)
}

func (h *DocumentHandler) handleUpload(w http.ResponseWriter, r *http.Request, service *services.DocumentService, clientID string) {
	// Include a small allowance for multipart headers around the 15 MiB file.
	r.Body = http.MaxBytesReader(w, r.Body, (16 << 20))
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

	doc, err := service.ProcessUpload(r.Context(), clientID, header.Filename, header.Size, file)
	if err != nil {
		response.BadRequest(w, fmt.Sprintf("Failed to process document: %v", err))
		return
	}

	response.Created(w, doc)
}

type manualKnowledgeRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

func (h *DocumentHandler) UploadTextAdmin(w http.ResponseWriter, r *http.Request) {
	h.handleTextUpload(w, r, h.AdminService, chi.URLParam(r, "id"))
}

func (h *DocumentHandler) UploadTextPortal(w http.ResponseWriter, r *http.Request) {
	clientID := middleware.GetPortalClientID(r)
	if clientID == "" {
		response.Unauthorized(w)
		return
	}
	h.handleTextUpload(w, r, h.PortalService, clientID)
}

func (h *DocumentHandler) handleTextUpload(
	w http.ResponseWriter,
	r *http.Request,
	service *services.DocumentService,
	clientID string,
) {
	body, ok := readRequestBody(w, r, (512<<10)+4096)
	if !ok {
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var request manualKnowledgeRequest
	if err := decoder.Decode(&request); err != nil {
		response.BadRequest(w, "Invalid request body")
		return
	}
	document, err := service.ProcessText(r.Context(), clientID, request.Title, request.Content)
	if err != nil {
		response.BadRequest(w, err.Error())
		return
	}
	response.Created(w, document)
}

// ListAdmin lists documents for a client in the admin dashboard.
//
// GET /api/v1/clients/:id/documents
func (h *DocumentHandler) ListAdmin(w http.ResponseWriter, r *http.Request) {
	clientID := chi.URLParam(r, "id")
	docs, err := h.AdminService.ListDocuments(r.Context(), clientID)
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
	docs, err := h.PortalService.ListDocuments(r.Context(), clientID)
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

	if err := h.AdminService.DeleteDocument(r.Context(), docID, clientID); err != nil {
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

	if err := h.PortalService.DeleteDocument(r.Context(), docID, clientID); err != nil {
		response.InternalError(w)
		return
	}
	response.NoContent(w)
}

// ReindexAdmin queues a fresh index generation for an owned client document.
func (h *DocumentHandler) ReindexAdmin(w http.ResponseWriter, r *http.Request) {
	h.handleReindex(w, r, h.AdminService, chi.URLParam(r, "id"), chi.URLParam(r, "docId"))
}

// ReindexPortal queues a fresh index generation for the active portal client.
func (h *DocumentHandler) ReindexPortal(w http.ResponseWriter, r *http.Request) {
	clientID := middleware.GetPortalClientID(r)
	if clientID == "" {
		response.Unauthorized(w)
		return
	}
	h.handleReindex(w, r, h.PortalService, clientID, chi.URLParam(r, "docId"))
}

func (h *DocumentHandler) handleReindex(
	w http.ResponseWriter,
	r *http.Request,
	service *services.DocumentService,
	clientID string,
	documentID string,
) {
	jobID, err := service.ReindexDocument(r.Context(), documentID, clientID)
	if errors.Is(err, repository.ErrDocumentAlreadyQueued) {
		response.Conflict(w, "Document is already queued or processing")
		return
	}
	if err != nil {
		response.NotFound(w, "Document")
		return
	}
	response.JSON(w, http.StatusAccepted, response.APIResponse{Success: true, Data: map[string]string{
		"job_id": jobID,
		"status": "queued",
	}})
}
