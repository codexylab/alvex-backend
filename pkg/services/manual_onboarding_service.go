package services

import (
	"context"
	"fmt"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/codexylab/alvex-backend/pkg/apierr"
	"github.com/codexylab/alvex-backend/pkg/crypto"
	"github.com/codexylab/alvex-backend/pkg/models"
	"github.com/codexylab/alvex-backend/pkg/repository"
)

type ManualClientRequest struct {
	Email            string             `json:"email"`
	ContactName      string             `json:"contact_name"`
	OrganizationName string             `json:"organization_name"`
	ClientName       string             `json:"client_name"`
	Domain           string             `json:"domain"`
	Provider         models.AIProvider  `json:"provider"`
	Model            string             `json:"model"`
	BillingPlan      models.BillingPlan `json:"billing_plan"`
}

type ManualClientResult struct {
	OrganizationID string `json:"organization_id"`
	ClientID       string `json:"client_id"`
	Email          string `json:"email"`
	Invitation     string `json:"invitation_status"`
}

type ManualOnboardingService struct {
	Inviter      IdentityInviter
	Repository   repository.ManualProvisioningRepository
	FrontendURL  string
	PublicAPIURL string
	Now          func() time.Time
}

func NewManualOnboardingService(
	inviter IdentityInviter,
	repo repository.ManualProvisioningRepository,
	frontendURL string,
	publicAPIURL string,
) *ManualOnboardingService {
	return &ManualOnboardingService{
		Inviter:      inviter,
		Repository:   repo,
		FrontendURL:  strings.TrimRight(frontendURL, "/"),
		PublicAPIURL: strings.TrimRight(publicAPIURL, "/"),
		Now:          time.Now,
	}
}

// InviteAndProvision creates a direct-customer tenant after Supabase accepts
// the server-side invitation. Database records are committed atomically.
func (s *ManualOnboardingService) InviteAndProvision(
	ctx context.Context,
	request ManualClientRequest,
	invitedBy string,
) (*ManualClientResult, error) {
	request.Email = strings.ToLower(strings.TrimSpace(request.Email))
	request.ContactName = strings.TrimSpace(request.ContactName)
	request.OrganizationName = strings.TrimSpace(request.OrganizationName)
	request.ClientName = strings.TrimSpace(request.ClientName)
	request.Domain = strings.TrimSpace(request.Domain)
	if err := validateManualClientRequest(request); err != nil {
		return nil, err
	}

	redirectURL := s.FrontendURL + "/auth/callback?next=/set-password"
	identity, err := s.Inviter.InviteUser(ctx, request.Email, request.ContactName, redirectURL)
	if err != nil {
		return nil, err
	}

	suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	clientID := crypto.SlugifyClientName(request.ClientName) + "-" + suffix
	organizationSlug := crypto.SlugifyClientName(request.OrganizationName) + "-" + suffix
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	input := repository.ManualClientProvisioning{
		InvitationID:     uuid.NewString(),
		InvitedBy:        invitedBy,
		UserID:           identity.ID,
		Email:            identity.Email,
		UserName:         request.ContactName,
		OrganizationID:   "org_" + uuid.NewString(),
		OrganizationName: request.OrganizationName,
		OrganizationSlug: organizationSlug,
		ClientID:         clientID,
		ClientName:       request.ClientName,
		Domain:           request.Domain,
		Provider:         string(request.Provider),
		Model:            request.Model,
		BillingPlan:      string(request.BillingPlan),
		SystemPersona: fmt.Sprintf(
			"You are an AI representative for %s. Help visitors with inquiries on %s.",
			request.ClientName,
			request.Domain,
		),
		WebhookURL:      crypto.GenerateWebhookURL(s.PublicAPIURL, clientID),
		InviteExpiresAt: now.Add(time.Hour),
	}

	if err := s.Repository.ProvisionInvitedClient(ctx, input); err != nil {
		return nil, fmt.Errorf("provision invited client: %w", err)
	}
	return &ManualClientResult{
		OrganizationID: input.OrganizationID,
		ClientID:       input.ClientID,
		Email:          input.Email,
		Invitation:     "sent",
	}, nil
}

func validateManualClientRequest(request ManualClientRequest) error {
	if _, err := mail.ParseAddress(request.Email); err != nil {
		return &apierr.ValidationError{Message: "A valid email is required"}
	}
	if request.ContactName == "" || request.OrganizationName == "" || request.ClientName == "" {
		return &apierr.ValidationError{Message: "Contact, organization, and client names are required"}
	}
	if len(request.ContactName) > 200 || len(request.OrganizationName) > 255 || len(request.ClientName) > 255 {
		return &apierr.ValidationError{Message: "One or more names exceed the allowed length"}
	}
	parsedDomain, err := url.ParseRequestURI(request.Domain)
	if err != nil || parsedDomain.Host == "" || (parsedDomain.Scheme != "http" && parsedDomain.Scheme != "https") {
		return &apierr.ValidationError{Message: "Domain must be a complete HTTP or HTTPS URL"}
	}
	validModels, ok := models.ProviderModels[request.Provider]
	if !ok || !containsString(validModels, request.Model) {
		return &apierr.ValidationError{Message: fmt.Sprintf("Model %q is not valid for provider %q", request.Model, request.Provider)}
	}
	switch request.BillingPlan {
	case models.BillingBasic, models.BillingPro, models.BillingEnterprise:
	default:
		return &apierr.ValidationError{Message: "Billing plan must be Basic, Pro, or Enterprise"}
	}
	return nil
}
