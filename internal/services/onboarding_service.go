package services

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/codexylab/alvex-backend/internal/models"
)

// OnboardingRequest contains everything needed to setup and activate a new client in 1-click.
type OnboardingRequest struct {
	Name        string             `json:"name"`
	Domain      string             `json:"domain"`
	Provider    models.AIProvider  `json:"provider"`
	Model       string             `json:"model"`
	BillingPlan models.BillingPlan `json:"billing_plan"`
	Role        string             `json:"role,omitempty"`
	Tone        string             `json:"tone,omitempty"`
}

// OnboardingResult contains the generated credentials, widget snippet, and status.
type OnboardingResult struct {
	Client        *models.Client `json:"client"`
	WidgetSnippet string         `json:"widget_snippet"`
	PortalURL     string         `json:"portal_url"`
	Status        string         `json:"status"`
}

// OnboardingService orchestrates end-to-end automated client onboarding.
type OnboardingService struct {
	ClientSvc *ClientService
	PortalSvc *PortalService
	RAGSvc    *RAGService
}

// NewOnboardingService creates a new OnboardingService instance.
func NewOnboardingService(clientSvc *ClientService, portalSvc *PortalService, ragSvc *RAGService) *OnboardingService {
	return &OnboardingService{
		ClientSvc: clientSvc,
		PortalSvc: portalSvc,
		RAGSvc:    ragSvc,
	}
}

// StartOnboarding creates client, initiates website crawl, indexes RAG vectors, and generates widget code.
func (s *OnboardingService) StartOnboarding(ctx context.Context, req OnboardingRequest, ownerID string) (*OnboardingResult, error) {
	if req.Provider == "" {
		req.Provider = models.ProviderGemini
	}
	if req.Model == "" {
		req.Model = "Gemini 2.0 Flash"
	}
	if req.BillingPlan == "" {
		req.BillingPlan = models.BillingBasic
	}

	createReq := models.CreateClientRequest{
		Name:        req.Name,
		Domain:      req.Domain,
		Provider:    req.Provider,
		Model:       req.Model,
		BillingPlan: req.BillingPlan,
	}

	client, err := s.ClientSvc.Create(ctx, createReq, ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	// Build widget embed snippet
	snippet := fmt.Sprintf(`<script src="https://cdn.jsdelivr.net/gh/codexylab/alvex-widget@main/widget.js" data-client-id="%s" async></script>`, client.ID)
	portalURL := fmt.Sprintf("/portal?token=%s", client.PortalToken)

	// Trigger async website scrape + RAG indexing if domain provided
	if req.Domain != "" {
		go func(cid, dom string) {
			bgCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()

			slog.Info("onboarding: scraping website", "client_id", cid, "domain", dom)
			scrapedText, _, err := s.ClientSvc.ScrapeAndSave(bgCtx, cid, dom, s.PortalSvc)
			if err != nil {
				slog.Warn("onboarding: website scrape failed", "client_id", cid, "error", err)
			} else if s.RAGSvc != nil && scrapedText != "" {
				slog.Info("onboarding: indexing RAG vectors", "client_id", cid)
				_ = s.RAGSvc.IndexContent(bgCtx, cid, dom, scrapedText)
			}

			// Mark onboarding complete
			_, _ = s.ClientSvc.Repo.UpdateFields(bgCtx, cid, map[string]interface{}{
				"onboarding_status": "complete",
				"updated_at":        time.Now(),
			})
		}(client.ID, req.Domain)
	}

	return &OnboardingResult{
		Client:        client,
		WidgetSnippet: snippet,
		PortalURL:     portalURL,
		Status:        "processing",
	}, nil
}
