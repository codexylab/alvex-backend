package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/codexylab/alvex-backend/internal/models"
	"github.com/codexylab/alvex-backend/internal/repository"
	aiservice "github.com/codexylab/alvex-backend/internal/services/ai"
	"github.com/codexylab/alvex-backend/pkg/apierr"
)

// PortalService coordinates client portal functionalities.
type PortalService struct {
	Repo           repository.PortalRepository
	EncryptionKey  string
	PlatformGemini string
	PlatformOpenAI string
	PlatformGroq   string
	FallbackGemini string
}

// NewPortalService creates a new PortalService instance.
func NewPortalService(
	repo repository.PortalRepository,
	encKey, platGemini, platOpenAI, platGroq, fallbackGemini string,
) *PortalService {
	return &PortalService{
		Repo:           repo,
		EncryptionKey:  encKey,
		PlatformGemini: platGemini,
		PlatformOpenAI: platOpenAI,
		PlatformGroq:   platGroq,
		FallbackGemini: fallbackGemini,
	}
}

// GetClientProfile returns portal details.
func (s *PortalService) GetClientProfile(ctx context.Context, id string) (*models.Client, error) {
	return s.Repo.GetClientProfile(ctx, id)
}

// GetStats returns portal aggregated statistics.
func (s *PortalService) GetStats(ctx context.Context, clientID string, isSQLite bool) (*repository.PortalStatsData, error) {
	return s.Repo.GetPortalStats(ctx, clientID, isSQLite)
}

// GetConversations lists conversations transcripts.
func (s *PortalService) GetConversations(ctx context.Context, clientID string, page, limit int) ([]models.ActivityLog, int, error) {
	return s.Repo.GetConversations(ctx, clientID, page, limit)
}

// GetFAQs retrieves FAQs.
func (s *PortalService) GetFAQs(ctx context.Context, clientID string) ([]models.FAQ, error) {
	return s.Repo.GetFAQs(ctx, clientID)
}

// CreateFAQ creates an FAQ.
func (s *PortalService) CreateFAQ(ctx context.Context, clientID string, req models.CreateFAQRequest) (map[string]interface{}, error) {
	if req.Question == "" || req.Answer == "" {
		return nil, fmt.Errorf("%w: question and answer are required", apierr.ErrValidation)
	}

	faqID := uuid.New().String()
	err := s.Repo.CreateFAQ(ctx, faqID, clientID, req.Question, req.Answer, req.IsApproved)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"id":          faqID,
		"client_id":   clientID,
		"question":    req.Question,
		"answer":      req.Answer,
		"is_approved": req.IsApproved,
	}, nil
}

// UpdateFAQ updates FAQ fields.
func (s *PortalService) UpdateFAQ(ctx context.Context, faqID, clientID string, req models.CreateFAQRequest) (map[string]interface{}, error) {
	if req.Question == "" || req.Answer == "" {
		return nil, fmt.Errorf("%w: question and answer are required", apierr.ErrValidation)
	}

	rowsAffected, err := s.Repo.UpdateFAQ(ctx, faqID, clientID, req.Question, req.Answer, req.IsApproved)
	if err != nil {
		return nil, err
	}
	if rowsAffected == 0 {
		return nil, fmt.Errorf("%w: faq not found", apierr.ErrNotFound)
	}

	return map[string]interface{}{
		"id":          faqID,
		"client_id":   clientID,
		"question":    req.Question,
		"answer":      req.Answer,
		"is_approved": req.IsApproved,
	}, nil
}

// DeleteFAQ deletes FAQ.
func (s *PortalService) DeleteFAQ(ctx context.Context, faqID, clientID string) error {
	rowsAffected, err := s.Repo.DeleteFAQ(ctx, faqID, clientID)
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("%w: faq not found", apierr.ErrNotFound)
	}
	return nil
}

// ReplyToConversation resolves a complaint ticket.
func (s *PortalService) ReplyToConversation(ctx context.Context, id, clientID, reply string) error {
	if reply == "" {
		return fmt.Errorf("%w: reply cannot be empty", apierr.ErrCannotBeEmpty)
	}

	rowsAffected, err := s.Repo.ReplyToConversation(ctx, id, clientID, reply)
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("%w: conversation not found", apierr.ErrNotFound)
	}
	return nil
}

// UpdateConfig updates configs.
func (s *PortalService) UpdateConfig(ctx context.Context, clientID string, req models.UpdatePortalConfigRequest) error {
	c, err := s.Repo.GetClientProfile(ctx, clientID)
	if err != nil {
		return err
	}

	sets := []string{"updated_at = $1"}
	args := []interface{}{time.Now()}
	argIdx := 2

	if req.WidgetTicketingEnabled != nil {
		val := *req.WidgetTicketingEnabled
		if !c.WidgetTicketingAllowed {
			val = false
		}
		sets = append(sets, fmt.Sprintf("widget_ticketing_enabled = $%d", argIdx))
		args = append(args, val)
		argIdx++
	}

	if req.WidgetAdminMsgEnabled != nil {
		val := *req.WidgetAdminMsgEnabled
		if !c.WidgetAdminMsgAllowed {
			val = false
		}
		sets = append(sets, fmt.Sprintf("widget_admin_msg_enabled = $%d", argIdx))
		args = append(args, val)
		argIdx++
	}

	if req.WidgetImageSearchEnabled != nil {
		val := *req.WidgetImageSearchEnabled
		if !c.WidgetImageSearchAllowed {
			val = false
		}
		sets = append(sets, fmt.Sprintf("widget_image_search_enabled = $%d", argIdx))
		args = append(args, val)
		argIdx++
	}

	if req.GuardrailsEnabled != nil {
		val := 0
		if *req.GuardrailsEnabled {
			val = 1
		}
		sets = append(sets, fmt.Sprintf("guardrails_enabled = $%d", argIdx))
		args = append(args, val)
		argIdx++
	}

	if req.GuardrailsReply != nil {
		sets = append(sets, fmt.Sprintf("guardrails_reply = $%d", argIdx))
		args = append(args, *req.GuardrailsReply)
		argIdx++
	}

	if req.ChatRetentionDays != nil {
		days := *req.ChatRetentionDays
		if days != 7 && days != 15 && days != 30 {
			days = 30
		}
		sets = append(sets, fmt.Sprintf("chat_retention_days = $%d", argIdx))
		args = append(args, days)
		argIdx++
	}

	if len(sets) > 1 {
		args = append(args, clientID)
		query := fmt.Sprintf("UPDATE clients SET %s WHERE id = $%d", strings.Join(sets, ", "), argIdx)
		_, err = s.Repo.UpdatePortalConfig(ctx, query, args...)
		return err
	}

	return nil
}

// GenerateFAQs parses scraped content and generates FAQ drafts.
func (s *PortalService) GenerateFAQs(ctx context.Context, clientID string) error {
	c, err := s.Repo.GetClientProfile(ctx, clientID)
	if err != nil {
		return err
	}

	if c.ScrapedContent == "" {
		return fmt.Errorf("no website content synced yet. Sync website knowledge first.")
	}

	return s.GenerateFAQsFromText(ctx, clientID, c.ScrapedContent)
}

// GenerateFAQsFromText generates FAQ drafts from crawled text content.
func (s *PortalService) GenerateFAQsFromText(ctx context.Context, clientID string, scrapedContent string) error {
	c, err := s.Repo.GetClientProfile(ctx, clientID)
	if err != nil {
		return err
	}

	apiKey := resolveProviderAPIKey(c, platformKeys{
		EncryptionKey:  s.EncryptionKey,
		Gemini:         s.PlatformGemini,
		OpenAI:         s.PlatformOpenAI,
		Groq:           s.PlatformGroq,
		FallbackGemini: s.FallbackGemini,
	})

	aiProvider, err := aiservice.NewProviderWithFallback(string(c.Provider), apiKey, c.Model, s.FallbackGemini)
	if err != nil {
		return fmt.Errorf("failed to build AI provider: %w", err)
	}

	systemPrompt := "You are a professional FAQ generator. Your task is to analyze crawled website content of a business and generate a list of 10 to 15 key frequently asked questions (FAQs) and their accurate answers in English. " +
		"Focus on representing actual customer questions about services, products, pricing, location, contact, or hours. " +
		"All questions and answers MUST be in English. " +
		"Output ONLY a valid JSON array of objects. Do not include markdown blocks like ```json or explanation text. " +
		"Each object must have 'question' and 'answer' keys. " +
		"Example: [{\"question\": \"What are your hours?\", \"answer\": \"Mon-Fri 9am-6pm.\"}]"

	contentToAnalyze := scrapedContent
	if len(contentToAnalyze) > 100000 {
		contentToAnalyze = contentToAnalyze[:100000]
	}

	reply, err := aiProvider.Chat(systemPrompt, nil, contentToAnalyze)
	if err != nil {
		return fmt.Errorf("AI generation failed: %w", err)
	}

	reply = strings.TrimSpace(reply)
	reply = strings.TrimPrefix(reply, "```json")
	reply = strings.TrimPrefix(reply, "```")
	reply = strings.TrimSuffix(reply, "```")
	reply = strings.TrimSpace(reply)

	type faqDraft struct {
		Question string `json:"question"`
		Answer   string `json:"answer"`
	}
	var drafts []faqDraft
	if err := json.Unmarshal([]byte(reply), &drafts); err != nil {
		return fmt.Errorf("failed to parse AI JSON response: %w (Response: %s)", err, reply)
	}

	repoDrafts := []struct{ Question, Answer string }{}
	for _, d := range drafts {
		q := strings.TrimSpace(d.Question)
		a := strings.TrimSpace(d.Answer)
		if q == "" || a == "" {
			continue
		}
		repoDrafts = append(repoDrafts, struct{ Question, Answer string }{Question: q, Answer: a})
	}

	return s.Repo.InsertFAQsDraftsTx(ctx, clientID, repoDrafts)
}

// GetClientInvoices lists invoices.
func (s *PortalService) GetClientInvoices(ctx context.Context, clientID string) ([]models.Invoice, error) {
	return s.Repo.GetClientInvoices(ctx, clientID)
}

// GetClientActivityLogs lists logs.
func (s *PortalService) GetClientActivityLogs(ctx context.Context, clientID string) ([]models.ActivityLog, error) {
	return s.Repo.GetClientActivityLogs(ctx, clientID)
}
