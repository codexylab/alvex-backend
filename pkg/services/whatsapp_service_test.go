package services

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/codexylab/alvex-backend/pkg/repository"
)

type whatsAppJobRepositoryStub struct {
	jobs map[string][]byte
}

func (s *whatsAppJobRepositoryStub) Enqueue(_ context.Context, _, key string, payload []byte, _ int) (bool, error) {
	if s.jobs == nil {
		s.jobs = make(map[string][]byte)
	}
	if _, exists := s.jobs[key]; exists {
		return false, nil
	}
	s.jobs[key] = append([]byte(nil), payload...)
	return true, nil
}
func (s *whatsAppJobRepositoryStub) ClaimNext(context.Context, string, time.Time) (*repository.BackgroundJob, error) {
	return nil, nil
}
func (s *whatsAppJobRepositoryStub) UpdatePayload(context.Context, string, []byte) error { return nil }
func (s *whatsAppJobRepositoryStub) Complete(context.Context, string, time.Time) error   { return nil }
func (s *whatsAppJobRepositoryStub) Fail(context.Context, repository.BackgroundJob, string, time.Time) error {
	return nil
}

type whatsAppDeliveryRepositorySpy struct {
	deliveries []repository.WhatsAppDelivery
}

func (s *whatsAppDeliveryRepositorySpy) ClientOwnsPhoneNumber(_ context.Context, clientID, phoneNumberID string) (bool, error) {
	return clientID == "client_one" && phoneNumberID == "12345", nil
}

func (s *whatsAppDeliveryRepositorySpy) Upsert(_ context.Context, delivery repository.WhatsAppDelivery) error {
	s.deliveries = append(s.deliveries, delivery)
	return nil
}

func TestWhatsAppWebhookAcceptsEveryMessageOnceAndTracksStatuses(t *testing.T) {
	jobs := &whatsAppJobRepositoryStub{}
	deliveries := &whatsAppDeliveryRepositorySpy{}
	service := NewWhatsAppWebhookService(jobs, deliveries, deliveries)
	payload := []byte(`{
		"object":"whatsapp_business_account",
		"entry":[
			{"changes":[{"value":{"metadata":{"phone_number_id":"12345"},"messages":[
				{"id":"wamid.one","from":"1001","type":"text","text":{"body":"First"}},
				{"id":"wamid.two","from":"1002","type":"text","text":{"body":"Second"}}
			]}}]},
			{"changes":[{"value":{"metadata":{"phone_number_id":"12345"},"statuses":[
				{"id":"wamid.out","status":"delivered","timestamp":"1788523200","recipient_id":"1003"}
			]}}]}
		]
	}`)

	queued, err := service.Accept(context.Background(), "client_one", payload)
	if err != nil || queued != 2 || len(jobs.jobs) != 2 {
		t.Fatalf("first accept queued=%d jobs=%d err=%v", queued, len(jobs.jobs), err)
	}
	queued, err = service.Accept(context.Background(), "client_one", payload)
	if err != nil || queued != 0 || len(jobs.jobs) != 2 {
		t.Fatalf("duplicate accept queued=%d jobs=%d err=%v", queued, len(jobs.jobs), err)
	}
	if len(deliveries.deliveries) != 6 {
		t.Fatalf("expected every status and inbound delivery to be upserted, got %d", len(deliveries.deliveries))
	}

	var first WhatsAppInboundJob
	if err := json.Unmarshal(jobs.jobs["wamid.one"], &first); err != nil {
		t.Fatalf("decode job: %v", err)
	}
	if first.PhoneNumberID != "12345" || first.From != "1001" || first.Message != "First" {
		t.Fatalf("unexpected first job: %#v", first)
	}
}

func TestWhatsAppCloudClientSendsBearerAuthenticatedText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v-test/12345/messages" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret-token" {
			t.Errorf("missing bearer token")
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"to":"1001"`) || !strings.Contains(string(body), `"body":"Hello"`) {
			t.Errorf("unexpected body %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"messages":[{"id":"wamid.out"}]}`))
	}))
	defer server.Close()

	client := NewWhatsAppCloudClient(server.URL+"/v-test", "secret-token")
	messageID, err := client.SendText(context.Background(), "12345", "1001", "Hello")
	if err != nil || messageID != "wamid.out" {
		t.Fatalf("send message ID=%q err=%v", messageID, err)
	}
}
