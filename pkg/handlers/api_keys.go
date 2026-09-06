package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/codexylab/alvex-backend/pkg/apierr"
	"github.com/codexylab/alvex-backend/pkg/middleware"
	"github.com/codexylab/alvex-backend/pkg/models"
	"github.com/codexylab/alvex-backend/pkg/response"
	"github.com/codexylab/alvex-backend/pkg/services"
)

// APIKeyHandler exposes tenant-owned machine credential lifecycle operations.
type APIKeyHandler struct {
	Service *services.APIKeyService
}

func NewAPIKeyHandler(service *services.APIKeyService) *APIKeyHandler {
	return &APIKeyHandler{Service: service}
}

func (h *APIKeyHandler) Create(w http.ResponseWriter, r *http.Request) {
	var request models.CreateAPIKeyRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		response.BadRequest(w, "Invalid request body")
		return
	}
	if err := requireJSONEOF(decoder); err != nil {
		response.BadRequest(w, "Request body must contain one JSON object")
		return
	}

	created, err := h.Service.Create(r.Context(), request, middleware.GetUserID(r))
	if err != nil {
		if errors.Is(err, apierr.ErrValidation) {
			response.BadRequest(w, err.Error())
			return
		}
		slog.Error("create API key failed", "error", err, "request_id", middleware.GetRequestID(r))
		response.InternalError(w)
		return
	}
	response.Created(w, created)
}

func (h *APIKeyHandler) List(w http.ResponseWriter, r *http.Request) {
	keys, err := h.Service.List(r.Context())
	if err != nil {
		slog.Error("list API keys failed", "error", err, "request_id", middleware.GetRequestID(r))
		response.InternalError(w)
		return
	}
	response.Success(w, keys)
}

func (h *APIKeyHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	if err := h.Service.Revoke(r.Context(), chi.URLParam(r, "id")); err != nil {
		switch {
		case errors.Is(err, apierr.ErrValidation):
			response.BadRequest(w, err.Error())
		case errors.Is(err, apierr.ErrNotFound):
			response.NotFound(w, "API key")
		default:
			slog.Error("revoke API key failed", "error", err, "request_id", middleware.GetRequestID(r))
			response.InternalError(w)
		}
		return
	}
	response.NoContent(w)
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra interface{}
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}
