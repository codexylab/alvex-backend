package services

import (
	"context"
	"fmt"
	"testing"

	"github.com/codexylab/alvex-backend/pkg/repository"
)

type stripeEventRepositoryStub struct {
	claimed   bool
	completed int
	failed    int
}

func (s *stripeEventRepositoryStub) ClaimEvent(context.Context, string, string, string) (bool, error) {
	if s.claimed {
		return false, nil
	}
	s.claimed = true
	return true, nil
}

func (s *stripeEventRepositoryStub) CompleteEvent(context.Context, string) error {
	s.completed++
	return nil
}

func (s *stripeEventRepositoryStub) FailEvent(context.Context, string, string) error {
	s.failed++
	return nil
}

type stripeProvisioningRepositorySpy struct {
	input   repository.CheckoutProvisioningInput
	invoice repository.StripeInvoiceInput
	calls   int
}

func (s *stripeProvisioningRepositorySpy) ProvisionPaidSignup(_ context.Context, input repository.CheckoutProvisioningInput) error {
	s.input = input
	s.calls++
	return nil
}

func (s *stripeProvisioningRepositorySpy) UpdateSubscription(context.Context, string, string, string, bool) error {
	return nil
}
func (s *stripeProvisioningRepositorySpy) UpsertInvoice(_ context.Context, input repository.StripeInvoiceInput) error {
	s.invoice = input
	return nil
}

type checkoutSignupLookupStub struct {
	signup repository.CheckoutSignup
}

func (s checkoutSignupLookupStub) Create(context.Context, repository.CheckoutSignup) error {
	return nil
}
func (s checkoutSignupLookupStub) GetByID(_ context.Context, id string) (*repository.CheckoutSignup, error) {
	if id != s.signup.ID {
		return nil, fmt.Errorf("signup not found")
	}
	return &s.signup, nil
}
func (s checkoutSignupLookupStub) MarkCheckoutCreated(context.Context, string, string) error {
	return nil
}
func (s checkoutSignupLookupStub) MarkFailed(context.Context, string, string) error { return nil }

func TestStripeServiceProvisionsPaidCheckoutOnce(t *testing.T) {
	events := &stripeEventRepositoryStub{}
	provisioning := &stripeProvisioningRepositorySpy{}
	checkouts := checkoutSignupLookupStub{signup: repository.CheckoutSignup{
		ID:               "11111111-2222-3333-4444-555555555555",
		AppUserID:        "user_one",
		OrganizationName: "Example Company",
		ClientName:       "Example Assistant",
		Domain:           "https://example.com",
	}}
	service := NewStripeService(events, provisioning, checkouts, nil, "https://api.alvex.example")
	payload := []byte(`{
		"id":"evt_checkout",
		"type":"checkout.session.completed",
		"data":{"object":{
			"id":"cs_test",
			"customer":"cus_test",
			"subscription":"sub_test",
			"client_reference_id":"user_one",
			"payment_status":"paid",
			"metadata":{"signup_id":"11111111-2222-3333-4444-555555555555"}
		}}
	}`)

	if err := service.HandleWebhook(context.Background(), payload); err != nil {
		t.Fatalf("process checkout: %v", err)
	}
	if err := service.HandleWebhook(context.Background(), payload); err != nil {
		t.Fatalf("process duplicate checkout: %v", err)
	}
	if provisioning.calls != 1 || events.completed != 1 || events.failed != 0 {
		t.Fatalf("unexpected webhook processing counts: provisioning=%d completed=%d failed=%d", provisioning.calls, events.completed, events.failed)
	}
	if provisioning.input.ClientID != "example-assistant-11111111" || provisioning.input.SubscriptionID != "sub_test" {
		t.Fatalf("unexpected provisioning input: %#v", provisioning.input)
	}
}

func TestStripeServiceDurablyAcceptsWebhookBeforeProcessing(t *testing.T) {
	jobs := &whatsAppJobRepositoryStub{}
	service := NewStripeService(nil, nil, nil, jobs, "https://api.alvex.example")
	payload := []byte(`{"id":"evt_one","type":"checkout.session.completed","data":{"object":{}}}`)

	if err := service.AcceptWebhook(context.Background(), payload); err != nil {
		t.Fatalf("accept webhook: %v", err)
	}
	if err := service.AcceptWebhook(context.Background(), payload); err != nil {
		t.Fatalf("accept duplicate webhook: %v", err)
	}
	if len(jobs.jobs) != 1 || string(jobs.jobs["evt_one"]) != string(payload) {
		t.Fatalf("expected one durable event job, got %#v", jobs.jobs)
	}
}

func TestStripeServiceAppliesCurrentInvoiceParentContract(t *testing.T) {
	events := &stripeEventRepositoryStub{}
	provisioning := &stripeProvisioningRepositorySpy{}
	service := NewStripeService(events, provisioning, checkoutSignupLookupStub{}, nil, "https://api.alvex.example")
	payload := []byte(`{
		"id":"evt_invoice_paid",
		"type":"invoice.paid",
		"data":{"object":{
			"id":"in_paid","customer":"cus_one","status":"paid","currency":"usd","total":1299,
			"parent":{"type":"subscription_details","subscription_details":{"subscription":"sub_one"}},
			"status_transitions":{"paid_at":1788523200}
		}}
	}`)

	if err := service.HandleWebhook(context.Background(), payload); err != nil {
		t.Fatalf("process invoice: %v", err)
	}
	if provisioning.invoice.ProviderInvoiceID != "in_paid" ||
		provisioning.invoice.ProviderSubscription != "sub_one" ||
		provisioning.invoice.InternalStatus != "Paid" ||
		provisioning.invoice.Amount != 12.99 || provisioning.invoice.Currency != "USD" {
		t.Fatalf("unexpected invoice input: %#v", provisioning.invoice)
	}
}

func TestStripeServiceRejectsInvalidInvoiceIdentifiersWithoutMutation(t *testing.T) {
	events := &stripeEventRepositoryStub{}
	provisioning := &stripeProvisioningRepositorySpy{}
	service := NewStripeService(events, provisioning, checkoutSignupLookupStub{}, nil, "https://api.alvex.example")
	payload := []byte(`{
		"id":"evt_invalid_invoice",
		"type":"invoice.paid",
		"data":{"object":{
			"id":"not-an-invoice","customer":"cus_one","status":"paid","currency":"usd","total":1299,
			"parent":{"type":"subscription_details","subscription_details":{"subscription":"sub_one"}}
		}}
	}`)

	if err := service.HandleWebhook(context.Background(), payload); err == nil {
		t.Fatal("expected invalid invoice identifier to be rejected")
	}
	if provisioning.invoice.ProviderInvoiceID != "" {
		t.Fatalf("invalid invoice reached repository: %#v", provisioning.invoice)
	}
	if events.completed != 0 || events.failed != 1 {
		t.Fatalf("unexpected event state: completed=%d failed=%d", events.completed, events.failed)
	}
}
