package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/codexylab/alvex-backend/pkg/tenant"
)

func TestQueueDocumentReindexIsTenantScopedAndPreventsConcurrentJobs(t *testing.T) {
	db := newTenantTestDB(t)
	if _, err := db.Exec(`
		UPDATE documents SET status = 'processed' WHERE id IN ('document_a', 'document_b');
		INSERT INTO document_contents (document_id, client_id, content, content_sha256)
		VALUES
		  ('document_a', 'client_a', 'A content', 'hash_a'),
		  ('document_b', 'client_b', 'B content', 'hash_b');
	`); err != nil {
		t.Fatalf("seed document contents: %v", err)
	}
	repo := NewTenantSQLDocumentRepository(db)
	ctx := tenant.WithScope(context.Background(), tenant.Scope{
		OrganizationID: "org_a",
		UserID:         "user_a",
		Role:           "owner",
	})

	if err := repo.QueueDocumentReindex(ctx, "document_a", "client_a"); err != nil {
		t.Fatalf("queue owned document: %v", err)
	}
	if err := repo.QueueDocumentReindex(ctx, "document_a", "client_a"); !errors.Is(err, ErrDocumentAlreadyQueued) {
		t.Fatalf("expected concurrent queue conflict, got %v", err)
	}
	if err := repo.QueueDocumentReindex(ctx, "document_b", "client_b"); err == nil {
		t.Fatal("expected cross-tenant re-index to fail")
	}
}
