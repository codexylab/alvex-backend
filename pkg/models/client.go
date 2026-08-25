package models

import (
	"time"
)

// BillingPlan defines available subscription tiers.
type BillingPlan string

const (
	BillingBasic      BillingPlan = "Basic"
	BillingPro        BillingPlan = "Pro"
	BillingEnterprise BillingPlan = "Enterprise"
	BillingCustom     BillingPlan = "Custom"
)

// ClientStatus represents the operational state of a client.
type ClientStatus string

const (
	ClientStatusActive    ClientStatus = "Active"
	ClientStatusSuspended ClientStatus = "Suspended"
)

// AIProvider represents supported LLM providers.
type AIProvider string

const (
	ProviderGemini AIProvider = "Gemini"
	ProviderOpenAI AIProvider = "OpenAI"
	ProviderGroq   AIProvider = "Groq"
)

// ProviderModels maps each provider to its available model list.
// Gemini: Only Flash models are offered (2.0 Flash → 2.5 Flash). Pro/Ultra removed.
var ProviderModels = map[AIProvider][]string{
	ProviderGemini: {"Gemini 2.0 Flash", "Gemini 2.5 Flash"},
	ProviderOpenAI: {"OpenAI (GPT-4o)", "OpenAI (GPT-4-turbo)", "OpenAI (GPT-3.5-turbo)"},
	ProviderGroq:   {"Groq Llama-3 70B", "Groq Llama-3 8B", "Groq Mixtral 8x7B"},
}

// Client represents an AI assistant configuration for a business client.
type Client struct {
	ID                  string       `json:"id"`
	Name                string       `json:"name"`
	Domain              string       `json:"domain"`
	Status              ClientStatus `json:"status"`
	Provider            AIProvider   `json:"provider"`
	Model               string       `json:"model"`
	APIKey              string       `json:"api_key,omitempty"`       // masked in list views, decrypted in GetOne
	GeminiAPIKey        string       `json:"gemini_api_key,omitempty"` // Gemini provider-specific key
	GroqAPIKey          string       `json:"groq_api_key,omitempty"`   // Groq provider-specific key
	GroqFallbackEnabled bool         `json:"groq_fallback_enabled"`    // auto-fallback Groq→Gemini
	PortalToken         string       `json:"portal_token,omitempty"` // client portal access token
	SystemPersona       string       `json:"system_persona"`
	WebhookURL          string       `json:"webhook_url"`
	Temperature         float64      `json:"temperature"`
	StrictAdherence     bool         `json:"strict_adherence"`
	BillingPlan         BillingPlan  `json:"billing_plan"`
	CustomRate          *float64     `json:"custom_rate"`
	OwnerID             *string      `json:"owner_id,omitempty"`
	ScrapedContent      string       `json:"scraped_content,omitempty"`
	ScrapeSyncedAt      *time.Time   `json:"scrape_synced_at,omitempty"`
	ScrapeEnabled       bool         `json:"scrape_enabled"`
	ScrapeIntervalHours int          `json:"scrape_interval_hours"`
	WidgetChatEnabled   bool         `json:"widget_chat_enabled"`
	WidgetTicketingEnabled bool      `json:"widget_ticketing_enabled"`
	WidgetAdminMsgEnabled bool       `json:"widget_admin_msg_enabled"`
	WidgetImageSearchEnabled bool    `json:"widget_image_search_enabled"`
	WidgetTicketingAllowed bool      `json:"widget_ticketing_allowed"`
	WidgetAdminMsgAllowed bool       `json:"widget_admin_msg_allowed"`
	WidgetImageSearchAllowed bool    `json:"widget_image_search_allowed"`
	WidgetBrandName     string       `json:"widget_brand_name"`
	WidgetLogoURL       string       `json:"widget_logo_url"`
	WidgetPrimaryColor  string       `json:"widget_primary_color"`
	WidgetSecondaryColor string      `json:"widget_secondary_color"`
	WidgetRemoveBranding bool        `json:"widget_remove_branding"`
	WidgetBrandingAllowed bool       `json:"widget_branding_allowed"`
	GuardrailsEnabled   bool         `json:"guardrails_enabled"`
	GuardrailsReply     string       `json:"guardrails_reply"`
	ChatRetentionDays   int          `json:"chat_retention_days"`
	DailyMsgCount       int          `json:"daily_msg_count"`
	MsgCountDate        *string      `json:"msg_count_date,omitempty"`
	StripeCustomerID    string       `json:"stripe_customer_id,omitempty"`
	StripeSubscriptionID string      `json:"stripe_subscription_id,omitempty"`
	OnboardingStatus    string       `json:"onboarding_status"`
	CreatedAt           time.Time    `json:"created_at"`
	UpdatedAt           time.Time    `json:"updated_at"`
}

// MonthlyCost returns the monthly subscription cost based on billing plan.
func (c *Client) MonthlyCost() float64 {
	switch c.BillingPlan {
	case BillingEnterprise:
		return 499.00
	case BillingPro:
		return 99.00
	case BillingCustom:
		if c.CustomRate != nil {
			return *c.CustomRate
		}
		return 0.00
	default: // Basic
		return 29.00
	}
}

// ActiveAPIKeys returns the number of API keys for this client based on plan.
func (c *Client) ActiveAPIKeys() int {
	if c.Status != ClientStatusActive {
		return 0
	}
	if c.BillingPlan == BillingEnterprise {
		return 2
	}
	return 1
}

// MaskedAPIKey returns a partially masked version of the API key for display.
func (c *Client) MaskedAPIKey() string {
	if len(c.APIKey) < 9 {
		return "••••••••"
	}
	return c.APIKey[:9] + "••••"
}

// CreateClientRequest is the payload for POST /api/v1/clients.
type CreateClientRequest struct {
	Name        string      `json:"name"        validate:"required,min=2,max=255"`
	Domain      string      `json:"domain"      validate:"required"`
	Provider    AIProvider  `json:"provider"    validate:"required,oneof=Gemini OpenAI Groq"`
	Model       string      `json:"model"       validate:"required"`
	BillingPlan BillingPlan `json:"billing_plan" validate:"required,oneof=Basic Pro Enterprise Custom"`
}

// UpdateClientRequest is the payload for PUT /api/v1/clients/:id.
type UpdateClientRequest struct {
	Domain              *string      `json:"domain"`
	Provider            *AIProvider  `json:"provider"`
	Model               *string      `json:"model"`
	APIKey              *string      `json:"api_key"`
	GeminiAPIKey        *string      `json:"gemini_api_key"`
	GroqAPIKey          *string      `json:"groq_api_key"`
	GroqFallbackEnabled *bool        `json:"groq_fallback_enabled"`
	SystemPersona       *string      `json:"system_persona"`
	Temperature         *float64     `json:"temperature"       validate:"omitempty,min=0,max=1"`
	StrictAdherence     *bool        `json:"strict_adherence"`
	BillingPlan         *BillingPlan `json:"billing_plan"      validate:"omitempty,oneof=Basic Pro Enterprise Custom"`
	CustomRate          *float64     `json:"custom_rate"`
	ScrapeEnabled       *bool        `json:"scrape_enabled"`
	ScrapeIntervalHours *int         `json:"scrape_interval_hours"`
	WidgetChatEnabled   *bool     `json:"widget_chat_enabled"`
	WidgetTicketingEnabled *bool     `json:"widget_ticketing_enabled"`
	WidgetAdminMsgEnabled *bool      `json:"widget_admin_msg_enabled"`
	WidgetImageSearchEnabled *bool   `json:"widget_image_search_enabled"`
	WidgetTicketingAllowed *bool     `json:"widget_ticketing_allowed"`
	WidgetAdminMsgAllowed *bool      `json:"widget_admin_msg_allowed"`
	WidgetImageSearchAllowed *bool   `json:"widget_image_search_allowed"`
	WidgetBrandName        *string   `json:"widget_brand_name"`
	WidgetLogoURL          *string   `json:"widget_logo_url"`
	WidgetPrimaryColor     *string   `json:"widget_primary_color"`
	WidgetSecondaryColor   *string   `json:"widget_secondary_color"`
	WidgetRemoveBranding   *bool     `json:"widget_remove_branding"`
	WidgetBrandingAllowed  *bool     `json:"widget_branding_allowed"`
	GuardrailsEnabled      *bool     `json:"guardrails_enabled"`
	GuardrailsReply        *string   `json:"guardrails_reply"`
	ChatRetentionDays      *int      `json:"chat_retention_days"`
}

// ClientListResponse is a paginated list result for GET /api/v1/clients.
type ClientListResponse struct {
	Data       []Client `json:"data"`
	Total      int      `json:"total"`
	Page       int      `json:"page"`
	Limit      int      `json:"limit"`
	TotalPages int      `json:"total_pages"`
}

// UpdatePortalConfigRequest is the shared payload struct for client portal configuration updates.
type UpdatePortalConfigRequest struct {
	WidgetTicketingEnabled   *bool   `json:"widget_ticketing_enabled"`
	WidgetAdminMsgEnabled    *bool   `json:"widget_admin_msg_enabled"`
	WidgetImageSearchEnabled *bool   `json:"widget_image_search_enabled"`
	GuardrailsEnabled        *bool   `json:"guardrails_enabled"`
	GuardrailsReply          *string `json:"guardrails_reply"`
	ChatRetentionDays        *int    `json:"chat_retention_days"`
}
