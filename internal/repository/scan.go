package repository

import (
	"database/sql"

	"github.com/codexylab/alvex-backend/internal/database"
	"github.com/codexylab/alvex-backend/internal/models"
)

type scanner interface {
	Scan(dest ...interface{}) error
}

// scanClientRow scans a client row from rows/row.
// Reuses the exact same logic across client_repo and portal_repo to satisfy DRY.
func scanClientRow(s scanner, includePortalAndGuardrails bool, includeRetention bool) (*models.Client, error) {
	c := &models.Client{}
	var ownerID             sql.NullString
	var portalToken         sql.NullString
	var geminiAPIKeyRaw     sql.NullString
	var groqAPIKeyRaw       sql.NullString
	var groqFallbackEnabled sql.NullBool
	var scrapedContent      sql.NullString
	var scrapeSyncedAt      database.NullTime
	var scrapeEnabled       sql.NullBool
	var scrapeIntervalHours sql.NullInt64
	var widgetChatEnabled, widgetTicketingEnabled, widgetAdminMsgEnabled, widgetImageSearchEnabled sql.NullBool
	var widgetTicketingAllowed, widgetAdminMsgAllowed, widgetImageSearchAllowed sql.NullBool
	var widgetBrandName, widgetLogoURL, widgetPrimaryColor, widgetSecondaryColor sql.NullString
	var widgetRemoveBranding, widgetBrandingAllowed sql.NullBool
	var guardrailsEnabled   sql.NullInt64
	var guardrailsReply     sql.NullString
	var chatRetentionDays   sql.NullInt64

	dest := []interface{}{
		&c.ID, &c.Name, &c.Domain, &c.Status, &c.Provider, &c.Model,
		&c.APIKey, &c.SystemPersona, &c.WebhookURL, &c.Temperature,
		&c.StrictAdherence, &c.BillingPlan, &c.CustomRate,
		&geminiAPIKeyRaw, &groqAPIKeyRaw, &groqFallbackEnabled,
		&scrapedContent, &scrapeSyncedAt, &scrapeEnabled, &scrapeIntervalHours,
		&widgetChatEnabled, &widgetTicketingEnabled, &widgetAdminMsgEnabled, &widgetImageSearchEnabled,
		&widgetTicketingAllowed, &widgetAdminMsgAllowed, &widgetImageSearchAllowed,
		&widgetBrandName, &widgetLogoURL, &widgetPrimaryColor, &widgetSecondaryColor, &widgetRemoveBranding, &widgetBrandingAllowed,
	}

	if includePortalAndGuardrails {
		dest = append(dest, &portalToken, &ownerID, &guardrailsEnabled, &guardrailsReply)
	}
	if includeRetention {
		dest = append(dest, &chatRetentionDays)
	}

	dest = append(dest, &c.CreatedAt, &c.UpdatedAt)

	if err := s.Scan(dest...); err != nil {
		return nil, err
	}

	if geminiAPIKeyRaw.Valid {
		c.GeminiAPIKey = geminiAPIKeyRaw.String
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

	if includePortalAndGuardrails {
		if ownerID.Valid {
			c.OwnerID = &ownerID.String
		}
		if portalToken.Valid {
			c.PortalToken = portalToken.String
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
