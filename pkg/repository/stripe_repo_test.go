package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestStripeEventRepositoryDeduplicatesProcessedEvents(t *testing.T) {
	db := newTenantTestDB(t)
	repo := NewSQLStripeRepository(db)
	repo.Now = func() time.Time { return time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC) }

	claimed, err := repo.ClaimEvent(context.Background(), "evt_one", "checkout.session.completed", "hash_one")
	if err != nil || !claimed {
		t.Fatalf("expected first delivery to be claimed, claimed=%v err=%v", claimed, err)
	}
	if err := repo.CompleteEvent(context.Background(), "evt_one"); err != nil {
		t.Fatalf("complete event: %v", err)
	}
	claimed, err = repo.ClaimEvent(context.Background(), "evt_one", "checkout.session.completed", "hash_one")
	if err != nil || claimed {
		t.Fatalf("expected processed duplicate to be ignored, claimed=%v err=%v", claimed, err)
	}
	_, err = repo.ClaimEvent(context.Background(), "evt_one", "checkout.session.completed", "different_hash")
	if !errors.Is(err, ErrWebhookPayloadMismatch) {
		t.Fatalf("expected altered duplicate payload rejection, got %v", err)
	}
}

func TestStripeRepositoryProvisioningIsTransactionalAndIdempotent(t *testing.T) {
	db := newTenantTestDB(t)
	if _, err := db.Exec(`
		INSERT INTO app_users (id, supabase_user_id, email, name)
		VALUES ('buyer_one', 'supabase_buyer', 'buyer@example.com', 'Buyer')
	`); err != nil {
		t.Fatalf("seed buyer: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO checkout_signups
		  (id, app_user_id, email, organization_name, client_name, domain, provider, model,
		   billing_plan, stripe_session_id, status)
		VALUES
		  ('signup_one', 'buyer_one', 'buyer@example.com', 'Buyer Org', 'Buyer Assistant',
		   'https://buyer.example.com', 'Gemini', 'Gemini 2.0 Flash', 'Basic', 'cs_one', 'checkout_created')
	`); err != nil {
		t.Fatalf("seed checkout signup: %v", err)
	}
	repo := NewSQLStripeRepository(db)
	input := CheckoutProvisioningInput{
		SignupID:         "signup_one",
		SessionID:        "cs_one",
		AppUserID:        "buyer_one",
		CustomerID:       "cus_one",
		SubscriptionID:   "sub_one",
		OrganizationID:   "org_signup_one",
		OrganizationSlug: "buyer-org-signup",
		ClientID:         "buyer-assistant-signup",
		SystemPersona:    "Helpful assistant",
		WebhookURL:       "https://api.example.com/webhook/wa/v2/buyer-assistant-signup",
	}

	if err := repo.ProvisionPaidSignup(context.Background(), input); err != nil {
		t.Fatalf("provision paid signup: %v", err)
	}
	if err := repo.ProvisionPaidSignup(context.Background(), input); err != nil {
		t.Fatalf("idempotent provisioning replay: %v", err)
	}

	for _, query := range []string{
		`SELECT COUNT(*) FROM organizations WHERE id = 'org_signup_one'`,
		`SELECT COUNT(*) FROM clients WHERE id = 'buyer-assistant-signup'`,
		`SELECT COUNT(*) FROM subscriptions WHERE provider_subscription_id = 'sub_one'`,
	} {
		var count int
		if err := db.QueryRow(query).Scan(&count); err != nil || count != 1 {
			t.Fatalf("expected one provisioned record for %q, count=%d err=%v", query, count, err)
		}
	}
}

func TestStripeRepositoryInvoiceTransitionsAreScopedAndMonotonic(t *testing.T) {
	db := newTenantTestDB(t)
	if _, err := db.Exec(`
		INSERT INTO subscriptions
		  (id, organization_id, client_id, provider_customer_id, provider_subscription_id, plan, status)
		VALUES
		  ('subscription_a', 'org_a', 'client_a', 'cus_a', 'sub_a', 'Basic', 'active'),
		  ('subscription_b', 'org_b', 'client_b', 'cus_b', 'sub_b', 'Basic', 'active')
	`); err != nil {
		t.Fatalf("seed subscriptions: %v", err)
	}

	repo := NewSQLStripeRepository(db)
	repo.Now = func() time.Time { return time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC) }
	paidAt := repo.Now()
	paid := StripeInvoiceInput{
		ProviderInvoiceID:    "in_paid",
		ProviderSubscription: "sub_a",
		ProviderCustomer:     "cus_a",
		ProviderStatus:       "paid",
		InternalStatus:       "Paid",
		Currency:             "USD",
		Amount:               12.99,
		PaidAt:               &paidAt,
	}
	if err := repo.UpsertInvoice(context.Background(), paid); err != nil {
		t.Fatalf("insert paid invoice: %v", err)
	}

	lateFailure := paid
	lateFailure.ProviderStatus = "payment_failed"
	lateFailure.InternalStatus = "Pending"
	lateFailure.PaidAt = nil
	if err := repo.UpsertInvoice(context.Background(), lateFailure); err != nil {
		t.Fatalf("apply delayed failure: %v", err)
	}

	var status, providerStatus, clientID string
	var paidAtCount int
	if err := db.QueryRow(`
		SELECT status, provider_status, client_id,
		       CASE WHEN paid_at IS NULL THEN 0 ELSE 1 END
		FROM invoices WHERE provider_invoice_id = 'in_paid'
	`).Scan(&status, &providerStatus, &clientID, &paidAtCount); err != nil {
		t.Fatalf("load invoice: %v", err)
	}
	if status != "Paid" || providerStatus != "payment_failed" || clientID != "client_a" || paidAtCount != 1 {
		t.Fatalf(
			"unexpected delayed transition: status=%q provider=%q client=%q paid_at=%d",
			status,
			providerStatus,
			clientID,
			paidAtCount,
		)
	}

	before := invoiceCount(t, db)
	invalid := paid
	invalid.ProviderInvoiceID = "in_invalid"
	invalid.ProviderCustomer = "cus_wrong"
	if err := repo.UpsertInvoice(context.Background(), invalid); err == nil {
		t.Fatal("expected customer mismatch to be rejected")
	}
	invalid.ProviderCustomer = "cus_a"
	invalid.ProviderSubscription = "sub_missing"
	if err := repo.UpsertInvoice(context.Background(), invalid); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected missing subscription to be rejected, got %v", err)
	}
	if after := invoiceCount(t, db); after != before {
		t.Fatalf("invalid invoices mutated storage: before=%d after=%d", before, after)
	}

	if _, err := db.Exec(`
		INSERT INTO invoices
		  (id, client_id, client_name, amount, status, provider_invoice_id, provider_status, currency)
		VALUES ('collision', 'client_b', 'Client B', 10, 'Pending', 'in_collision', 'open', 'USD')
	`); err != nil {
		t.Fatalf("seed provider invoice collision: %v", err)
	}
	collision := paid
	collision.ProviderInvoiceID = "in_collision"
	if err := repo.UpsertInvoice(context.Background(), collision); err == nil {
		t.Fatal("expected cross-client provider invoice collision to be rejected")
	}
	if err := db.QueryRow(`
		SELECT client_id, status FROM invoices WHERE provider_invoice_id = 'in_collision'
	`).Scan(&clientID, &status); err != nil {
		t.Fatalf("load collided invoice: %v", err)
	}
	if clientID != "client_b" || status != "Pending" {
		t.Fatalf("cross-client invoice was mutated: client=%q status=%q", clientID, status)
	}
}

func invoiceCount(t *testing.T, db interface {
	QueryRow(query string, args ...interface{}) *sql.Row
}) int {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM invoices`).Scan(&count); err != nil {
		t.Fatalf("count invoices: %v", err)
	}
	return count
}
