package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/codexylab/alvex-backend/pkg/database"
	"github.com/codexylab/alvex-backend/pkg/tenant"
)

func TestTenantRepositoriesRejectCrossOrganizationRecords(t *testing.T) {
	db := newTenantTestDB(t)
	ctxA := tenant.WithScope(context.Background(), tenant.Scope{OrganizationID: "org_a", UserID: "user_a", Role: "owner"})

	clientRepo := NewTenantSQLClientRepository(db)
	if _, err := clientRepo.GetByID(ctxA, "client_b"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected cross-tenant client lookup to return no rows, got %v", err)
	}
	clients, total, err := clientRepo.List(ctxA, "", "", 1, 10)
	if err != nil {
		t.Fatalf("list clients: %v", err)
	}
	if total != 1 || len(clients) != 1 || clients[0].ID != "client_a" {
		t.Fatalf("unexpected tenant client list: total=%d clients=%#v", total, clients)
	}
	if affected, err := clientRepo.UpdateFields(ctxA, "client_b", map[string]interface{}{"name": "tampered"}); err != nil || affected != 0 {
		t.Fatalf("cross-tenant client update affected=%d err=%v", affected, err)
	}

	billingRepo := NewTenantSQLBillingRepository(db)
	invoices, err := billingRepo.ListInvoices(ctxA)
	if err != nil {
		t.Fatalf("list invoices: %v", err)
	}
	if len(invoices) != 1 || invoices[0].ID != "invoice_a" {
		t.Fatalf("unexpected tenant invoice list: %#v", invoices)
	}
	if affected, err := billingRepo.MarkPaid(ctxA, "invoice_b", time.Now()); err != nil || affected != 0 {
		t.Fatalf("cross-tenant invoice update affected=%d err=%v", affected, err)
	}

	analyticsRepo := NewSQLAnalyticsRepository(db)
	totalClients, activeClients, err := analyticsRepo.GetOverviewCounts(ctxA)
	if err != nil || totalClients != 1 || activeClients != 1 {
		t.Fatalf("unexpected overview total=%d active=%d err=%v", totalClients, activeClients, err)
	}
	totalLogs, _, err := analyticsRepo.GetActivityLogsSummary(ctxA)
	if err != nil || totalLogs != 1 {
		t.Fatalf("unexpected activity summary total=%d err=%v", totalLogs, err)
	}

	handoffRepo := NewSQLHandoffRepository(db)
	if affected, err := handoffRepo.Resolve(ctxA, "activity_b"); err != nil || affected != 0 {
		t.Fatalf("cross-tenant handoff update affected=%d err=%v", affected, err)
	}

	documentRepo := NewTenantSQLDocumentRepository(db)
	documents, err := documentRepo.GetDocumentsByClient(ctxA, "client_b")
	if err != nil || len(documents) != 0 {
		t.Fatalf("expected no cross-tenant documents, got %#v err=%v", documents, err)
	}
}

func TestTenantRepositoriesFailClosedWithoutScope(t *testing.T) {
	db := newTenantTestDB(t)
	ctx := context.Background()

	if _, _, err := NewTenantSQLClientRepository(db).List(ctx, "", "", 1, 10); !errors.Is(err, tenant.ErrMissingOrganizationScope) {
		t.Fatalf("expected missing-scope error from clients, got %v", err)
	}
	if _, err := NewTenantSQLBillingRepository(db).ListInvoices(ctx); !errors.Is(err, tenant.ErrMissingOrganizationScope) {
		t.Fatalf("expected missing-scope error from billing, got %v", err)
	}
	if _, _, err := NewSQLAnalyticsRepository(db).GetOverviewCounts(ctx); !errors.Is(err, tenant.ErrMissingOrganizationScope) {
		t.Fatalf("expected missing-scope error from analytics, got %v", err)
	}
	if _, err := NewSQLHandoffRepository(db).ListNeedsAttention(ctx, 10); !errors.Is(err, tenant.ErrMissingOrganizationScope) {
		t.Fatalf("expected missing-scope error from handoff, got %v", err)
	}
	if _, err := NewTenantSQLDocumentRepository(db).GetDocumentsByClient(ctx, "client_a"); !errors.Is(err, tenant.ErrMissingOrganizationScope) {
		t.Fatalf("expected missing-scope error from documents, got %v", err)
	}
}

func newTenantTestDB(t *testing.T) *database.DB {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := database.NewDB(sqlDB, "sqlite")
	if err := db.RunMigrations(); err != nil {
		t.Fatalf("run schema: %v", err)
	}
	if err := db.RunColumnMigrations(); err != nil {
		t.Fatalf("run column migrations: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	statements := []string{
		`INSERT INTO organizations (id, name, slug, type) VALUES ('org_a', 'Org A', 'org-a', 'direct_customer')`,
		`INSERT INTO organizations (id, name, slug, type) VALUES ('org_b', 'Org B', 'org-b', 'direct_customer')`,
		`INSERT INTO clients (id, organization_id, name, domain, api_key, system_persona, webhook_url)
		 VALUES ('client_a', 'org_a', 'Client A', 'a.example', '', '', '')`,
		`INSERT INTO clients (id, organization_id, name, domain, api_key, system_persona, webhook_url)
		 VALUES ('client_b', 'org_b', 'Client B', 'b.example', '', '', '')`,
		`INSERT INTO invoices (id, client_id, client_name, amount) VALUES ('invoice_a', 'client_a', 'Client A', 10)`,
		`INSERT INTO invoices (id, client_id, client_name, amount) VALUES ('invoice_b', 'client_b', 'Client B', 20)`,
		`INSERT INTO activity_logs (id, client_id, client_name, message, needs_human)
		 VALUES ('activity_a', 'client_a', 'Client A', 'A', 1)`,
		`INSERT INTO activity_logs (id, client_id, client_name, message, needs_human)
		 VALUES ('activity_b', 'client_b', 'Client B', 'B', 1)`,
		`INSERT INTO documents (id, client_id, filename, file_type, status)
		 VALUES ('document_a', 'client_a', 'a.txt', '.txt', 'processed')`,
		`INSERT INTO documents (id, client_id, filename, file_type, status)
		 VALUES ('document_b', 'client_b', 'b.txt', '.txt', 'processed')`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("seed tenant test database: %v", err)
		}
	}
	return db
}
