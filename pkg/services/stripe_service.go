package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/codexylab/alvex-backend/pkg/crypto"
	"github.com/codexylab/alvex-backend/pkg/repository"
)

const StripeWebhookJobType = "stripe.webhook_event"

var ErrInvalidStripeWebhook = errors.New("invalid Stripe webhook")

type StripeWebhookEvent struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Data struct {
		Object json.RawMessage `json:"object"`
	} `json:"data"`
}

type StripeCheckoutCompletedObject struct {
	ID                string            `json:"id"`
	Customer          string            `json:"customer"`
	Subscription      string            `json:"subscription"`
	ClientReferenceID string            `json:"client_reference_id"`
	PaymentStatus     string            `json:"payment_status"`
	Metadata          map[string]string `json:"metadata"`
}

type StripeSubscriptionObject struct {
	ID                string `json:"id"`
	Customer          string `json:"customer"`
	Status            string `json:"status"`
	CancelAtPeriodEnd bool   `json:"cancel_at_period_end"`
}

type StripeInvoiceObject struct {
	ID           string `json:"id"`
	Customer     string `json:"customer"`
	Subscription string `json:"subscription"` // Legacy Stripe API versions.
	Status       string `json:"status"`
	Currency     string `json:"currency"`
	Total        int64  `json:"total"`
	DueDate      int64  `json:"due_date"`
	Parent       struct {
		Type                string `json:"type"`
		SubscriptionDetails struct {
			Subscription string `json:"subscription"`
		} `json:"subscription_details"`
	} `json:"parent"`
	StatusTransitions struct {
		PaidAt int64 `json:"paid_at"`
	} `json:"status_transitions"`
}

// StripeService owns idempotent webhook dispatch and paid-signup provisioning.
type StripeService struct {
	Events       repository.StripeEventRepository
	Provisioning repository.StripeProvisioningRepository
	Checkouts    repository.CheckoutSignupRepository
	Jobs         repository.BackgroundJobRepository
	PublicAPIURL string
}

func NewStripeService(
	events repository.StripeEventRepository,
	provisioning repository.StripeProvisioningRepository,
	checkouts repository.CheckoutSignupRepository,
	jobs repository.BackgroundJobRepository,
	publicAPIURL string,
) *StripeService {
	return &StripeService{
		Events:       events,
		Provisioning: provisioning,
		Checkouts:    checkouts,
		Jobs:         jobs,
		PublicAPIURL: publicAPIURL,
	}
}

// AcceptWebhook validates the minimal event envelope and durably queues the raw
// JSON before the HTTP handler acknowledges Stripe.
func (s *StripeService) AcceptWebhook(ctx context.Context, payload []byte) error {
	var event StripeWebhookEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("%w: decode event payload: %v", ErrInvalidStripeWebhook, err)
	}
	if event.ID == "" || event.Type == "" {
		return fmt.Errorf("%w: event ID and type are required", ErrInvalidStripeWebhook)
	}
	if s.Jobs == nil {
		return fmt.Errorf("Stripe webhook queue is not configured")
	}
	_, err := s.Jobs.Enqueue(ctx, StripeWebhookJobType, event.ID, payload, 6)
	if err != nil {
		if errors.Is(err, repository.ErrBackgroundJobPayloadMismatch) {
			return fmt.Errorf("%w: duplicate event payload mismatch", ErrInvalidStripeWebhook)
		}
		return fmt.Errorf("enqueue Stripe webhook event: %w", err)
	}
	return nil
}

func (s *StripeService) HandleWebhook(ctx context.Context, payload []byte) error {
	var event StripeWebhookEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("invalid Stripe webhook event payload: %w", err)
	}
	if event.ID == "" || event.Type == "" {
		return fmt.Errorf("Stripe webhook event ID and type are required")
	}
	digest := sha256.Sum256(payload)
	claimed, err := s.Events.ClaimEvent(ctx, event.ID, event.Type, hex.EncodeToString(digest[:]))
	if err != nil {
		return fmt.Errorf("claim Stripe webhook event: %w", err)
	}
	if !claimed {
		return nil
	}

	if err := s.processEvent(ctx, event); err != nil {
		_ = s.Events.FailEvent(ctx, event.ID, err.Error())
		return err
	}
	if err := s.Events.CompleteEvent(ctx, event.ID); err != nil {
		return fmt.Errorf("complete Stripe webhook event: %w", err)
	}
	return nil
}

func (s *StripeService) processEvent(ctx context.Context, event StripeWebhookEvent) error {
	switch event.Type {
	case "checkout.session.completed":
		var checkout StripeCheckoutCompletedObject
		if err := json.Unmarshal(event.Data.Object, &checkout); err != nil {
			return fmt.Errorf("decode completed Checkout session: %w", err)
		}
		return s.provisionCheckout(ctx, checkout)
	case "customer.subscription.created", "customer.subscription.updated", "customer.subscription.deleted":
		var subscription StripeSubscriptionObject
		if err := json.Unmarshal(event.Data.Object, &subscription); err != nil {
			return fmt.Errorf("decode Stripe subscription: %w", err)
		}
		if subscription.ID == "" || subscription.Customer == "" || subscription.Status == "" {
			return fmt.Errorf("Stripe subscription event is missing required fields")
		}
		return s.Provisioning.UpdateSubscription(
			ctx,
			subscription.ID,
			subscription.Customer,
			subscription.Status,
			subscription.CancelAtPeriodEnd,
		)
	case "invoice.paid", "invoice.payment_failed":
		var invoice StripeInvoiceObject
		if err := json.Unmarshal(event.Data.Object, &invoice); err != nil {
			return fmt.Errorf("decode Stripe invoice: %w", err)
		}
		return s.applyInvoice(ctx, event.Type, invoice)
	default:
		return nil
	}
}

func (s *StripeService) applyInvoice(ctx context.Context, eventType string, invoice StripeInvoiceObject) error {
	subscriptionID := invoice.Subscription
	if invoice.Parent.Type == "subscription_details" && invoice.Parent.SubscriptionDetails.Subscription != "" {
		subscriptionID = invoice.Parent.SubscriptionDetails.Subscription
	}
	if !strings.HasPrefix(invoice.ID, "in_") ||
		!strings.HasPrefix(invoice.Customer, "cus_") ||
		!strings.HasPrefix(subscriptionID, "sub_") ||
		len(invoice.Currency) != 3 || invoice.Total < 0 {
		return fmt.Errorf("Stripe invoice event is missing required fields")
	}

	internalStatus := "Pending"
	providerStatus := invoice.Status
	var paidAt *time.Time
	if eventType == "invoice.paid" {
		internalStatus = "Paid"
		providerStatus = "paid"
		if invoice.StatusTransitions.PaidAt > 0 {
			value := time.Unix(invoice.StatusTransitions.PaidAt, 0).UTC()
			paidAt = &value
		}
	} else {
		providerStatus = "payment_failed"
	}
	var dueAt *time.Time
	if invoice.DueDate > 0 {
		value := time.Unix(invoice.DueDate, 0).UTC()
		dueAt = &value
	}

	return s.Provisioning.UpsertInvoice(ctx, repository.StripeInvoiceInput{
		ProviderInvoiceID:    invoice.ID,
		ProviderSubscription: subscriptionID,
		ProviderCustomer:     invoice.Customer,
		ProviderStatus:       providerStatus,
		InternalStatus:       internalStatus,
		Currency:             strings.ToUpper(invoice.Currency),
		Amount:               float64(invoice.Total) / 100,
		DueAt:                dueAt,
		PaidAt:               paidAt,
	})
}

func (s *StripeService) provisionCheckout(ctx context.Context, checkout StripeCheckoutCompletedObject) error {
	signupID := checkout.Metadata["signup_id"]
	if checkout.ID == "" || signupID == "" || checkout.Customer == "" || checkout.Subscription == "" {
		return fmt.Errorf("completed Checkout session is missing required fields")
	}
	if checkout.PaymentStatus != "paid" && checkout.PaymentStatus != "no_payment_required" {
		return fmt.Errorf("Checkout session payment is not complete")
	}

	signup, err := s.Checkouts.GetByID(ctx, signupID)
	if err != nil {
		return fmt.Errorf("load checkout signup: %w", err)
	}
	if checkout.ClientReferenceID != signup.AppUserID {
		return fmt.Errorf("Checkout session owner does not match signup")
	}

	suffix := strings.ReplaceAll(signup.ID, "-", "")
	if len(suffix) > 8 {
		suffix = suffix[:8]
	}
	clientID := crypto.SlugifyClientName(signup.ClientName) + "-" + suffix
	organizationSlug := crypto.SlugifyClientName(signup.OrganizationName) + "-" + suffix
	return s.Provisioning.ProvisionPaidSignup(ctx, repository.CheckoutProvisioningInput{
		SignupID:         signup.ID,
		SessionID:        checkout.ID,
		AppUserID:        signup.AppUserID,
		CustomerID:       checkout.Customer,
		SubscriptionID:   checkout.Subscription,
		OrganizationID:   "org_" + signup.ID,
		OrganizationSlug: organizationSlug,
		ClientID:         clientID,
		SystemPersona: fmt.Sprintf(
			"You are an AI representative for %s. Help visitors with inquiries on %s.",
			signup.ClientName,
			signup.Domain,
		),
		WebhookURL: crypto.GenerateWebhookURL(s.PublicAPIURL, clientID),
	})
}
