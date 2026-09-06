package services

import (
	"context"
	"testing"
	"time"

	"github.com/codexylab/alvex-backend/pkg/models"
	"github.com/codexylab/alvex-backend/pkg/repository"
)

type identityInviterStub struct {
	identity *InvitedIdentity
	redirect string
}

func (s *identityInviterStub) InviteUser(_ context.Context, _, _, redirectURL string) (*InvitedIdentity, error) {
	s.redirect = redirectURL
	return s.identity, nil
}

type manualProvisioningRepositorySpy struct {
	input repository.ManualClientProvisioning
}

func (s *manualProvisioningRepositorySpy) ProvisionInvitedClient(_ context.Context, input repository.ManualClientProvisioning) error {
	s.input = input
	return nil
}

func TestManualOnboardingInvitesAndBuildsAtomicProvisioning(t *testing.T) {
	inviter := &identityInviterStub{identity: &InvitedIdentity{ID: "supabase-user", Email: "owner@example.com"}}
	repo := &manualProvisioningRepositorySpy{}
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	service := NewManualOnboardingService(
		inviter,
		repo,
		"https://app.example.com/",
		"https://api.example.com/",
	)
	service.Now = func() time.Time { return now }

	result, err := service.InviteAndProvision(context.Background(), ManualClientRequest{
		Email:            " OWNER@EXAMPLE.COM ",
		ContactName:      "Owner",
		OrganizationName: "Example Company",
		ClientName:       "Example Assistant",
		Domain:           "https://example.com",
		Provider:         models.ProviderGemini,
		Model:            "Gemini 2.0 Flash",
		BillingPlan:      models.BillingBasic,
	}, "platform-admin")
	if err != nil {
		t.Fatalf("invite and provision: %v", err)
	}
	if inviter.redirect != "https://app.example.com/auth/callback?next=/set-password" {
		t.Fatalf("unexpected redirect %q", inviter.redirect)
	}
	if result.ClientID == "" || result.OrganizationID == "" || result.Invitation != "sent" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if repo.input.UserID != "supabase-user" || repo.input.InvitedBy != "platform-admin" {
		t.Fatalf("unexpected provisioning identity: %#v", repo.input)
	}
	if !repo.input.InviteExpiresAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("unexpected invite expiry: %v", repo.input.InviteExpiresAt)
	}
}

func TestManualOnboardingRejectsInvalidDomainBeforeInvitation(t *testing.T) {
	service := NewManualOnboardingService(
		&identityInviterStub{identity: &InvitedIdentity{ID: "unused", Email: "owner@example.com"}},
		&manualProvisioningRepositorySpy{},
		"https://app.example.com",
		"https://api.example.com",
	)
	_, err := service.InviteAndProvision(context.Background(), ManualClientRequest{
		Email:            "owner@example.com",
		ContactName:      "Owner",
		OrganizationName: "Example Company",
		ClientName:       "Example Assistant",
		Domain:           "example.com",
		Provider:         models.ProviderGemini,
		Model:            "Gemini 2.0 Flash",
		BillingPlan:      models.BillingBasic,
	}, "platform-admin")
	if err == nil {
		t.Fatal("expected invalid domain to be rejected")
	}
}
