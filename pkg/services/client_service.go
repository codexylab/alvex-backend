package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/codexylab/alvex-backend/pkg/models"
	"github.com/codexylab/alvex-backend/pkg/repository"
	"github.com/codexylab/alvex-backend/pkg/services/scraper"
	"github.com/codexylab/alvex-backend/pkg/apierr"
	"github.com/codexylab/alvex-backend/pkg/crypto"
)

// ClientService manages the business logic for client configuration.
type ClientService struct {
	Repo          repository.ClientRepository
	EncryptionKey string
}

// NewClientService creates a new ClientService instance.
func NewClientService(repo repository.ClientRepository, encKey string) *ClientService {
	return &ClientService{
		Repo:          repo,
		EncryptionKey: encKey,
	}
}

// List returns a paginated list of client profiles.
func (s *ClientService) List(ctx context.Context, search, status string, page, limit int) ([]models.Client, int, error) {
	return s.Repo.List(ctx, search, status, page, limit)
}

// GetByID returns a single client profile with its decrypted main API key.
func (s *ClientService) GetByID(ctx context.Context, id string) (*models.Client, error) {
	c, err := s.Repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Decrypt the main API key
	if s.EncryptionKey != "" && c.APIKey != "" {
		c.APIKey = crypto.DecryptAPIKey(s.EncryptionKey, c.APIKey)
	}

	// Decrypt gemini_api_key
	if c.GeminiAPIKey != "" {
		if s.EncryptionKey != "" {
			c.GeminiAPIKey = crypto.DecryptAPIKey(s.EncryptionKey, c.GeminiAPIKey)
		}
	}

	// Decrypt groq_api_key
	if c.GroqAPIKey != "" {
		if s.EncryptionKey != "" {
			c.GroqAPIKey = crypto.DecryptAPIKey(s.EncryptionKey, c.GroqAPIKey)
		}
	}

	return c, nil
}

// Create generates credentials and saves a new client profile.
func (s *ClientService) Create(ctx context.Context, req models.CreateClientRequest, ownerID string) (*models.Client, error) {
	if req.Name == "" || req.Domain == "" {
		return nil, fmt.Errorf("name and domain are required")
	}

	// Generate a URL-safe slug ID from the client name
	id := crypto.SlugifyClientName(req.Name)
	if id == "" {
		return nil, fmt.Errorf("could not generate a valid ID from the client name")
	}

	// Check duplicates
	exists, err := s.Repo.ExistsByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("duplicate: client with ID %q already exists", id)
	}

	// Validate model
	validModels := models.ProviderModels[req.Provider]
	if !containsString(validModels, req.Model) {
		return nil, fmt.Errorf("model %q is not valid for provider %q", req.Model, req.Provider)
	}

	apiKey := crypto.GenerateAPIKey(req.Name)
	webhookURL := crypto.GenerateWebhookURL(id)
	portalToken := crypto.GeneratePortalToken()
	systemPersona := fmt.Sprintf(
		"You are an AI representative for %s. Help visitors with inquiries on %s.\n\nTONE: Friendly and professional.",
		req.Name, req.Domain,
	)

	// Encrypt API key
	apiKeyToStore := apiKey
	if s.EncryptionKey != "" {
		if encrypted, err := crypto.EncryptAPIKey(s.EncryptionKey, apiKey); err == nil {
			apiKeyToStore = encrypted
		}
	}

	c := &models.Client{
		ID:                  id,
		Name:                req.Name,
		Domain:              req.Domain,
		Status:              models.ClientStatusActive,
		Provider:            req.Provider,
		Model:               req.Model,
		APIKey:              apiKeyToStore,
		SystemPersona:       systemPersona,
		WebhookURL:          webhookURL,
		Temperature:         0.7,
		StrictAdherence:     true,
		BillingPlan:         req.BillingPlan,
		PortalToken:         portalToken,
		OwnerID:             &ownerID,
		WidgetChatEnabled:   true,
		WidgetTicketingEnabled: true,
		WidgetAdminMsgEnabled:   true,
		WidgetImageSearchEnabled: true,
		WidgetTicketingAllowed:   true,
		WidgetAdminMsgAllowed:     true,
		WidgetImageSearchAllowed:   true,
	}

	if err := s.Repo.Create(ctx, c); err != nil {
		return nil, err
	}

	// Fetch fresh copy (which decrypts fields correctly)
	return s.GetByID(ctx, id)
}

// Update compiles configuration changes, encrypts secrets, and updates the client profile.
func (s *ClientService) Update(ctx context.Context, id string, req models.UpdateClientRequest) (*models.Client, string, error) {
	var domainToSync string
	if req.Domain != nil && *req.Domain != "" {
		domainToSync = *req.Domain
	}

	fields := map[string]interface{}{
		"updated_at": time.Now(),
	}

	if req.Domain != nil {
		fields["domain"] = *req.Domain
	}
	if req.Provider != nil {
		fields["provider"] = *req.Provider
	}
	if req.Model != nil {
		fields["model"] = *req.Model
	}
	if req.APIKey != nil {
		keyToStore := *req.APIKey
		if s.EncryptionKey != "" {
			if enc, err := crypto.EncryptAPIKey(s.EncryptionKey, *req.APIKey); err == nil {
				keyToStore = enc
			}
		}
		fields["api_key"] = keyToStore
	}
	if req.GeminiAPIKey != nil {
		keyToStore := *req.GeminiAPIKey
		if s.EncryptionKey != "" {
			if enc, err := crypto.EncryptAPIKey(s.EncryptionKey, *req.GeminiAPIKey); err == nil {
				keyToStore = enc
			}
		}
		fields["gemini_api_key"] = keyToStore
	}
	if req.GroqAPIKey != nil {
		keyToStore := *req.GroqAPIKey
		if s.EncryptionKey != "" {
			if enc, err := crypto.EncryptAPIKey(s.EncryptionKey, *req.GroqAPIKey); err == nil {
				keyToStore = enc
			}
		}
		fields["groq_api_key"] = keyToStore
	}
	if req.GroqFallbackEnabled != nil {
		fields["groq_fallback_enabled"] = *req.GroqFallbackEnabled
	}
	if req.SystemPersona != nil {
		fields["system_persona"] = *req.SystemPersona
	}
	if req.Temperature != nil {
		fields["temperature"] = *req.Temperature
	}
	if req.StrictAdherence != nil {
		fields["strict_adherence"] = *req.StrictAdherence
	}
	if req.BillingPlan != nil {
		fields["billing_plan"] = *req.BillingPlan
	}
	if req.CustomRate != nil {
		fields["custom_rate"] = *req.CustomRate
	} else if req.BillingPlan != nil && *req.BillingPlan != models.BillingCustom {
		fields["custom_rate"] = nil
	}
	if req.ScrapeEnabled != nil {
		fields["scrape_enabled"] = *req.ScrapeEnabled
	}
	if req.ScrapeIntervalHours != nil {
		fields["scrape_interval_hours"] = *req.ScrapeIntervalHours
	}
	if req.WidgetChatEnabled != nil {
		fields["widget_chat_enabled"] = *req.WidgetChatEnabled
	}
	if req.WidgetTicketingEnabled != nil {
		fields["widget_ticketing_enabled"] = *req.WidgetTicketingEnabled
	}
	if req.WidgetAdminMsgEnabled != nil {
		fields["widget_admin_msg_enabled"] = *req.WidgetAdminMsgEnabled
	}
	if req.WidgetImageSearchEnabled != nil {
		fields["widget_image_search_enabled"] = *req.WidgetImageSearchEnabled
	}
	if req.WidgetTicketingAllowed != nil {
		fields["widget_ticketing_allowed"] = *req.WidgetTicketingAllowed
	}
	if req.WidgetAdminMsgAllowed != nil {
		fields["widget_admin_msg_allowed"] = *req.WidgetAdminMsgAllowed
	}
	if req.WidgetImageSearchAllowed != nil {
		fields["widget_image_search_allowed"] = *req.WidgetImageSearchAllowed
	}
	if req.WidgetBrandName != nil {
		fields["widget_brand_name"] = *req.WidgetBrandName
	}
	if req.WidgetLogoURL != nil {
		fields["widget_logo_url"] = *req.WidgetLogoURL
	}
	if req.WidgetPrimaryColor != nil {
		fields["widget_primary_color"] = *req.WidgetPrimaryColor
	}
	if req.WidgetSecondaryColor != nil {
		fields["widget_secondary_color"] = *req.WidgetSecondaryColor
	}
	if req.WidgetRemoveBranding != nil {
		fields["widget_remove_branding"] = *req.WidgetRemoveBranding
	}
	if req.WidgetBrandingAllowed != nil {
		fields["widget_branding_allowed"] = *req.WidgetBrandingAllowed
	}
	if req.GuardrailsEnabled != nil {
		val := 0
		if *req.GuardrailsEnabled {
			val = 1
		}
		fields["guardrails_enabled"] = val
	}
	if req.GuardrailsReply != nil {
		fields["guardrails_reply"] = *req.GuardrailsReply
	}

	rowsAffected, err := s.Repo.UpdateFields(ctx, id, fields)
	if err != nil {
		return nil, "", err
	}
	if rowsAffected == 0 {
		return nil, "", fmt.Errorf("%w: client not found", apierr.ErrNotFound)
	}

	c, err := s.GetByID(ctx, id)
	return c, domainToSync, err
}

// ToggleStatus toggles client account status between Active and Suspended.
func (s *ClientService) ToggleStatus(ctx context.Context, id string) (string, error) {
	c, err := s.Repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "no rows") {
			return "", fmt.Errorf("%w: client not found", apierr.ErrNotFound)
		}
		return "", err
	}

	newStatus := models.ClientStatusActive
	if c.Status == models.ClientStatusActive {
		newStatus = models.ClientStatusSuspended
	}

	fields := map[string]interface{}{
		"status":     newStatus,
		"updated_at": time.Now(),
	}

	_, err = s.Repo.UpdateFields(ctx, id, fields)
	if err != nil {
		return "", err
	}

	return string(newStatus), nil
}

// Delete deletes a suspended client.
func (s *ClientService) Delete(ctx context.Context, id string) error {
	c, err := s.Repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "no rows") {
			return fmt.Errorf("%w: client not found", apierr.ErrNotFound)
		}
		return err
	}

	if c.Status == models.ClientStatusActive {
		return fmt.Errorf("%w: active client cannot be deleted", apierr.ErrActiveClient)
	}

	rowsAffected, err := s.Repo.Delete(ctx, id)
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("%w: client not found", apierr.ErrNotFound)
	}
	return nil
}

// RotateToken regenerates the secure access portal token.
func (s *ClientService) RotateToken(ctx context.Context, id string) (*models.Client, error) {
	exists, err := s.Repo.ExistsByID(ctx, id)
	if err != nil || !exists {
		return nil, fmt.Errorf("%w: client not found", apierr.ErrNotFound)
	}

	newToken := crypto.GeneratePortalToken()
	fields := map[string]interface{}{
		"portal_token": newToken,
		"updated_at":   time.Now(),
	}

	_, err = s.Repo.UpdateFields(ctx, id, fields)
	if err != nil {
		return nil, err
	}

	return s.GetByID(ctx, id)
}

// RotateAPIKey generates a new platform API key for the client, invalidating the old one.
func (s *ClientService) RotateAPIKey(ctx context.Context, id, rotatedBy string) (*models.Client, error) {
	c, err := s.Repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("%w: client not found", apierr.ErrNotFound)
	}

	newRawKey := crypto.GenerateAPIKey(c.Name)
	encKey := newRawKey
	if s.EncryptionKey != "" {
		var encErr error
		encKey, encErr = crypto.EncryptAPIKey(s.EncryptionKey, newRawKey)
		if encErr != nil {
			return nil, fmt.Errorf("failed to encrypt new API key: %w", encErr)
		}
	}

	fields := map[string]interface{}{
		"api_key":    encKey,
		"updated_at": time.Now(),
	}

	_, err = s.Repo.UpdateFields(ctx, id, fields)
	if err != nil {
		return nil, err
	}

	return s.GetByID(ctx, id)
}

// Helper functions

func containsString(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// FAQGenerator defines the interface for generating FAQs from scraped content.
type FAQGenerator interface {
	GenerateFAQsFromText(ctx context.Context, clientID string, scrapedContent string) error
}

// ScrapeAndSave runs the website scraper for a client and saves the clean text content in the database.
func (s *ClientService) ScrapeAndSave(ctx context.Context, clientID string, domain string, faqGen FAQGenerator) (string, *time.Time, error) {
	scrapedText, err := scraper.ScrapeWebsite(domain)
	if err != nil {
		return "", nil, err
	}

	now := time.Now().UTC()
	fields := map[string]interface{}{
		"scraped_content":  scrapedText,
		"scrape_synced_at": now,
		"updated_at":       now,
	}

	_, err = s.Repo.UpdateFields(ctx, clientID, fields)
	if err != nil {
		return "", nil, err
	}

	// Trigger FAQ draft generation from scraped content in background
	if faqGen != nil {
		go func(cid, text string) {
			bgCtx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
			defer cancel()
			_ = faqGen.GenerateFAQsFromText(bgCtx, cid, text)
		}(clientID, scrapedText)
	}

	return scrapedText, &now, nil
}

// AutoSyncClientWebsites checks which clients are due for a scraped website sync,
// scrapes their content, and generates FAQ drafts. Iterates through all pages
// to handle more than 1000 clients.
func (s *ClientService) AutoSyncClientWebsites(ctx context.Context, faqGen FAQGenerator) {
	now := time.Now()
	page := 1
	const pageSize = 100

	for {
		clients, total, err := s.Repo.List(ctx, "", "", page, pageSize)
		if err != nil {
			slog.Error("failed to list clients for auto-sync", "error", err)
			return
		}

		for _, c := range clients {
			if !c.ScrapeEnabled || c.Domain == "" {
				continue
			}

			shouldSync := false
			if c.ScrapeSyncedAt == nil {
				shouldSync = true
			} else {
				elapsed := now.Sub(*c.ScrapeSyncedAt)
				if elapsed >= time.Duration(c.ScrapeIntervalHours)*time.Hour {
					shouldSync = true
				}
			}

			if shouldSync {
				slog.Info("starting auto-sync for client website", "client_id", c.ID, "domain", c.Domain)
				go func(cid, dom string) {
					bgCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
					defer cancel()
					_, _, err := s.ScrapeAndSave(bgCtx, cid, dom, faqGen)
					if err != nil {
						slog.Warn("auto-sync failed for client website", "client_id", cid, "error", err)
					} else {
						slog.Info("auto-sync completed for client website", "client_id", cid)
					}
				}(c.ID, c.Domain)
			}
		}

		if page*pageSize >= total {
			break
		}
		page++
	}
}
