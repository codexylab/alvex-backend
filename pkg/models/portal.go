package models

import "time"

// PortalClientProfile is the read-only view of a client's own profile
// returned by GET /client-portal/me.
type PortalClientProfile struct {
	ID                       string    `json:"id"`
	Name                     string    `json:"name"`
	Domain                   string    `json:"domain"`
	Status                   string    `json:"status"`
	Provider                 string    `json:"provider"`
	Model                    string    `json:"model"`
	WebhookURL               string    `json:"webhook_url"`
	BillingPlan              string    `json:"billing_plan"`
	WidgetChatEnabled        bool      `json:"widget_chat_enabled"`
	WidgetTicketingEnabled   bool      `json:"widget_ticketing_enabled"`
	WidgetAdminMsgEnabled    bool      `json:"widget_admin_msg_enabled"`
	WidgetImageSearchEnabled bool      `json:"widget_image_search_enabled"`
	WidgetTicketingAllowed   bool      `json:"widget_ticketing_allowed"`
	WidgetAdminMsgAllowed    bool      `json:"widget_admin_msg_allowed"`
	WidgetImageSearchAllowed bool      `json:"widget_image_search_allowed"`
	WidgetBrandName          string    `json:"widget_brand_name"`
	WidgetLogoURL            string    `json:"widget_logo_url"`
	WidgetPrimaryColor       string    `json:"widget_primary_color"`
	WidgetSecondaryColor     string    `json:"widget_secondary_color"`
	WidgetRemoveBranding     bool      `json:"widget_remove_branding"`
	WidgetBrandingAllowed    bool      `json:"widget_branding_allowed"`
	GuardrailsEnabled        bool      `json:"guardrails_enabled"`
	GuardrailsReply          string    `json:"guardrails_reply"`
	ChatRetentionDays        int       `json:"chat_retention_days"`
	CreatedAt                time.Time `json:"created_at"`
}

// PortalStatsView is the analytics overview returned by GET /client-portal/stats.
type PortalStatsView struct {
	TotalConversations int     `json:"total_conversations"`
	ThisMonthConvs     int     `json:"this_month_conversations"`
	SuccessRate        string  `json:"success_rate"`
	AvgResponseMs      float64 `json:"avg_response_ms"`
	PendingInvoice     float64 `json:"pending_invoice"`
	ActiveSince        string  `json:"active_since"`
}

// PortalConversationEntry is a single conversation record in the portal feed
// returned by GET /client-portal/conversations.
type PortalConversationEntry struct {
	ID        string    `json:"id"`
	UserRef   string    `json:"user"`
	Message   string    `json:"message"`
	AIReply   string    `json:"ai_reply"`
	Channel   string    `json:"channel"`
	Status    string    `json:"status"`
	LatencyMs int64     `json:"latency_ms"`
	CreatedAt time.Time `json:"created_at"`
}

// PortalBotInfo is the read-only view of the client's AI bot configuration
// returned by GET /client-portal/bot.
type PortalBotInfo struct {
	Provider       string     `json:"provider"`
	Model          string     `json:"model"`
	WebhookURL     string     `json:"webhook_url"`
	SystemPersona  string     `json:"system_persona"`
	Temperature    float64    `json:"temperature"`
	ScrapeSyncedAt *time.Time `json:"scrape_synced_at,omitempty"`
}

// PortalInvoiceView is a simplified invoice view for the client portal
// returned by GET /client-portal/billing.
type PortalInvoiceView struct {
	ID        string    `json:"id"`
	Amount    float64   `json:"amount"`
	Status    string    `json:"status"`
	DueDate   string    `json:"due_date"`
	CreatedAt time.Time `json:"created_at"`
}

// PortalBillingResponse bundles the client's plan with their invoice history
// returned by GET /client-portal/billing.
type PortalBillingResponse struct {
	Plan       string              `json:"plan"`
	CustomRate *float64            `json:"custom_rate"`
	Invoices   []PortalInvoiceView `json:"invoices"`
}

// ReplyToConversationRequest is the request body for POST /client-portal/conversations/{id}/reply.
type ReplyToConversationRequest struct {
	Reply string `json:"reply"`
}
