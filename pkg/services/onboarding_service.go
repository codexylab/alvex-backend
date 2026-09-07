package services

import (
	"context"
	"fmt"
	"html"
	"strings"

	"github.com/codexylab/alvex-backend/pkg/models"
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
	ClientSvc       *ClientService
	WebsiteSync     *WebsiteIndexScheduler
	WidgetScriptURL string
	PublicAPIURL    string
}

// NewOnboardingService creates a new OnboardingService instance.
func NewOnboardingService(
	clientSvc *ClientService,
	websiteSync *WebsiteIndexScheduler,
	frontendURL string,
	publicAPIURL string,
) *OnboardingService {
	return &OnboardingService{
		ClientSvc:       clientSvc,
		WebsiteSync:     websiteSync,
		WidgetScriptURL: strings.TrimRight(frontendURL, "/") + "/widget.js",
		PublicAPIURL:    strings.TrimRight(publicAPIURL, "/"),
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
	snippet := fmt.Sprintf(
		`<script src="%s" data-id="asst_%s" data-api-url="%s" async></script>`,
		html.EscapeString(s.WidgetScriptURL),
		html.EscapeString(client.ID),
		html.EscapeString(s.PublicAPIURL),
	)
	portalURL := "/portal"

	status := "complete"
	if req.Domain != "" {
		status = "processing"
		if err := s.ClientSvc.UpdateOnboardingStatus(ctx, client.ID, status); err != nil {
			return nil, fmt.Errorf("mark onboarding processing: %w", err)
		}
		key := WebsiteSyncOnboarding + ":" + client.ID
		if _, err := s.WebsiteSync.Enqueue(ctx, client.ID, req.Domain, WebsiteSyncOnboarding, key); err != nil {
			_ = s.ClientSvc.UpdateOnboardingStatus(ctx, client.ID, "failed")
			return nil, fmt.Errorf("queue onboarding website sync: %w", err)
		}
	}

	return &OnboardingResult{
		Client:        client,
		WidgetSnippet: snippet,
		PortalURL:     portalURL,
		Status:        status,
	}, nil
}
