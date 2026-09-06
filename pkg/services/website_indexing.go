package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/codexylab/alvex-backend/pkg/models"
	"github.com/codexylab/alvex-backend/pkg/repository"
	"github.com/codexylab/alvex-backend/pkg/services/scraper"
)

const WebsiteIndexJobType = "knowledge.website_index"

const (
	WebsiteSyncOnboarding   = "onboarding"
	WebsiteSyncClientCreate = "client_created"
	WebsiteSyncDomainChange = "domain_changed"
	WebsiteSyncManual       = "manual"
	WebsiteSyncAutomatic    = "automatic"
)

type WebsiteIndexJobPayload struct {
	ClientID     string `json:"client_id"`
	Domain       string `json:"domain"`
	Reason       string `json:"reason"`
	ContentReady bool   `json:"content_ready,omitempty"`
}

type WebsiteClientOperations interface {
	GetByID(ctx context.Context, id string) (*models.Client, error)
	ScrapeAndSave(ctx context.Context, clientID, domain string) (string, *time.Time, error)
	UpdateOnboardingStatus(ctx context.Context, clientID, status string) error
}

// WebsiteIndexScheduler creates small durable jobs; scraped website content
// remains in the client record instead of being copied into the queue payload.
type WebsiteIndexScheduler struct {
	Jobs repository.BackgroundJobRepository
}

func NewWebsiteIndexScheduler(jobs repository.BackgroundJobRepository) *WebsiteIndexScheduler {
	return &WebsiteIndexScheduler{Jobs: jobs}
}

func (s *WebsiteIndexScheduler) Enqueue(
	ctx context.Context,
	clientID string,
	domain string,
	reason string,
	idempotencyKey string,
) (string, error) {
	clientID = strings.TrimSpace(clientID)
	domain = strings.TrimSpace(domain)
	if clientID == "" || domain == "" || reason == "" {
		return "", fmt.Errorf("website sync requires client, domain, and reason")
	}
	if s == nil || s.Jobs == nil {
		return "", fmt.Errorf("website indexing queue is not configured")
	}
	normalizedDomain, err := scraper.NormalizeWebsiteURL(domain)
	if err != nil {
		return "", fmt.Errorf("invalid website domain: %w", err)
	}
	domain = normalizedDomain
	if idempotencyKey == "" {
		idempotencyKey = reason + ":" + clientID + ":" + uuid.NewString()
	}
	payload, err := json.Marshal(WebsiteIndexJobPayload{
		ClientID: clientID,
		Domain:   domain,
		Reason:   reason,
	})
	if err != nil {
		return "", fmt.Errorf("encode website index job: %w", err)
	}
	if _, err := s.Jobs.Enqueue(ctx, WebsiteIndexJobType, idempotencyKey, payload, 6); err != nil {
		return "", fmt.Errorf("queue website indexing: %w", err)
	}
	return idempotencyKey, nil
}

func (s *WebsiteIndexScheduler) EnqueueManual(
	ctx context.Context,
	clientID string,
	domain string,
) (string, error) {
	return s.Enqueue(ctx, clientID, domain, WebsiteSyncManual, "")
}

// WebsiteIndexJobProcessor performs scraping, indexing, and FAQ generation in
// a recoverable sequence. The updated payload prevents re-scraping on retries.
type WebsiteIndexJobProcessor struct {
	Jobs    repository.BackgroundJobRepository
	Clients WebsiteClientOperations
	RAG     *RAGService
	FAQs    FAQGenerator
}

func (p *WebsiteIndexJobProcessor) Process(ctx context.Context, job repository.BackgroundJob) error {
	var payload WebsiteIndexJobPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("decode website index job: %w", err)
	}
	if payload.ClientID == "" || payload.Domain == "" || payload.Reason == "" {
		return fmt.Errorf("website index job is missing required fields")
	}
	if p.Clients == nil || p.RAG == nil || p.Jobs == nil {
		return fmt.Errorf("website index processor is not configured")
	}

	client, err := p.Clients.GetByID(ctx, payload.ClientID)
	if err != nil {
		return fmt.Errorf("load website client: %w", err)
	}
	if strings.TrimSpace(client.Domain) == "" {
		return fmt.Errorf("client website domain is empty")
	}
	if client.Domain != payload.Domain {
		payload.Domain = client.Domain
		payload.ContentReady = false
	}

	content := client.ScrapedContent
	if !payload.ContentReady {
		content, _, err = p.Clients.ScrapeAndSave(ctx, payload.ClientID, payload.Domain)
		if err != nil {
			p.markTerminalOnboardingFailure(ctx, job, payload)
			return fmt.Errorf("scrape website: %w", err)
		}
		payload.ContentReady = true
		updatedPayload, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return fmt.Errorf("encode website index progress: %w", marshalErr)
		}
		if err := p.Jobs.UpdatePayload(ctx, job.ID, updatedPayload); err != nil {
			return fmt.Errorf("save website index progress: %w", err)
		}
	}
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("scraped website content is empty")
	}
	if err := p.RAG.IndexContent(ctx, payload.ClientID, "website", content); err != nil {
		p.markTerminalOnboardingFailure(ctx, job, payload)
		return fmt.Errorf("index website content: %w", err)
	}
	if p.FAQs != nil {
		if err := p.FAQs.GenerateFAQsFromText(ctx, payload.ClientID, content); err != nil {
			p.markTerminalOnboardingFailure(ctx, job, payload)
			return fmt.Errorf("generate website FAQs: %w", err)
		}
	}
	if payload.Reason == WebsiteSyncOnboarding {
		if err := p.Clients.UpdateOnboardingStatus(ctx, payload.ClientID, "complete"); err != nil {
			return fmt.Errorf("complete onboarding: %w", err)
		}
	}
	return nil
}

func (p *WebsiteIndexJobProcessor) markTerminalOnboardingFailure(
	ctx context.Context,
	job repository.BackgroundJob,
	payload WebsiteIndexJobPayload,
) {
	if payload.Reason != WebsiteSyncOnboarding || job.Attempts < job.MaxAttempts {
		return
	}
	_ = p.Clients.UpdateOnboardingStatus(ctx, payload.ClientID, "failed")
}
