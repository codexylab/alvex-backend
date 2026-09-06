package repository

import (
	"database/sql"
	"encoding/json"

	"github.com/codexylab/alvex-backend/pkg/database"
	"github.com/codexylab/alvex-backend/pkg/models"
)

type scanner interface {
	Scan(dest ...interface{}) error
}

// scanClientRow scans a client row from rows/row.
// Reuses the exact same logic across client_repo and portal_repo to satisfy DRY.
func scanClientRow(s scanner, includeOwnerAndGuardrails bool, includeRetention bool) (*models.Client, error) {
	c := &models.Client{}
	var ownerID sql.NullString
	var allowedOriginsJSON string
	var whatsAppPhoneNumberID sql.NullString
	var openAIAPIKeyRaw sql.NullString
	var geminiAPIKeyRaw sql.NullString
	var groqAPIKeyRaw sql.NullString
	var groqFallbackEnabled sql.NullBool
	var scrapedContent sql.NullString
	var scrapeSyncedAt database.NullTime
	var scrapeEnabled sql.NullBool
	var scrapeIntervalHours sql.NullInt64
	var widgetChatEnabled, widgetTicketingEnabled, widgetAdminMsgEnabled, widgetImageSearchEnabled sql.NullBool
	var widgetTicketingAllowed, widgetAdminMsgAllowed, widgetImageSearchAllowed sql.NullBool
	var widgetBrandName, widgetLogoURL, widgetPrimaryColor, widgetSecondaryColor sql.NullString
	var widgetRemoveBranding, widgetBrandingAllowed sql.NullBool
	var guardrailsEnabled sql.NullInt64
	var guardrailsReply sql.NullString
	var chatRetentionDays sql.NullInt64

	dest := []interface{}{
		&c.ID, &c.OrganizationID, &c.Name, &c.Domain, &allowedOriginsJSON, &whatsAppPhoneNumberID,
		&c.Status, &c.Provider, &c.Model,
		&c.SystemPersona, &c.WebhookURL, &c.Temperature,
		&c.StrictAdherence, &c.BillingPlan, &c.CustomRate,
		&openAIAPIKeyRaw, &geminiAPIKeyRaw, &groqAPIKeyRaw, &groqFallbackEnabled,
		&scrapedContent, &scrapeSyncedAt, &scrapeEnabled, &scrapeIntervalHours,
		&widgetChatEnabled, &widgetTicketingEnabled, &widgetAdminMsgEnabled, &widgetImageSearchEnabled,
		&widgetTicketingAllowed, &widgetAdminMsgAllowed, &widgetImageSearchAllowed,
		&widgetBrandName, &widgetLogoURL, &widgetPrimaryColor, &widgetSecondaryColor, &widgetRemoveBranding, &widgetBrandingAllowed,
	}

	if includeOwnerAndGuardrails {
		dest = append(dest, &ownerID, &guardrailsEnabled, &guardrailsReply)
	}
	if includeRetention {
		dest = append(dest, &chatRetentionDays)
	}

	dest = append(dest, &c.CreatedAt, &c.UpdatedAt)

	if err := s.Scan(dest...); err != nil {
		return nil, err
	}

	if openAIAPIKeyRaw.Valid {
		c.OpenAIAPIKey = openAIAPIKeyRaw.String
	}
	if geminiAPIKeyRaw.Valid {
		c.GeminiAPIKey = geminiAPIKeyRaw.String
	}
	if err := json.Unmarshal([]byte(allowedOriginsJSON), &c.AllowedOrigins); err != nil {
		return nil, err
	}
	if whatsAppPhoneNumberID.Valid {
		c.WhatsAppPhoneNumberID = whatsAppPhoneNumberID.String
	}
	if groqAPIKeyRaw.Valid {
		c.GroqAPIKey = groqAPIKeyRaw.String
	}
	if groqFallbackEnabled.Valid {
		c.GroqFallbackEnabled = groqFallbackEnabled.Bool
	}
	if scrapedContent.Valid {
		c.ScrapedContent = scrapedContent.String
	}
	if scrapeSyncedAt.Valid {
		c.ScrapeSyncedAt = &scrapeSyncedAt.Time
	}
	if scrapeEnabled.Valid {
		c.ScrapeEnabled = scrapeEnabled.Bool
	}
	if scrapeIntervalHours.Valid {
		c.ScrapeIntervalHours = int(scrapeIntervalHours.Int64)
	}
	if widgetChatEnabled.Valid {
		c.WidgetChatEnabled = widgetChatEnabled.Bool
	}
	if widgetTicketingEnabled.Valid {
		c.WidgetTicketingEnabled = widgetTicketingEnabled.Bool
	}
	if widgetAdminMsgEnabled.Valid {
		c.WidgetAdminMsgEnabled = widgetAdminMsgEnabled.Bool
	}
	if widgetImageSearchEnabled.Valid {
		c.WidgetImageSearchEnabled = widgetImageSearchEnabled.Bool
	}
	if widgetTicketingAllowed.Valid {
		c.WidgetTicketingAllowed = widgetTicketingAllowed.Bool
	}
	if widgetAdminMsgAllowed.Valid {
		c.WidgetAdminMsgAllowed = widgetAdminMsgAllowed.Bool
	}
	if widgetImageSearchAllowed.Valid {
		c.WidgetImageSearchAllowed = widgetImageSearchAllowed.Bool
	}
	if widgetBrandName.Valid {
		c.WidgetBrandName = widgetBrandName.String
	}
	if widgetLogoURL.Valid {
		c.WidgetLogoURL = widgetLogoURL.String
	}
	if widgetPrimaryColor.Valid {
		c.WidgetPrimaryColor = widgetPrimaryColor.String
	}
	if widgetSecondaryColor.Valid {
		c.WidgetSecondaryColor = widgetSecondaryColor.String
	}
	if widgetRemoveBranding.Valid {
		c.WidgetRemoveBranding = widgetRemoveBranding.Bool
	}
	if widgetBrandingAllowed.Valid {
		c.WidgetBrandingAllowed = widgetBrandingAllowed.Bool
	}

	if includeOwnerAndGuardrails {
		if ownerID.Valid {
			c.OwnerID = &ownerID.String
		}
		if guardrailsEnabled.Valid {
			c.GuardrailsEnabled = guardrailsEnabled.Int64 == 1
		}
		if guardrailsReply.Valid {
			c.GuardrailsReply = guardrailsReply.String
		}
	}

	if includeRetention {
		if chatRetentionDays.Valid {
			c.ChatRetentionDays = int(chatRetentionDays.Int64)
		} else {
			c.ChatRetentionDays = 30
		}
	}

	return c, nil
}

// MarshalStringSlice serializes a string slice for JSON/JSONB persistence.
func MarshalStringSlice(values []string) string {
	if values == nil {
		values = []string{}
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

// boolToSQL converts a bool to sqlite-compatible int (1/0) or postgres-compatible bool.
func boolToSQL(db *database.DB, v bool) interface{} {
	if db.IsSQLite() {
		if v {
			return 1
		}
		return 0
	}
	return v
}

// parseBoolValue parses interface (from DB scan) into a go bool.
func parseBoolValue(v interface{}) bool {
	switch val := v.(type) {
	case int64:
		return val == 1
	case int:
		return val == 1
	case bool:
		return val
	}
	return false
}
