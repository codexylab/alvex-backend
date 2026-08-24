package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/codexylab/alvex-backend/internal/models"
	"github.com/codexylab/alvex-backend/internal/repository"
)

// StripeWebhookEvent represents minimal Stripe event payload structure.
type StripeWebhookEvent struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Data struct {
		Object json.RawMessage `json:"object"`
	} `json:"data"`
}

// StripeInvoiceObject represents the invoice payload from Stripe webhook.
type StripeInvoiceObject struct {
	ID             string `json:"id"`
	Customer       string `json:"customer"`
	Subscription   string `json:"subscription"`
	AmountPaid     int64  `json:"amount_paid"`
	Status         string `json:"status"`
	CustomerEmail  string `json:"customer_email"`
}

// StripeSubscriptionObject represents the subscription payload from Stripe webhook.
type StripeSubscriptionObject struct {
	ID       string `json:"id"`
	Customer string `json:"customer"`
	Status   string `json:"status"`
}

// StripeService handles automated billing, webhooks, and subscription lifecycle events.
type StripeService struct {
	BillingRepo repository.BillingRepository
	ClientRepo  repository.ClientRepository
}

// NewStripeService creates a new StripeService instance.
func NewStripeService(billingRepo repository.BillingRepository, clientRepo repository.ClientRepository) *StripeService {
	return &StripeService{
		BillingRepo: billingRepo,
		ClientRepo:  clientRepo,
	}
}

// HandleWebhook processes inbound Stripe webhook events and updates database state.
func (s *StripeService) HandleWebhook(ctx context.Context, payload []byte) error {
	var event StripeWebhookEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("invalid stripe webhook event payload: %w", err)
	}

	slog.Info("processing stripe webhook event", "type", event.Type, "id", event.ID)

	switch event.Type {
	case "invoice.paid", "invoice.payment_succeeded":
		var inv StripeInvoiceObject
		if err := json.Unmarshal(event.Data.Object, &inv); err == nil {
			slog.Info("stripe invoice paid", "stripe_invoice_id", inv.ID, "customer", inv.Customer)
			// Mark corresponding invoice paid in database if matching invoice ID or customer
			if inv.ID != "" {
				_, _ = s.BillingRepo.MarkPaid(ctx, inv.ID, time.Now())
			}
		}

	case "invoice.payment_failed":
		var inv StripeInvoiceObject
		if err := json.Unmarshal(event.Data.Object, &inv); err == nil {
			slog.Warn("stripe payment failed for customer", "customer", inv.Customer)
		}

	case "customer.subscription.deleted":
		var sub StripeSubscriptionObject
		if err := json.Unmarshal(event.Data.Object, &sub); err == nil {
			slog.Warn("stripe subscription cancelled", "subscription_id", sub.ID, "customer", sub.Customer)
		}

	default:
		slog.Debug("unhandled stripe webhook event", "type", event.Type)
	}

	return nil
}

// UpdateClientStripeInfo associates a client with their Stripe Customer and Subscription IDs.
func (s *StripeService) UpdateClientStripeInfo(ctx context.Context, clientID, stripeCustID, stripeSubID string) error {
	fields := map[string]interface{}{
		"stripe_customer_id":     stripeCustID,
		"stripe_subscription_id": stripeSubID,
		"updated_at":             time.Now(),
	}
	_, err := s.ClientRepo.UpdateFields(ctx, clientID, fields)
	return err
}

// GetPlanPricing returns standard pricing details for subscription plans.
func (s *StripeService) GetPlanPricing(plan models.BillingPlan) float64 {
	switch plan {
	case models.BillingEnterprise:
		return 499.00
	case models.BillingPro:
		return 99.00
	default:
		return 29.00
	}
}
