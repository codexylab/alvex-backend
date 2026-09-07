package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/codexylab/alvex-backend/pkg/models"
	"github.com/codexylab/alvex-backend/pkg/repository"
)

const WhatsAppInboundJobType = "whatsapp.inbound_message"

var ErrInvalidWhatsAppWebhook = errors.New("invalid WhatsApp webhook")

type whatsAppWebhookPayload struct {
	Object string `json:"object"`
	Entry  []struct {
		Changes []struct {
			Value struct {
				Metadata struct {
					PhoneNumberID string `json:"phone_number_id"`
				} `json:"metadata"`
				Messages []struct {
					ID   string `json:"id"`
					From string `json:"from"`
					Type string `json:"type"`
					Text struct {
						Body string `json:"body"`
					} `json:"text"`
				} `json:"messages"`
				Statuses []struct {
					ID          string `json:"id"`
					Status      string `json:"status"`
					Timestamp   string `json:"timestamp"`
					RecipientID string `json:"recipient_id"`
					Errors      []struct {
						Code      int    `json:"code"`
						Title     string `json:"title"`
						Message   string `json:"message"`
						ErrorData struct {
							Details string `json:"details"`
						} `json:"error_data"`
					} `json:"errors"`
				} `json:"statuses"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

type WhatsAppInboundJob struct {
	ClientID          string `json:"client_id"`
	ProviderMessageID string `json:"provider_message_id"`
	PhoneNumberID     string `json:"phone_number_id"`
	From              string `json:"from"`
	Message           string `json:"message"`
	Reply             string `json:"reply,omitempty"`
}

type WhatsAppWebhookService struct {
	Jobs       repository.BackgroundJobRepository
	Deliveries repository.WhatsAppDeliveryRepository
	Clients    repository.WhatsAppClientVerifier
}

func NewWhatsAppWebhookService(
	jobs repository.BackgroundJobRepository,
	deliveries repository.WhatsAppDeliveryRepository,
	clients repository.WhatsAppClientVerifier,
) *WhatsAppWebhookService {
	return &WhatsAppWebhookService{Jobs: jobs, Deliveries: deliveries, Clients: clients}
}

// Accept validates every change in a Meta webhook, tracks delivery statuses,
// and durably queues each unique inbound text message before HTTP acknowledgement.
func (s *WhatsAppWebhookService) Accept(ctx context.Context, clientID string, body []byte) (int, error) {
	var payload whatsAppWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, fmt.Errorf("%w: decode payload: %v", ErrInvalidWhatsAppWebhook, err)
	}
	if payload.Object != "whatsapp_business_account" {
		return 0, fmt.Errorf("%w: unsupported object", ErrInvalidWhatsAppWebhook)
	}

	queued := 0
	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			if len(change.Value.Messages) > 0 || len(change.Value.Statuses) > 0 {
				ownsPhoneNumber, err := s.Clients.ClientOwnsPhoneNumber(
					ctx,
					clientID,
					change.Value.Metadata.PhoneNumberID,
				)
				if err != nil {
					return queued, fmt.Errorf("verify WhatsApp phone number ownership: %w", err)
				}
				if !ownsPhoneNumber {
					return queued, fmt.Errorf("%w: phone number is not registered to this client", ErrInvalidWhatsAppWebhook)
				}
			}
			for _, status := range change.Value.Statuses {
				if status.ID == "" || status.Status == "" {
					continue
				}
				delivery := repository.WhatsAppDelivery{
					ProviderMessageID: status.ID,
					ClientID:          clientID,
					Recipient:         status.RecipientID,
					Direction:         "outbound",
					Status:            status.Status,
					ProviderTimestamp: parseWhatsAppTimestamp(status.Timestamp),
				}
				if len(status.Errors) > 0 {
					delivery.ErrorCode = strconv.Itoa(status.Errors[0].Code)
					delivery.ErrorMessage = firstNonEmpty(
						status.Errors[0].ErrorData.Details,
						status.Errors[0].Message,
						status.Errors[0].Title,
					)
				}
				if err := s.Deliveries.Upsert(ctx, delivery); err != nil {
					return queued, fmt.Errorf("record WhatsApp delivery status: %w", err)
				}
			}

			for _, message := range change.Value.Messages {
				text := strings.TrimSpace(message.Text.Body)
				if message.ID == "" || message.From == "" {
					continue
				}
				if message.Type != "text" || text == "" {
					if err := s.Deliveries.Upsert(ctx, repository.WhatsAppDelivery{
						ProviderMessageID: message.ID,
						ClientID:          clientID,
						Recipient:         message.From,
						Direction:         "inbound",
						Status:            "unsupported",
					}); err != nil {
						return queued, fmt.Errorf("record unsupported WhatsApp message: %w", err)
					}
					continue
				}
				jobPayload := WhatsAppInboundJob{
					ClientID:          clientID,
					ProviderMessageID: message.ID,
					PhoneNumberID:     change.Value.Metadata.PhoneNumberID,
					From:              message.From,
					Message:           text,
				}
				encoded, err := json.Marshal(jobPayload)
				if err != nil {
					return queued, fmt.Errorf("encode WhatsApp job: %w", err)
				}
				if err := s.Deliveries.Upsert(ctx, repository.WhatsAppDelivery{
					ProviderMessageID: message.ID,
					ClientID:          clientID,
					Recipient:         message.From,
					Direction:         "inbound",
					Status:            "received",
				}); err != nil {
					return queued, fmt.Errorf("record inbound WhatsApp message: %w", err)
				}
				created, err := s.Jobs.Enqueue(ctx, WhatsAppInboundJobType, message.ID, encoded, 6)
				if err != nil {
					if errors.Is(err, repository.ErrBackgroundJobPayloadMismatch) {
						return queued, fmt.Errorf("%w: duplicate message payload mismatch", ErrInvalidWhatsAppWebhook)
					}
					return queued, fmt.Errorf("enqueue WhatsApp message: %w", err)
				}
				if created {
					queued++
				}
			}
		}
	}
	return queued, nil
}

type WhatsAppMessageSender interface {
	SendText(ctx context.Context, phoneNumberID, recipient, message string) (string, error)
}

type WhatsAppJobProcessor struct {
	Jobs       repository.BackgroundJobRepository
	Deliveries repository.WhatsAppDeliveryRepository
	Chat       *ChatService
	Sender     WhatsAppMessageSender
}

func (p *WhatsAppJobProcessor) Process(ctx context.Context, job repository.BackgroundJob) error {
	var payload WhatsAppInboundJob
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("decode WhatsApp background job: %w", err)
	}
	if payload.ClientID == "" || payload.ProviderMessageID == "" || payload.PhoneNumberID == "" || payload.From == "" {
		return fmt.Errorf("WhatsApp background job is missing required fields")
	}

	if payload.Reply == "" {
		payload.Reply = truncateWhatsAppText(p.Chat.ProcessMessage(
			ctx,
			payload.ClientID,
			payload.From,
			payload.From,
			payload.Message,
			"",
			string(models.ChannelWhatsApp),
		))
		encoded, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode generated WhatsApp reply: %w", err)
		}
		if err := p.Jobs.UpdatePayload(ctx, job.ID, encoded); err != nil {
			return fmt.Errorf("persist generated WhatsApp reply: %w", err)
		}
	}

	providerMessageID, err := p.Sender.SendText(ctx, payload.PhoneNumberID, payload.From, payload.Reply)
	if err != nil {
		return fmt.Errorf("send WhatsApp reply: %w", err)
	}
	if err := p.Deliveries.Upsert(ctx, repository.WhatsAppDelivery{
		ProviderMessageID: providerMessageID,
		ClientID:          payload.ClientID,
		Recipient:         payload.From,
		Direction:         "outbound",
		Status:            "accepted",
	}); err != nil {
		return fmt.Errorf("record outbound WhatsApp reply: %w", err)
	}
	return p.Deliveries.Upsert(ctx, repository.WhatsAppDelivery{
		ProviderMessageID: payload.ProviderMessageID,
		ClientID:          payload.ClientID,
		Recipient:         payload.From,
		Direction:         "inbound",
		Status:            "processed",
	})
}

type WhatsAppCloudClient struct {
	baseURL     string
	accessToken string
	httpClient  *http.Client
}

func NewWhatsAppCloudClient(baseURL, accessToken string) *WhatsAppCloudClient {
	return &WhatsAppCloudClient{
		baseURL:     strings.TrimRight(baseURL, "/"),
		accessToken: strings.TrimSpace(accessToken),
		httpClient:  &http.Client{Timeout: 10 * time.Second},
	}
}

var whatsAppPhoneNumberIDPattern = regexp.MustCompile(`^[0-9]+$`)

func (c *WhatsAppCloudClient) SendText(
	ctx context.Context,
	phoneNumberID string,
	recipient string,
	message string,
) (string, error) {
	if c.baseURL == "" || c.accessToken == "" {
		return "", fmt.Errorf("WhatsApp Cloud API is not configured")
	}
	if !whatsAppPhoneNumberIDPattern.MatchString(phoneNumberID) || strings.TrimSpace(recipient) == "" || strings.TrimSpace(message) == "" {
		return "", fmt.Errorf("invalid WhatsApp send request")
	}
	endpoint, err := url.JoinPath(c.baseURL, phoneNumberID, "messages")
	if err != nil {
		return "", fmt.Errorf("build WhatsApp endpoint: %w", err)
	}
	body, err := json.Marshal(map[string]any{
		"messaging_product": "whatsapp",
		"recipient_type":    "individual",
		"to":                recipient,
		"type":              "text",
		"text": map[string]string{
			"body": message,
		},
	})
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+c.accessToken)
	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return "", err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("WhatsApp Cloud API returned HTTP %d: %s", response.StatusCode, truncateJobErrorText(string(responseBody)))
	}

	var result struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(responseBody, &result); err != nil || len(result.Messages) == 0 || result.Messages[0].ID == "" {
		return "", fmt.Errorf("WhatsApp Cloud API returned an invalid response")
	}
	return result.Messages[0].ID, nil
}

func parseWhatsAppTimestamp(value string) *time.Time {
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil
	}
	timestamp := time.Unix(seconds, 0).UTC()
	return &timestamp
}

func truncateWhatsAppText(value string) string {
	const maxRunes = 4_000
	if utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxRunes])
}

func truncateJobErrorText(value string) string {
	const maxLength = 500
	value = strings.TrimSpace(value)
	if len(value) <= maxLength {
		return value
	}
	return value[:maxLength]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
