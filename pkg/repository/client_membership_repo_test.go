package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/codexylab/alvex-backend/pkg/database"
)

func TestClientMembershipRepositoryScopesRequestedClient(t *testing.T) {
	db := newTenantTestDB(t)
	seedClientMemberships(t, db)
	repo := NewSQLClientMembershipRepository(db)

	access, err := repo.ResolveActiveAccess(context.Background(), "user_a", "client_a")
	if err != nil {
		t.Fatalf("resolve owned client: %v", err)
	}
	if access.ClientID != "client_a" || access.OrganizationID != "org_a" {
		t.Fatalf("unexpected access scope: %#v", access)
	}

	_, err = repo.ResolveActiveAccess(context.Background(), "user_a", "client_b")
	if !errors.Is(err, ErrClientMembershipNotFound) {
		t.Fatalf("expected cross-client access to fail closed, got %v", err)
	}
}

func TestClientMembershipRepositoryRequiresSelectionForMultipleClients(t *testing.T) {
	db := newTenantTestDB(t)
	seedClientMemberships(t, db)
	if _, err := db.Exec(`INSERT INTO client_memberships (client_id, user_id, role) VALUES ('client_b', 'user_a', 'client_viewer')`); err != nil {
		t.Fatalf("seed second membership: %v", err)
	}

	_, err := NewSQLClientMembershipRepository(db).ResolveActiveAccess(context.Background(), "user_a", "")
	if !errors.Is(err, ErrClientSelectionRequired) {
		t.Fatalf("expected explicit client selection, got %v", err)
	}
}

func seedClientMemberships(t *testing.T, db *database.DB) {
	t.Helper()
	statements := []string{
		`INSERT INTO app_users (id, supabase_user_id, email, name) VALUES ('user_a', 'supabase_a', 'a@example.com', 'User A')`,
		`INSERT INTO app_users (id, supabase_user_id, email, name) VALUES ('user_b', 'supabase_b', 'b@example.com', 'User B')`,
		`INSERT INTO client_memberships (client_id, user_id, role) VALUES ('client_a', 'user_a', 'client_admin')`,
		`INSERT INTO client_memberships (client_id, user_id, role) VALUES ('client_b', 'user_b', 'client_admin')`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("seed client membership database: %v", err)
		}
	}
}
