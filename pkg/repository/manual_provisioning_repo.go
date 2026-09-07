package repository

import (
	"context"
	"time"

	"github.com/codexylab/alvex-backend/pkg/database"
)

// ManualClientProvisioning contains validated values for one atomic tenant setup.
type ManualClientProvisioning struct {
	InvitationID     string
	InvitedBy        string
	UserID           string
	Email            string
	UserName         string
	OrganizationID   string
	OrganizationName string
	OrganizationSlug string
	ClientID         string
	ClientName       string
	Domain           string
	Provider         string
	Model            string
	BillingPlan      string
	SystemPersona    string
	WebhookURL       string
	InviteExpiresAt  time.Time
}

type ManualProvisioningRepository interface {
	ProvisionInvitedClient(ctx context.Context, input ManualClientProvisioning) error
}

type SQLManualProvisioningRepository struct {
	DB *database.DB
}

func NewSQLManualProvisioningRepository(db *database.DB) *SQLManualProvisioningRepository {
	return &SQLManualProvisioningRepository{DB: db}
}

// ProvisionInvitedClient atomically creates the identity mapping, tenant,
// assistant, and authorization memberships after Supabase sends the invite.
func (r *SQLManualProvisioningRepository) ProvisionInvitedClient(
	ctx context.Context,
	input ManualClientProvisioning,
) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	statements := []struct {
		query string
		args  []interface{}
	}{
		{
			query: `INSERT INTO app_users
				(id, supabase_user_id, email, name, platform_role, status)
				VALUES ($1, $2, $3, $4, 'platform_user', 'active')
				ON CONFLICT (supabase_user_id) DO UPDATE
				SET email = EXCLUDED.email, name = EXCLUDED.name, updated_at = CURRENT_TIMESTAMP`,
			args: []interface{}{input.UserID, input.UserID, input.Email, input.UserName},
		},
		{
			query: `INSERT INTO organizations (id, name, slug, type, status)
				VALUES ($1, $2, $3, 'direct_customer', 'active')`,
			args: []interface{}{input.OrganizationID, input.OrganizationName, input.OrganizationSlug},
		},
		{
			query: `INSERT INTO organization_memberships
				(organization_id, user_id, role, status)
				VALUES ($1, $2, 'owner', 'invited')`,
			args: []interface{}{input.OrganizationID, input.UserID},
		},
		{
			query: `INSERT INTO clients
				(id, organization_id, name, domain, status, provider, model,
				 system_persona, webhook_url, billing_plan, owner_id, onboarding_status)
				VALUES ($1, $2, $3, $4, 'Active', $5, $6, $7, $8, $9, $10, 'pending')`,
			args: []interface{}{
				input.ClientID, input.OrganizationID, input.ClientName, input.Domain,
				input.Provider, input.Model, input.SystemPersona,
				input.WebhookURL, input.BillingPlan, input.UserID,
			},
		},
		{
			query: `INSERT INTO client_memberships (client_id, user_id, role, status)
				VALUES ($1, $2, 'client_admin', 'invited')`,
			args: []interface{}{input.ClientID, input.UserID},
		},
		{
			query: `INSERT INTO identity_invitations
				(id, email, supabase_user_id, client_id, invited_by, role, status, expires_at)
				VALUES ($1, $2, $3, $4, $5, 'client_admin', 'sent', $6)`,
			args: []interface{}{
				input.InvitationID, input.Email, input.UserID, input.ClientID,
				input.InvitedBy, input.InviteExpiresAt,
			},
		},
	}

	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, r.DB.Adapt(statement.query), statement.args...); err != nil {
			return err
		}
	}
	return tx.Commit()
}
