package services

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/codexylab/alvex-backend/pkg/models"
	"github.com/codexylab/alvex-backend/pkg/repository"
)

type websiteJobRepositorySpy struct {
	jobType        string
	idempotencyKey string
	payload        []byte
	updatedPayload []byte
}

func (s *websiteJobRepositorySpy) Enqueue(_ context.Context, jobType, key string, payload []byte, _ int) (bool, error) {
	s.jobType = jobType
	s.idempotencyKey = key
	s.payload = append([]byte(nil), payload...)
	return true, nil
}
func (s *websiteJobRepositorySpy) ClaimNext(context.Context, string, time.Time) (*repository.BackgroundJob, error) {
	return nil, errors.New("not implemented")
}
func (s *websiteJobRepositorySpy) UpdatePayload(_ context.Context, _ string, payload []byte) error {
	s.updatedPayload = append([]byte(nil), payload...)
	return nil
}
func (s *websiteJobRepositorySpy) Complete(context.Context, string, time.Time) error { return nil }
func (s *websiteJobRepositorySpy) Fail(context.Context, repository.BackgroundJob, string, time.Time) error {
	return nil
}

type websiteClientOperationsStub struct {
	client        models.Client
	scraped       string
	scrapeCalls   int
	statusUpdates []string
}

func (s *websiteClientOperationsStub) GetByID(context.Context, string) (*models.Client, error) {
	copy := s.client
	return &copy, nil
}
func (s *websiteClientOperationsStub) ScrapeAndSave(context.Context, string, string) (string, *time.Time, error) {
	s.scrapeCalls++
	now := time.Now().UTC()
	s.client.ScrapedContent = s.scraped
	return s.scraped, &now, nil
}
func (s *websiteClientOperationsStub) UpdateOnboardingStatus(_ context.Context, _ string, status string) error {
	s.statusUpdates = append(s.statusUpdates, status)
	return nil
}

type faqGeneratorSpy struct {
	calls int
}

func (s *faqGeneratorSpy) GenerateFAQsFromText(context.Context, string, string) error {
	s.calls++
	return nil
}

func TestWebsiteSchedulerQueuesIdentifierOnlyPayload(t *testing.T) {
	jobs := &websiteJobRepositorySpy{}
	scheduler := NewWebsiteIndexScheduler(jobs)

	jobID, err := scheduler.EnqueueManual(context.Background(), "client_one", "https://example.com")
	if err != nil {
		t.Fatalf("queue website sync: %v", err)
	}
	if jobID == "" || jobs.jobType != WebsiteIndexJobType || jobs.idempotencyKey != jobID {
		t.Fatalf("unexpected queued job: id=%q type=%q key=%q", jobID, jobs.jobType, jobs.idempotencyKey)
	}
	var payload WebsiteIndexJobPayload
	if err := json.Unmarshal(jobs.payload, &payload); err != nil {
		t.Fatalf("decode queued payload: %v", err)
	}
	if payload.ClientID != "client_one" || payload.Domain != "https://example.com" ||
		payload.Reason != WebsiteSyncManual || payload.ContentReady {
		t.Fatalf("unexpected queued payload: %#v", payload)
	}
}

func TestWebsiteProcessorPersistsProgressAndCompletesOnboarding(t *testing.T) {
	jobs := &websiteJobRepositorySpy{}
	clients := &websiteClientOperationsStub{
		client:  models.Client{ID: "client_one", Domain: "https://example.com"},
		scraped: "durable website knowledge",
	}
	chunks := &chunkRepositorySpy{}
	faqs := &faqGeneratorSpy{}
	processor := &WebsiteIndexJobProcessor{
		Jobs:    jobs,
		Clients: clients,
		RAG:     NewRAGService(chunks, embeddingGeneratorStub{}),
		FAQs:    faqs,
	}
	payload, _ := json.Marshal(WebsiteIndexJobPayload{
		ClientID: "client_one",
		Domain:   "https://example.com",
		Reason:   WebsiteSyncOnboarding,
	})

	if err := processor.Process(context.Background(), repository.BackgroundJob{
		ID: "job_one", Payload: payload, Attempts: 1, MaxAttempts: 6,
	}); err != nil {
		t.Fatalf("process website job: %v", err)
	}
	if clients.scrapeCalls != 1 || faqs.calls != 1 || len(chunks.replaced) != 1 {
		t.Fatalf(
			"unexpected processing calls: scrape=%d faq=%d chunks=%d",
			clients.scrapeCalls,
			faqs.calls,
			len(chunks.replaced),
		)
	}
	if chunks.replacedDocumentID != "" || chunks.replaced[0].SourceURL != "website" {
		t.Fatalf("website chunks used wrong source identity: %#v", chunks.replaced[0])
	}
	var progress WebsiteIndexJobPayload
	if err := json.Unmarshal(jobs.updatedPayload, &progress); err != nil || !progress.ContentReady {
		t.Fatalf("scrape progress was not persisted: payload=%s err=%v", jobs.updatedPayload, err)
	}
	if len(clients.statusUpdates) != 1 || clients.statusUpdates[0] != "complete" {
		t.Fatalf("unexpected onboarding states: %#v", clients.statusUpdates)
	}
}

func TestWebsiteProcessorMarksTerminalOnboardingFailure(t *testing.T) {
	clients := &websiteClientOperationsStub{client: models.Client{
		ID:             "client_one",
		Domain:         "https://example.com",
		ScrapedContent: "existing knowledge",
	}}
	processor := &WebsiteIndexJobProcessor{
		Jobs:    &websiteJobRepositorySpy{},
		Clients: clients,
		RAG:     NewRAGService(&chunkRepositorySpy{}, embeddingGeneratorStub{err: errors.New("offline")}),
	}
	payload, _ := json.Marshal(WebsiteIndexJobPayload{
		ClientID:     "client_one",
		Domain:       "https://example.com",
		Reason:       WebsiteSyncOnboarding,
		ContentReady: true,
	})

	if err := processor.Process(context.Background(), repository.BackgroundJob{
		ID: "job_one", Payload: payload, Attempts: 6, MaxAttempts: 6,
	}); err == nil {
		t.Fatal("expected indexing failure")
	}
	if len(clients.statusUpdates) != 1 || clients.statusUpdates[0] != "failed" {
		t.Fatalf("terminal failure did not update onboarding: %#v", clients.statusUpdates)
	}
}
