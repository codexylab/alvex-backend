package repository

import (
	"context"

	"github.com/codexylab/alvex-backend/pkg/database"
)

type CheckoutSignup struct {
	ID               string
	AppUserID        string
	Email            string
	OrganizationName string
	ClientName       string
	Domain           string
	Provider         string
	Model            string
	BillingPlan      string
}

type CheckoutSignupRepository interface {
	Create(ctx context.Context, signup CheckoutSignup) error
	GetByID(ctx context.Context, signupID string) (*CheckoutSignup, error)
	MarkCheckoutCreated(ctx context.Context, signupID, stripeSessionID string) error
	MarkFailed(ctx context.Context, signupID, reason string) error
}

func (r *SQLCheckoutSignupRepository) GetByID(ctx context.Context, signupID string) (*CheckoutSignup, error) {
	var signup CheckoutSignup
	err := r.DB.QueryRowContext(ctx, r.DB.Adapt(`
		SELECT id, app_user_id, email, organization_name, client_name, domain, provider, model, billing_plan
		FROM checkout_signups WHERE id = $1`), signupID).Scan(
		&signup.ID, &signup.AppUserID, &signup.Email, &signup.OrganizationName,
		&signup.ClientName, &signup.Domain, &signup.Provider, &signup.Model, &signup.BillingPlan,
	)
	if err != nil {
		return nil, err
	}
	return &signup, nil
}

type SQLCheckoutSignupRepository struct {
	DB *database.DB
}

func NewSQLCheckoutSignupRepository(db *database.DB) *SQLCheckoutSignupRepository {
	return &SQLCheckoutSignupRepository{DB: db}
}

func (r *SQLCheckoutSignupRepository) Create(ctx context.Context, signup CheckoutSignup) error {
	_, err := r.DB.ExecContext(ctx, r.DB.Adapt(`
		INSERT INTO checkout_signups
		  (id, app_user_id, email, organization_name, client_name, domain, provider, model, billing_plan)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`),
		signup.ID, signup.AppUserID, signup.Email, signup.OrganizationName,
		signup.ClientName, signup.Domain, signup.Provider, signup.Model, signup.BillingPlan,
	)
	return err
}

func (r *SQLCheckoutSignupRepository) MarkCheckoutCreated(
	ctx context.Context,
	signupID string,
	stripeSessionID string,
) error {
	_, err := r.DB.ExecContext(ctx, r.DB.Adapt(`
		UPDATE checkout_signups
		SET stripe_session_id = $1, status = 'checkout_created', last_error = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE id = $2 AND status = 'pending'`), stripeSessionID, signupID)
	return err
}

func (r *SQLCheckoutSignupRepository) MarkFailed(ctx context.Context, signupID, reason string) error {
	_, err := r.DB.ExecContext(ctx, r.DB.Adapt(`
		UPDATE checkout_signups
		SET status = 'failed', last_error = $1, updated_at = CURRENT_TIMESTAMP
		WHERE id = $2 AND status <> 'provisioned'`), reason, signupID)
	return err
}
