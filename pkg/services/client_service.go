package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/codexylab/alvex-backend/pkg/apierr"
	"github.com/codexylab/alvex-backend/pkg/crypto"
	"github.com/codexylab/alvex-backend/pkg/models"
	"github.com/codexylab/alvex-backend/pkg/repository"
	"github.com/codexylab/alvex-backend/pkg/services/scraper"
)

// ClientService manages the business logic for client configuration.
type ClientService struct {
	Repo                   repository.ClientRepository
	EncryptionKey          string
	PreviousEncryptionKeys []string
	PublicAPIURL           string
}

// WithPreviousEncryptionKeys enables decrypt-only compatibility during a
// controlled encryption key rotation.
func (s *ClientService) WithPreviousEncryptionKeys(keys []string) *ClientService {
	s.PreviousEncryptionKeys = append([]string(nil), keys...)
	return s
}

// NewClientService creates a new ClientService instance.
func NewClientService(repo repository.ClientRepository, encKey, publicAPIURL string) *ClientService {
	return &ClientService{
		Repo:          repo,
		EncryptionKey: encKey,
		PublicAPIURL:  publicAPIURL,
	}
}

// List returns a paginated list of client profiles.
func (s *ClientService) List(ctx context.Context, search, status string, page, limit int) ([]models.Client, int, error) {
	return s.Repo.List(ctx, search, status, page, limit)
}

// GetByID returns a client profile with write-only provider credentials loaded
// for service use. JSON transport redaction prevents these values from leaving
// the backend.
func (s *ClientService) GetByID(ctx context.Context, id string) (*models.Client, error) {
	c, err := s.Repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "no rows") {
			return nil, fmt.Errorf("%w: client not found", apierr.ErrNotFound)
		}
		return nil, err
	}

	rotationFields := make(map[string]interface{})
	c.OpenAIAPIKey, err = s.decryptClientSecret(c.OpenAIAPIKey, "OpenAI API key", "openai_api_key", rotationFields)
	if err != nil {
		return nil, err
	}
	c.GeminiAPIKey, err = s.decryptClientSecret(c.GeminiAPIKey, "Gemini API key", "gemini_api_key", rotationFields)
	if err != nil {
		return nil, err
	}
	c.GroqAPIKey, err = s.decryptClientSecret(c.GroqAPIKey, "Groq API key", "groq_api_key", rotationFields)
	if err != nil {
		return nil, err
	}
	if len(rotationFields) > 0 {
		rotationFields["updated_at"] = time.Now().UTC()
		if _, err := s.Repo.UpdateFields(ctx, id, rotationFields); err != nil {
			return nil, fmt.Errorf("persist rotated client secrets: %w", err)
		}
	}

	return c, nil
}

func (s *ClientService) decryptClientSecret(
	storedValue string,
	label string,
	column string,
	rotationFields map[string]interface{},
) (string, error) {
	if storedValue == "" || s.EncryptionKey == "" {
		return storedValue, nil
	}
	plaintext, usedPreviousKey, err := crypto.DecryptAPIKeyWithKeyring(
		s.EncryptionKey,
		s.PreviousEncryptionKeys,
		storedValue,
	)
	if err != nil {
		return "", fmt.Errorf("decrypt %s: %w", label, err)
	}
	if usedPreviousKey || !crypto.IsVersionedCiphertext(storedValue) {
		rotatedValue, err := crypto.EncryptAPIKey(s.EncryptionKey, plaintext)
		if err != nil {
			return "", fmt.Errorf("re-encrypt %s: %w", label, err)
		}
		rotationFields[column] = rotatedValue
	}
	return plaintext, nil
}

// Create saves a client profile. Machine credentials are created separately
// through APIKeyService so they are hashed, scoped, expirable, and revocable.
func (s *ClientService) Create(ctx context.Context, req models.CreateClientRequest, ownerID string) (*models.Client, error) {
	if req.Name == "" || req.Domain == "" {
		return nil, fmt.Errorf("name and domain are required")
	}
	normalizedDomain, err := scraper.NormalizeWebsiteURL(req.Domain)
	if err != nil {
		return nil, &apierr.ValidationError{Message: "domain must be a valid public HTTP or HTTPS website URL"}
	}
	req.Domain = normalizedDomain

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
	allowedOrigins, err := validateAllowedOrigins(req.AllowedOrigins)
	if err != nil {
		return nil, err
	}

	// Validate model
	validModels := models.ProviderModels[req.Provider]
	if !containsString(validModels, req.Model) {
		return nil, fmt.Errorf("model %q is not valid for provider %q", req.Model, req.Provider)
	}

	webhookURL := crypto.GenerateWebhookURL(s.PublicAPIURL, id)
	systemPersona := fmt.Sprintf(
		"You are an AI representative for %s. Help visitors with inquiries on %s.\n\nTONE: Friendly and professional.",
		req.Name, req.Domain,
	)

	c := &models.Client{
		ID:                       id,
		Name:                     req.Name,
		Domain:                   req.Domain,
		AllowedOrigins:           allowedOrigins,
		Status:                   models.ClientStatusActive,
		Provider:                 req.Provider,
		Model:                    req.Model,
		SystemPersona:            systemPersona,
		WebhookURL:               webhookURL,
		Temperature:              0.7,
		StrictAdherence:          true,
		BillingPlan:              req.BillingPlan,
		OwnerID:                  &ownerID,
		WidgetChatEnabled:        true,
		WidgetTicketingEnabled:   true,
		WidgetAdminMsgEnabled:    true,
		WidgetImageSearchEnabled: true,
		WidgetTicketingAllowed:   true,
		WidgetAdminMsgAllowed:    true,
		WidgetImageSearchAllowed: true,
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
	if req.Domain != nil {
		normalizedDomain, err := scraper.NormalizeWebsiteURL(*req.Domain)
		if err != nil {
			return nil, "", &apierr.ValidationError{Message: "domain must be a valid public HTTP or HTTPS website URL"}
		}
		req.Domain = &normalizedDomain
		domainToSync = normalizedDomain
	}

	fields := map[string]interface{}{
		"updated_at": time.Now(),
	}

	if req.Domain != nil {
		fields["domain"] = *req.Domain
	}
	if req.AllowedOrigins != nil {
		allowedOrigins, err := validateAllowedOrigins(*req.AllowedOrigins)
		if err != nil {
			return nil, "", err
		}
		fields["allowed_origins"] = repository.MarshalStringSlice(allowedOrigins)
	}
	if req.WhatsAppPhoneNumberID != nil {
		phoneNumberID := strings.TrimSpace(*req.WhatsAppPhoneNumberID)
		if phoneNumberID != "" && !whatsAppPhoneNumberIDPattern.MatchString(phoneNumberID) {
			return nil, "", &apierr.ValidationError{Message: "whatsapp_phone_number_id must contain digits only"}
		}
		fields["whatsapp_phone_number_id"] = phoneNumberID
	}
	if req.Provider != nil {
		fields["provider"] = *req.Provider
	}
	if req.Model != nil {
		fields["model"] = *req.Model
	}
	if req.OpenAIAPIKey != nil {
		keyToStore := *req.OpenAIAPIKey
		if s.EncryptionKey != "" {
			var err error
			keyToStore, err = crypto.EncryptAPIKey(s.EncryptionKey, *req.OpenAIAPIKey)
			if err != nil {
				return nil, "", fmt.Errorf("encrypt OpenAI API key: %w", err)
			}
		}
		fields["openai_api_key"] = keyToStore
	}
	if req.GeminiAPIKey != nil {
		keyToStore := *req.GeminiAPIKey
		if s.EncryptionKey != "" {
			var err error
			keyToStore, err = crypto.EncryptAPIKey(s.EncryptionKey, *req.GeminiAPIKey)
			if err != nil {
				return nil, "", fmt.Errorf("encrypt Gemini API key: %w", err)
			}
		}
		fields["gemini_api_key"] = keyToStore
	}
	if req.GroqAPIKey != nil {
		keyToStore := *req.GroqAPIKey
		if s.EncryptionKey != "" {
			var err error
			keyToStore, err = crypto.EncryptAPIKey(s.EncryptionKey, *req.GroqAPIKey)
			if err != nil {
				return nil, "", fmt.Errorf("encrypt Groq API key: %w", err)
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
	return s.updateStatus(ctx, id, c.Status, newStatus)
}

// SetStatus applies an explicit desired state. Repeating the same request is
// idempotent, which is required for reliable bulk operations and retries.
func (s *ClientService) SetStatus(ctx context.Context, id string, status models.ClientStatus) (string, error) {
	if err := validateClientStatus(status); err != nil {
		return "", err
	}

	c, err := s.Repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "no rows") {
			return "", fmt.Errorf("%w: client not found", apierr.ErrNotFound)
		}
		return "", err
	}

	return s.updateStatus(ctx, id, c.Status, status)
}

// SetAllStatuses applies one desired state to every client in the current
// organization in a single tenant-scoped database update.
func (s *ClientService) SetAllStatuses(ctx context.Context, status models.ClientStatus) (int64, error) {
	if err := validateClientStatus(status); err != nil {
		return 0, err
	}
	return s.Repo.UpdateAllStatuses(ctx, status, time.Now().UTC())
}

func validateClientStatus(status models.ClientStatus) error {
	if status != models.ClientStatusActive && status != models.ClientStatusSuspended {
		return &apierr.ValidationError{Message: "status must be Active or Suspended"}
	}
	return nil
}

func (s *ClientService) updateStatus(
	ctx context.Context,
	id string,
	currentStatus models.ClientStatus,
	desiredStatus models.ClientStatus,
) (string, error) {
	if currentStatus == desiredStatus {
		return string(desiredStatus), nil
	}

	fields := map[string]interface{}{
		"status":     desiredStatus,
		"updated_at": time.Now(),
	}

	rowsAffected, err := s.Repo.UpdateFields(ctx, id, fields)
	if err != nil {
		return "", err
	}
	if rowsAffected == 0 {
		return "", fmt.Errorf("%w: client not found", apierr.ErrNotFound)
	}

	return string(desiredStatus), nil
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

// Helper functions

func containsString(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func validateAllowedOrigins(values []string) ([]string, error) {
	if len(values) > 20 {
		return nil, &apierr.ValidationError{Message: "allowed_origins cannot contain more than 20 entries"}
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		parsed, err := url.Parse(strings.TrimSpace(value))
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
			parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return nil, &apierr.ValidationError{Message: fmt.Sprintf(
				"allowed origin %q must contain only an HTTP or HTTPS scheme and host",
				value,
			)}
		}
		normalized := strings.ToLower(parsed.Scheme + "://" + parsed.Host)
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result, nil
}

// FAQGenerator defines the interface for generating FAQs from scraped content.
type FAQGenerator interface {
	GenerateFAQsFromText(ctx context.Context, clientID string, scrapedContent string) error
}

// ScrapeAndSave runs the website scraper and persists clean source text. RAG
// indexing and FAQ generation are separate durable job steps.
func (s *ClientService) ScrapeAndSave(ctx context.Context, clientID string, domain string) (string, *time.Time, error) {
	scrapedText, err := scraper.ScrapeWebsite(ctx, domain)
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

	return scrapedText, &now, nil
}

func (s *ClientService) UpdateOnboardingStatus(ctx context.Context, clientID, status string) error {
	if status != "processing" && status != "complete" && status != "failed" {
		return fmt.Errorf("invalid onboarding status %q", status)
	}
	result, err := s.Repo.UpdateFields(ctx, clientID, map[string]interface{}{
		"onboarding_status": status,
		"updated_at":        time.Now().UTC(),
	})
	if err != nil {
		return err
	}
	if result == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// QueueDueWebsiteSyncs enqueues due website refreshes without starting
// untracked goroutines. It iterates through all pages for large installations.
func (s *ClientService) QueueDueWebsiteSyncs(
	ctx context.Context,
	scheduler *WebsiteIndexScheduler,
	now time.Time,
) (int, error) {
	if scheduler == nil {
		return 0, fmt.Errorf("website sync scheduler is not configured")
	}
	page := 1
	const pageSize = 100
	queued := 0

	for {
		clients, total, err := s.Repo.List(ctx, "", "", page, pageSize)
		if err != nil {
			return queued, fmt.Errorf("list clients for auto-sync: %w", err)
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
				key := fmt.Sprintf("%s:%s:%d", WebsiteSyncAutomatic, c.ID, now.UTC().Truncate(time.Hour).Unix())
				if _, err := scheduler.Enqueue(ctx, c.ID, c.Domain, WebsiteSyncAutomatic, key); err != nil {
					return queued, fmt.Errorf("queue website sync for client %s: %w", c.ID, err)
				}
				queued++
			}
		}

		if page*pageSize >= total {
			break
		}
		page++
	}
	return queued, nil
}
