package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/codexylab/alvex-backend/internal/models"
	"github.com/codexylab/alvex-backend/internal/services"
	"github.com/codexylab/alvex-backend/pkg/apierr"
	"github.com/codexylab/alvex-backend/pkg/response"
)

// BillingHandler handles all /api/v1/billing HTTP endpoints.
type BillingHandler struct {
	Service *services.BillingService
}

// Stats returns aggregated billing metrics.
//
// GET /api/v1/billing/stats
func (h *BillingHandler) Stats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.Service.GetStats(r.Context())
	if err != nil {
		log.Printf("[ERROR] billing Stats query failed: %v", err)
		response.InternalError(w)
		return
	}
	response.Success(w, stats)
}

// ListInvoices returns all invoices ordered by date descending.
//
// GET /api/v1/billing/invoices
func (h *BillingHandler) ListInvoices(w http.ResponseWriter, r *http.Request) {
	invoices, err := h.Service.ListInvoices(r.Context())
	if err != nil {
		log.Printf("[ERROR] billing ListInvoices query failed: %v", err)
		response.InternalError(w)
		return
	}
	response.Success(w, invoices)
}

// CreateInvoice manually creates an invoice for a client.
//
// POST /api/v1/billing/invoices
func (h *BillingHandler) CreateInvoice(w http.ResponseWriter, r *http.Request) {
	var req models.CreateInvoiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "Invalid JSON body")
		return
	}

	payload, err := h.Service.CreateInvoice(r.Context(), req)
	if err != nil {
		if errors.Is(err, apierr.ErrNotFound) {
			response.NotFound(w, "Client")
			return
		}
		if errors.Is(err, apierr.ErrValidation) {
			response.BadRequest(w, err.Error())
			return
		}
		log.Printf("[ERROR] billing CreateInvoice failed: %v", err)
		response.InternalError(w)
		return
	}

	response.Created(w, payload)
}

// MarkPaid marks an invoice as paid.
//
// PATCH /api/v1/billing/invoices/:id/pay
func (h *BillingHandler) MarkPaid(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	err := h.Service.MarkPaid(r.Context(), id)
	if err != nil {
		if errors.Is(err, apierr.ErrNotFound) {
			response.NotFound(w, "Invoice (or already paid)")
			return
		}
		log.Printf("[ERROR] billing MarkPaid failed: %v", err)
		response.InternalError(w)
		return
	}

	response.Message(w, fmt.Sprintf("Invoice %s marked as paid", id))
}
