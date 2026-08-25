package handlers

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/codexylab/alvex-backend/pkg/middleware"
	"github.com/codexylab/alvex-backend/pkg/models"
	"github.com/codexylab/alvex-backend/pkg/services"
	"github.com/codexylab/alvex-backend/pkg/apierr"
	"github.com/codexylab/alvex-backend/pkg/response"
)

// ClientPortalHandler serves the client-facing portal API.
type ClientPortalHandler struct {
	Service  *services.PortalService
	IsSQLite bool // reflects the active database driver; used for DB-specific query variants
}

// Me returns the authenticated client's own profile.
//
// GET /client-portal/me
func (h *ClientPortalHandler) Me(w http.ResponseWriter, r *http.Request) {
	clientID := middleware.GetPortalClientID(r)

	c, err := h.Service.GetClientProfile(r.Context(), clientID)
	if err != nil {
		response.NotFound(w, "Client")
		return
	}

	p := models.PortalClientProfile{
		ID:                       c.ID,
		Name:                     c.Name,
		Domain:                   c.Domain,
		Status:                   string(c.Status),
		Provider:                 string(c.Provider),
		Model:                    c.Model,
		WebhookURL:               c.WebhookURL,
		BillingPlan:              string(c.BillingPlan),
		WidgetChatEnabled:        c.WidgetChatEnabled,
		WidgetTicketingEnabled:   c.WidgetTicketingEnabled,
		WidgetAdminMsgEnabled:    c.WidgetAdminMsgEnabled,
		WidgetImageSearchEnabled: c.WidgetImageSearchEnabled,
		WidgetTicketingAllowed:   c.WidgetTicketingAllowed,
		WidgetAdminMsgAllowed:    c.WidgetAdminMsgAllowed,
		WidgetImageSearchAllowed: c.WidgetImageSearchAllowed,
		WidgetBrandName:          c.WidgetBrandName,
		WidgetLogoURL:            c.WidgetLogoURL,
		WidgetPrimaryColor:       c.WidgetPrimaryColor,
		WidgetSecondaryColor:     c.WidgetSecondaryColor,
		WidgetRemoveBranding:     c.WidgetRemoveBranding,
		WidgetBrandingAllowed:    c.WidgetBrandingAllowed,
		GuardrailsEnabled:        c.GuardrailsEnabled,
		GuardrailsReply:          c.GuardrailsReply,
		ChatRetentionDays:        c.ChatRetentionDays,
		CreatedAt:                c.CreatedAt,
	}

	response.Success(w, p)
}

// Stats returns client-scoped analytics (only this client's data).
//
// GET /client-portal/stats
func (h *ClientPortalHandler) Stats(w http.ResponseWriter, r *http.Request) {
	clientID := middleware.GetPortalClientID(r)

	// Delegate DB-dialect-specific query formatting to the service layer via the driver flag.
	isSQLite := h.IsSQLite

	data, err := h.Service.GetStats(r.Context(), clientID, isSQLite)
	if err != nil {
		log.Printf("[ERROR] client portal Stats failed: %v", err)
		response.InternalError(w)
		return
	}

	stats := models.PortalStatsView{
		TotalConversations: data.TotalConversations,
		AvgResponseMs:      data.AvgLatency,
		PendingInvoice:     data.PendingInvoice,
	}

	if data.TotalConversations > 0 {
		rate := float64(data.TotalConversations-data.FailedConversations) / float64(data.TotalConversations) * 100
		stats.SuccessRate = fmt.Sprintf("%.1f%%", rate)
	} else {
		stats.SuccessRate = "100.0%"
	}

	stats.ThisMonthConvs = data.ThisMonthConvs
	if !data.ActiveSince.IsZero() {
		stats.ActiveSince = data.ActiveSince.Format("Jan 2006")
	}

	response.Success(w, stats)
}

// Conversations returns a paginated list of this client's conversation transcripts.
//
// GET /client-portal/conversations?page=1&limit=20&search=hello
func (h *ClientPortalHandler) Conversations(w http.ResponseWriter, r *http.Request) {
	clientID := middleware.GetPortalClientID(r)

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	entries, total, err := h.Service.GetConversations(r.Context(), clientID, page, limit)
	if err != nil {
		log.Printf("[ERROR] client portal Conversations failed: %v", err)
		response.InternalError(w)
		return
	}

	portalEntries := []models.PortalConversationEntry{}
	for _, e := range entries {
		portalEntries = append(portalEntries, models.PortalConversationEntry{
			ID:        e.ID,
			UserRef:   e.UserRef,
			Message:   e.Message,
			AIReply:   e.AIResponse,
			Channel:   string(e.Channel),
			Status:    string(e.Status),
			LatencyMs: e.LatencyMs,
			CreatedAt: e.CreatedAt,
		})
	}

	totalPages := (total + limit - 1) / limit
	if totalPages == 0 {
		totalPages = 1
	}

	response.Success(w, map[string]interface{}{
		"data":        portalEntries,
		"total":       total,
		"page":        page,
		"limit":       limit,
		"total_pages": totalPages,
	})
}

// BotInfo returns this client's bot configuration (read-only, no secret fields).
//
// GET /client-portal/bot
func (h *ClientPortalHandler) BotInfo(w http.ResponseWriter, r *http.Request) {
	clientID := middleware.GetPortalClientID(r)

	c, err := h.Service.GetClientProfile(r.Context(), clientID)
	if err != nil {
		response.NotFound(w, "Client")
		return
	}

	info := models.PortalBotInfo{
		Provider:       string(c.Provider),
		Model:          c.Model,
		WebhookURL:     c.WebhookURL,
		SystemPersona:  c.SystemPersona,
		Temperature:    c.Temperature,
		ScrapeSyncedAt: c.ScrapeSyncedAt,
	}

	response.Success(w, info)
}

// Billing returns this client's current plan and invoice history.
//
// GET /client-portal/billing
func (h *ClientPortalHandler) Billing(w http.ResponseWriter, r *http.Request) {
	clientID := middleware.GetPortalClientID(r)

	c, err := h.Service.GetClientProfile(r.Context(), clientID)
	if err != nil {
		response.NotFound(w, "Client")
		return
	}

	clientInvoices, err := h.Service.GetClientInvoices(r.Context(), clientID)
	if err != nil {
		response.InternalError(w)
		return
	}

	invoices := []models.PortalInvoiceView{}
	for _, inv := range clientInvoices {
		dueDateStr := ""
		if inv.DueDate != nil {
			dueDateStr = inv.DueDate.Format("2006-01-02")
		}
		invoices = append(invoices, models.PortalInvoiceView{
			ID:        inv.ID,
			Amount:    inv.Amount,
			Status:    string(inv.Status),
			DueDate:   dueDateStr,
			CreatedAt: inv.CreatedAt,
		})
	}

	response.Success(w, models.PortalBillingResponse{
		Plan:       string(c.BillingPlan),
		CustomRate: c.CustomRate,
		Invoices:   invoices,
	})
}

// ExportConversations downloads this client's conversations as a CSV file.
//
// GET /client-portal/export
func (h *ClientPortalHandler) ExportConversations(w http.ResponseWriter, r *http.Request) {
	clientID := middleware.GetPortalClientID(r)

	filename := fmt.Sprintf("conversations_%s.csv", time.Now().Format("2006-01-02"))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	cw := csv.NewWriter(w)
	defer cw.Flush()

	cw.Write([]string{"ID", "User", "Channel", "Message", "AI Reply", "Status", "Latency (ms)", "Date"})

	logs, err := h.Service.GetClientActivityLogs(r.Context(), clientID)
	if err != nil {
		slog.Warn("failed to fetch activity logs for export", "client_id", clientID, "error", err)
	}
	for _, log := range logs {
		cw.Write([]string{
			log.ID, log.UserRef, string(log.Channel), log.Message, log.AIResponse, string(log.Status),
			fmt.Sprintf("%d", log.LatencyMs),
			log.CreatedAt.Format(time.RFC3339),
		})
	}
}

// ReplyToConversation handles client replying to a ticket.
//
// POST /api/v1/client-portal/conversations/{id}/reply
func (h *ClientPortalHandler) ReplyToConversation(w http.ResponseWriter, r *http.Request) {
	clientID := middleware.GetPortalClientID(r)
	id := chi.URLParam(r, "id")

	var req models.ReplyToConversationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "Invalid JSON body")
		return
	}

	err := h.Service.ReplyToConversation(r.Context(), id, clientID, req.Reply)
	if err != nil {
		if errors.Is(err, apierr.ErrCannotBeEmpty) {
			response.BadRequest(w, err.Error())
			return
		}
		response.NotFound(w, "Conversation or Ticket")
		return
	}

	response.Success(w, map[string]string{"message": "Reply saved successfully"})
}

// UpdateConfig updates client configurations from portal view.
// PUT /api/v1/client-portal/config
func (h *ClientPortalHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	clientID := middleware.GetPortalClientID(r)

	var req models.UpdatePortalConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "Invalid JSON body")
		return
	}

	err := h.Service.UpdateConfig(r.Context(), clientID, req)
	if err != nil {
		response.NotFound(w, "Client")
		return
	}

	response.Success(w, map[string]string{"message": "Widget configuration updated successfully"})
}

// GetFAQs retrieves all FAQs for the authenticated client.
//
// GET /api/v1/client-portal/faqs
func (h *ClientPortalHandler) GetFAQs(w http.ResponseWriter, r *http.Request) {
	clientID := middleware.GetPortalClientID(r)

	faqs, err := h.Service.GetFAQs(r.Context(), clientID)
	if err != nil {
		response.InternalError(w)
		return
	}
	response.Success(w, faqs)
}

// CreateFAQ creates a new FAQ.
//
// POST /api/v1/client-portal/faqs
func (h *ClientPortalHandler) CreateFAQ(w http.ResponseWriter, r *http.Request) {
	clientID := middleware.GetPortalClientID(r)

	var req models.CreateFAQRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "Invalid JSON body")
		return
	}

	payload, err := h.Service.CreateFAQ(r.Context(), clientID, req)
	if err != nil {
		if errors.Is(err, apierr.ErrValidation) {
			response.BadRequest(w, err.Error())
			return
		}
		response.BadRequest(w, err.Error())
		return
	}

	response.Success(w, payload)
}

// UpdateFAQ updates an FAQ.
//
// PUT /api/v1/client-portal/faqs/{id}
func (h *ClientPortalHandler) UpdateFAQ(w http.ResponseWriter, r *http.Request) {
	clientID := middleware.GetPortalClientID(r)
	faqID := chi.URLParam(r, "id")

	var req models.CreateFAQRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "Invalid JSON body")
		return
	}

	payload, err := h.Service.UpdateFAQ(r.Context(), faqID, clientID, req)
	if err != nil {
		if errors.Is(err, apierr.ErrValidation) {
			response.BadRequest(w, err.Error())
			return
		}
		if errors.Is(err, apierr.ErrNotFound) {
			response.NotFound(w, "FAQ")
			return
		}
		response.InternalError(w)
		return
	}

	response.Success(w, payload)
}

// DeleteFAQ deletes an FAQ.
//
// DELETE /api/v1/client-portal/faqs/{id}
func (h *ClientPortalHandler) DeleteFAQ(w http.ResponseWriter, r *http.Request) {
	clientID := middleware.GetPortalClientID(r)
	faqID := chi.URLParam(r, "id")

	err := h.Service.DeleteFAQ(r.Context(), faqID, clientID)
	if err != nil {
		if errors.Is(err, apierr.ErrNotFound) {
			response.NotFound(w, "FAQ")
			return
		}
		response.InternalError(w)
		return
	}

	response.Success(w, map[string]string{"message": "FAQ deleted successfully"})
}

// GenerateFAQs manually triggers AI FAQ extraction from synced content.
//
// POST /api/v1/client-portal/faqs/generate
func (h *ClientPortalHandler) GenerateFAQs(w http.ResponseWriter, r *http.Request) {
	clientID := middleware.GetPortalClientID(r)

	err := h.Service.GenerateFAQs(r.Context(), clientID)
	if err != nil {
		if errors.Is(err, apierr.ErrNotFound) {
			response.NotFound(w, "Client")
			return
		}
		response.BadRequest(w, "FAQ Generation failed: "+err.Error())
		return
	}

	response.Success(w, map[string]string{"message": "FAQs generated successfully"})
}
