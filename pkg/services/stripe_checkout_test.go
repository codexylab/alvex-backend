package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/codexylab/alvex-backend/pkg/models"
	"github.com/codexylab/alvex-backend/pkg/repository"
)

func TestStripeCheckoutClientUsesConfiguredPriceAndIdempotency(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, _, ok := r.BasicAuth()
		if !ok || username != "sk_test_secret" {
			t.Fatal("missing Stripe secret authentication")
		}
		if r.Header.Get("Idempotency-Key") != "signup_one" {
			t.Fatalf("unexpected idempotency key %q", r.Header.Get("Idempotency-Key"))
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse Stripe form: %v", err)
		}
		if r.Form.Get("mode") != "subscription" || r.Form.Get("line_items[0][price]") != "price_basic" {
			t.Fatalf("unexpected checkout form: %#v", r.Form)
		}
		if r.Form.Get("metadata[signup_id]") != "signup_one" {
			t.Fatal("signup reconciliation metadata is missing")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"cs_test_one","url":"https://checkout.stripe.com/c/pay/test"}`))
	}))
	defer server.Close()

	client := NewStripeCheckoutClient("sk_test_secret")
	client.endpoint = server.URL
	session, err := client.CreateSubscriptionSession(context.Background(), StripeCheckoutInput{
		SignupID:      "signup_one",
		AppUserID:     "user_one",
		CustomerEmail: "owner@example.com",
		PriceID:       "price_basic",
		SuccessURL:    "https://app.example.com/success",
		CancelURL:     "https://app.example.com/cancel",
	})
	if err != nil {
		t.Fatalf("create checkout: %v", err)
	}
	if session.ID != "cs_test_one" {
		t.Fatalf("unexpected session: %#v", session)
	}
}

type checkoutGatewayStub struct {
	input StripeCheckoutInput
}

func (s *checkoutGatewayStub) CreateSubscriptionSession(_ context.Context, input StripeCheckoutInput) (*StripeCheckoutSession, error) {
	s.input = input
	return &StripeCheckoutSession{ID: "cs_test", URL: "https://checkout.stripe.com/test"}, nil
}

type checkoutSignupRepositorySpy struct {
	signup    repository.CheckoutSignup
	sessionID string
}

func (s *checkoutSignupRepositorySpy) Create(_ context.Context, signup repository.CheckoutSignup) error {
	s.signup = signup
	return nil
}

func (s *checkoutSignupRepositorySpy) GetByID(_ context.Context, _ string) (*repository.CheckoutSignup, error) {
	return &s.signup, nil
}

func (s *checkoutSignupRepositorySpy) MarkCheckoutCreated(_ context.Context, _, sessionID string) error {
	s.sessionID = sessionID
	return nil
}

func (s *checkoutSignupRepositorySpy) MarkFailed(context.Context, string, string) error { return nil }

func TestSelfServiceCheckoutPersistsSignupBeforeStripeSession(t *testing.T) {
	gateway := &checkoutGatewayStub{}
	repo := &checkoutSignupRepositorySpy{}
	service := NewSelfServiceCheckoutService(
		gateway,
		repo,
		map[models.BillingPlan]string{models.BillingBasic: "price_basic"},
		"https://app.example.com/",
	)

	session, err := service.Create(context.Background(), SelfServiceCheckoutRequest{
		OrganizationName: "Example Company",
		ClientName:       "Example Assistant",
		Domain:           "https://example.com",
		Provider:         models.ProviderGemini,
		Model:            "Gemini 2.0 Flash",
		BillingPlan:      models.BillingBasic,
	}, "user_one", "owner@example.com", "Owner")
	if err != nil {
		t.Fatalf("create self-service checkout: %v", err)
	}
	if repo.signup.ID == "" || gateway.input.SignupID != repo.signup.ID {
		t.Fatal("checkout session was not reconciled to persisted signup")
	}
	if gateway.input.PriceID != "price_basic" || repo.sessionID != session.ID {
		t.Fatalf("unexpected checkout state: input=%#v session=%#v", gateway.input, session)
	}
}
