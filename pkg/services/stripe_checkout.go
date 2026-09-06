package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/codexylab/alvex-backend/pkg/models"
	"github.com/codexylab/alvex-backend/pkg/repository"
)

var ErrStripeCheckoutNotConfigured = errors.New("Stripe Checkout is not configured")

type StripeCheckoutSession struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

type StripeCheckoutGateway interface {
	CreateSubscriptionSession(
		ctx context.Context,
		input StripeCheckoutInput,
	) (*StripeCheckoutSession, error)
}

type StripeCheckoutInput struct {
	SignupID      string
	AppUserID     string
	CustomerEmail string
	PriceID       string
	SuccessURL    string
	CancelURL     string
}

type StripeCheckoutClient struct {
	secretKey string
	endpoint  string
	http      *http.Client
}

func NewStripeCheckoutClient(secretKey string) *StripeCheckoutClient {
	return &StripeCheckoutClient{
		secretKey: secretKey,
		endpoint:  "https://api.stripe.com/v1/checkout/sessions",
		http:      &http.Client{Timeout: 10 * time.Second},
	}
}

// CreateSubscriptionSession creates a Stripe-hosted checkout using a configured
// Price ID. User-supplied amounts are never accepted.
func (c *StripeCheckoutClient) CreateSubscriptionSession(
	ctx context.Context,
	input StripeCheckoutInput,
) (*StripeCheckoutSession, error) {
	if c.secretKey == "" || input.PriceID == "" {
		return nil, ErrStripeCheckoutNotConfigured
	}

	form := url.Values{
		"mode":                                   {"subscription"},
		"customer_email":                         {input.CustomerEmail},
		"client_reference_id":                    {input.AppUserID},
		"line_items[0][price]":                   {input.PriceID},
		"line_items[0][quantity]":                {"1"},
		"success_url":                            {input.SuccessURL},
		"cancel_url":                             {input.CancelURL},
		"metadata[signup_id]":                    {input.SignupID},
		"subscription_data[metadata][signup_id]": {input.SignupID},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create Stripe Checkout request: %w", err)
	}
	request.SetBasicAuth(c.secretKey, "")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Idempotency-Key", input.SignupID)

	response, err := c.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("send Stripe Checkout request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return nil, fmt.Errorf("Stripe Checkout failed with status %d", response.StatusCode)
	}

	var session StripeCheckoutSession
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&session); err != nil {
		return nil, fmt.Errorf("decode Stripe Checkout response: %w", err)
	}
	checkoutURL, err := url.Parse(session.URL)
	if session.ID == "" || err != nil || checkoutURL.Scheme != "https" || checkoutURL.Host == "" {
		return nil, fmt.Errorf("Stripe Checkout returned an invalid session")
	}
	return &session, nil
}

type SelfServiceCheckoutRequest struct {
	OrganizationName string             `json:"organization_name"`
	ClientName       string             `json:"client_name"`
	Domain           string             `json:"domain"`
	Provider         models.AIProvider  `json:"provider"`
	Model            string             `json:"model"`
	BillingPlan      models.BillingPlan `json:"billing_plan"`
}

type SelfServiceCheckoutService struct {
	Gateway     StripeCheckoutGateway
	Repository  repository.CheckoutSignupRepository
	PriceIDs    map[models.BillingPlan]string
	FrontendURL string
}

func NewSelfServiceCheckoutService(
	gateway StripeCheckoutGateway,
	repo repository.CheckoutSignupRepository,
	priceIDs map[models.BillingPlan]string,
	frontendURL string,
) *SelfServiceCheckoutService {
	return &SelfServiceCheckoutService{
		Gateway:     gateway,
		Repository:  repo,
		PriceIDs:    priceIDs,
		FrontendURL: strings.TrimRight(frontendURL, "/"),
	}
}

func (s *SelfServiceCheckoutService) Create(
	ctx context.Context,
	request SelfServiceCheckoutRequest,
	appUserID string,
	email string,
	name string,
) (*StripeCheckoutSession, error) {
	manualRequest := ManualClientRequest{
		Email:            email,
		ContactName:      name,
		OrganizationName: strings.TrimSpace(request.OrganizationName),
		ClientName:       strings.TrimSpace(request.ClientName),
		Domain:           strings.TrimSpace(request.Domain),
		Provider:         request.Provider,
		Model:            request.Model,
		BillingPlan:      request.BillingPlan,
	}
	if err := validateManualClientRequest(manualRequest); err != nil {
		return nil, err
	}
	priceID := s.PriceIDs[request.BillingPlan]
	if priceID == "" {
		return nil, ErrStripeCheckoutNotConfigured
	}

	signup := repository.CheckoutSignup{
		ID:               uuid.NewString(),
		AppUserID:        appUserID,
		Email:            strings.ToLower(strings.TrimSpace(email)),
		OrganizationName: manualRequest.OrganizationName,
		ClientName:       manualRequest.ClientName,
		Domain:           manualRequest.Domain,
		Provider:         string(manualRequest.Provider),
		Model:            manualRequest.Model,
		BillingPlan:      string(manualRequest.BillingPlan),
	}
	if err := s.Repository.Create(ctx, signup); err != nil {
		return nil, fmt.Errorf("create checkout signup: %w", err)
	}

	session, err := s.Gateway.CreateSubscriptionSession(ctx, StripeCheckoutInput{
		SignupID:      signup.ID,
		AppUserID:     appUserID,
		CustomerEmail: signup.Email,
		PriceID:       priceID,
		SuccessURL:    s.FrontendURL + "/subscribe/success?session_id={CHECKOUT_SESSION_ID}",
		CancelURL:     s.FrontendURL + "/subscribe?checkout=cancelled",
	})
	if err != nil {
		_ = s.Repository.MarkFailed(ctx, signup.ID, "checkout_session_creation_failed")
		return nil, err
	}
	if err := s.Repository.MarkCheckoutCreated(ctx, signup.ID, session.ID); err != nil {
		return nil, fmt.Errorf("save Stripe Checkout session: %w", err)
	}
	return session, nil
}
