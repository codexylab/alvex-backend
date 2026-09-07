package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/codexylab/alvex-backend/pkg/database"
)

var ErrWebhookPayloadMismatch = errors.New("webhook event payload does not match the original delivery")

type StripeEventRepository interface {
	ClaimEvent(ctx context.Context, eventID, eventType, payloadSHA256 string) (bool, error)
	CompleteEvent(ctx context.Context, eventID string) error
	FailEvent(ctx context.Context, eventID, reason string) error
}

type CheckoutProvisioningInput struct {
	SignupID         string
	SessionID        string
	AppUserID        string
	CustomerID       string
	SubscriptionID   string
	OrganizationID   string
	OrganizationSlug string
	ClientID         string
	SystemPersona    string
	WebhookURL       string
}

type StripeProvisioningRepository interface {
	ProvisionPaidSignup(ctx context.Context, input CheckoutProvisioningInput) error
	UpdateSubscription(ctx context.Context, subscriptionID, customerID, status string, cancelAtPeriodEnd bool) error
	UpsertInvoice(ctx context.Context, input StripeInvoiceInput) error
}

type StripeInvoiceInput struct {
	ProviderInvoiceID    string
	ProviderSubscription string
	ProviderCustomer     string
	ProviderStatus       string
	InternalStatus       string
	Currency             string
	Amount               float64
	DueAt                *time.Time
	PaidAt               *time.Time
}

type SQLStripeRepository struct {
	DB  *database.DB
	Now func() time.Time
}

func NewSQLStripeRepository(db *database.DB) *SQLStripeRepository {
	return &SQLStripeRepository{DB: db, Now: time.Now}
}

// ClaimEvent atomically grants one worker the right to process an event. Failed
// events and workers stuck for more than ten minutes may be retried.
func (r *SQLStripeRepository) ClaimEvent(
	ctx context.Context,
	eventID string,
	eventType string,
	payloadSHA256 string,
) (bool, error) {
	now := r.now()
	_, err := r.DB.ExecContext(ctx, r.DB.Adapt(`
		INSERT INTO webhook_events
		  (id, provider, provider_event_id, event_type, status, payload_sha256, attempts, created_at, updated_at)
		VALUES ($1, 'stripe', $2, $3, 'received', $4, 0, $5, $6)
		ON CONFLICT (provider, provider_event_id) DO NOTHING`),
		uuid.NewString(), eventID, eventType, payloadSHA256, now, now,
	)
	if err != nil {
		return false, err
	}

	result, err := r.DB.ExecContext(ctx, r.DB.Adapt(`
		UPDATE webhook_events
		SET status = 'processing', attempts = attempts + 1, last_error = NULL, updated_at = $1
		WHERE provider = 'stripe' AND provider_event_id = $2 AND payload_sha256 = $3
		  AND (status IN ('received', 'failed') OR (status = 'processing' AND updated_at < $4))`),
		now, eventID, payloadSHA256, now.Add(-10*time.Minute),
	)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected > 0 {
		return true, nil
	}

	var storedHash string
	err = r.DB.QueryRowContext(ctx, r.DB.Adapt(`
		SELECT payload_sha256 FROM webhook_events
		WHERE provider = 'stripe' AND provider_event_id = $1`), eventID).Scan(&storedHash)
	if err != nil {
		return false, err
	}
	if storedHash != payloadSHA256 {
		return false, ErrWebhookPayloadMismatch
	}
	return false, nil
}

func (r *SQLStripeRepository) CompleteEvent(ctx context.Context, eventID string) error {
	_, err := r.DB.ExecContext(ctx, r.DB.Adapt(`
		UPDATE webhook_events
		SET status = 'processed', processed_at = $1, updated_at = $2
		WHERE provider = 'stripe' AND provider_event_id = $3`), r.now(), r.now(), eventID)
	return err
}

func (r *SQLStripeRepository) FailEvent(ctx context.Context, eventID, reason string) error {
	_, err := r.DB.ExecContext(ctx, r.DB.Adapt(`
		UPDATE webhook_events
		SET status = 'failed', last_error = $1, updated_at = $2
		WHERE provider = 'stripe' AND provider_event_id = $3`), reason, r.now(), eventID)
	return err
}

// ProvisionPaidSignup creates the tenant, client, memberships and subscription
// in one transaction. A provisioned signup is a successful idempotent replay.
func (r *SQLStripeRepository) ProvisionPaidSignup(
	ctx context.Context,
	input CheckoutProvisioningInput,
) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var signup CheckoutSignup
	var storedSession sql.NullString
	var status string
	err = tx.QueryRowContext(ctx, r.DB.Adapt(`
		SELECT id, app_user_id, email, organization_name, client_name, domain, provider, model,
		       billing_plan, stripe_session_id, status
		FROM checkout_signups WHERE id = $1`), input.SignupID).Scan(
		&signup.ID, &signup.AppUserID, &signup.Email, &signup.OrganizationName,
		&signup.ClientName, &signup.Domain, &signup.Provider, &signup.Model,
		&signup.BillingPlan, &storedSession, &status,
	)
	if err != nil {
		return err
	}
	if status == "provisioned" {
		return nil
	}
	if status != "checkout_created" || !storedSession.Valid || storedSession.String != input.SessionID {
		return fmt.Errorf("checkout signup is not ready for provisioning")
	}
	if signup.AppUserID != input.AppUserID {
		return fmt.Errorf("checkout user does not match signup owner")
	}

	statements := []struct {
		query string
		args  []interface{}
	}{
		{`INSERT INTO organizations (id, name, slug, type, status)
		  VALUES ($1, $2, $3, 'direct_customer', 'active')`,
			[]interface{}{input.OrganizationID, signup.OrganizationName, input.OrganizationSlug}},
		{`INSERT INTO organization_memberships (organization_id, user_id, role, status)
		  VALUES ($1, $2, 'owner', 'active')`,
			[]interface{}{input.OrganizationID, signup.AppUserID}},
		{`INSERT INTO clients
		  (id, organization_id, name, domain, status, provider, model, system_persona,
		   webhook_url, billing_plan, owner_id, stripe_customer_id, stripe_subscription_id, onboarding_status)
		  VALUES ($1, $2, $3, $4, 'Active', $5, $6, $7, $8, $9, $10, $11, $12, 'pending')`,
			[]interface{}{
				input.ClientID, input.OrganizationID, signup.ClientName, signup.Domain,
				signup.Provider, signup.Model, input.SystemPersona,
				input.WebhookURL, signup.BillingPlan, signup.AppUserID,
				input.CustomerID, input.SubscriptionID,
			}},
		{`INSERT INTO client_memberships (client_id, user_id, role, status)
		  VALUES ($1, $2, 'client_admin', 'active')`,
			[]interface{}{input.ClientID, signup.AppUserID}},
		{`INSERT INTO subscriptions
		  (id, organization_id, client_id, provider_customer_id, provider_subscription_id, plan, status)
		  VALUES ($1, $2, $3, $4, $5, $6, 'active')`,
			[]interface{}{
				uuid.NewString(), input.OrganizationID, input.ClientID,
				input.CustomerID, input.SubscriptionID, signup.BillingPlan,
			}},
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, r.DB.Adapt(statement.query), statement.args...); err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, r.DB.Adapt(`
		UPDATE checkout_signups
		SET status = 'provisioned', organization_id = $1, client_id = $2,
		    last_error = NULL, updated_at = $3
		WHERE id = $4 AND status = 'checkout_created'`),
		input.OrganizationID, input.ClientID, r.now(), input.SignupID,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return fmt.Errorf("checkout signup provisioning state changed concurrently")
	}
	return tx.Commit()
}

func (r *SQLStripeRepository) UpdateSubscription(
	ctx context.Context,
	subscriptionID string,
	customerID string,
	status string,
	cancelAtPeriodEnd bool,
) error {
	result, err := r.DB.ExecContext(ctx, r.DB.Adapt(`
		UPDATE subscriptions
		SET provider_customer_id = $1, status = $2, cancel_at_period_end = $3, updated_at = $4
		WHERE provider = 'stripe' AND provider_subscription_id = $5`),
		customerID, status, cancelAtPeriodEnd, r.now(), subscriptionID,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// UpsertInvoice resolves tenancy through the stored Stripe subscription and
// applies a monotonic internal transition: a paid invoice is never downgraded
// by a delayed failure/open event.
func (r *SQLStripeRepository) UpsertInvoice(ctx context.Context, input StripeInvoiceInput) error {
	var clientID, clientName, storedCustomerID string
	err := r.DB.QueryRowContext(ctx, r.DB.Adapt(`
		SELECT s.client_id, c.name, s.provider_customer_id
		FROM subscriptions s
		JOIN clients c ON c.id = s.client_id
		WHERE s.provider = 'stripe' AND s.provider_subscription_id = $1`), input.ProviderSubscription).Scan(
		&clientID,
		&clientName,
		&storedCustomerID,
	)
	if err != nil {
		return err
	}
	if storedCustomerID != input.ProviderCustomer {
		return fmt.Errorf("Stripe invoice customer does not match subscription")
	}

	result, err := r.DB.ExecContext(ctx, r.DB.Adapt(`
		INSERT INTO invoices
		  (id, client_id, client_name, amount, status, due_date, paid_at,
		   provider_invoice_id, provider_status, currency, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (provider_invoice_id) WHERE provider_invoice_id IS NOT NULL DO UPDATE SET
		  amount = excluded.amount,
		  status = CASE WHEN invoices.status = 'Paid' THEN 'Paid' ELSE excluded.status END,
		  due_date = COALESCE(excluded.due_date, invoices.due_date),
		  paid_at = COALESCE(invoices.paid_at, excluded.paid_at),
		  provider_status = excluded.provider_status,
		  currency = excluded.currency
		WHERE invoices.client_id = excluded.client_id`),
		"stripe_"+input.ProviderInvoiceID,
		clientID,
		clientName,
		input.Amount,
		input.InternalStatus,
		input.DueAt,
		input.PaidAt,
		input.ProviderInvoiceID,
		input.ProviderStatus,
		input.Currency,
		r.now(),
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("Stripe invoice belongs to another client")
	}
	return nil
}

func (r *SQLStripeRepository) now() time.Time {
	if r.Now != nil {
		return r.Now().UTC()
	}
	return time.Now().UTC()
}
