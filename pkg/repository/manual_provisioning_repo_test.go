package repository

import (
	"context"
	"testing"
	"time"

	"github.com/codexylab/alvex-backend/pkg/database"
)

func TestManualProvisioningRepositoryCommitsCompleteTenant(t *testing.T) {
	db := newTenantTestDB(t)
	seedPlatformAdmin(t, db)
	repo := NewSQLManualProvisioningRepository(db)
	input := manualProvisioningFixture("client_new", "user_new", "new@example.com")

	if err := repo.ProvisionInvitedClient(context.Background(), input); err != nil {
		t.Fatalf("provision invited client: %v", err)
	}
	if err := NewSQLUserRepository(db).Upsert(context.Background(), "user_new", "New Owner", "new@example.com"); err != nil {
		t.Fatalf("accept invited identity: %v", err)
	}

	checks := []struct {
		query string
		want  int
	}{
		{`SELECT COUNT(*) FROM organizations WHERE id = 'org_new'`, 1},
		{`SELECT COUNT(*) FROM clients WHERE id = 'client_new'`, 1},
		{`SELECT COUNT(*) FROM organization_memberships WHERE user_id = 'user_new'`, 1},
		{`SELECT COUNT(*) FROM client_memberships WHERE user_id = 'user_new'`, 1},
		{`SELECT COUNT(*) FROM identity_invitations WHERE supabase_user_id = 'user_new'`, 1},
		{`SELECT COUNT(*) FROM identity_invitations WHERE supabase_user_id = 'user_new' AND status = 'accepted'`, 1},
		{`SELECT COUNT(*) FROM client_memberships WHERE user_id = 'user_new' AND status = 'active'`, 1},
		{`SELECT COUNT(*) FROM organization_memberships WHERE user_id = 'user_new' AND status = 'active'`, 1},
	}
	for _, check := range checks {
		var count int
		if err := db.QueryRow(check.query).Scan(&count); err != nil {
			t.Fatalf("query provisioned record: %v", err)
		}
		if count != check.want {
			t.Fatalf("query %q returned %d, want %d", check.query, count, check.want)
		}
	}
}

func TestManualProvisioningRepositoryRollsBackPartialTenant(t *testing.T) {
	db := newTenantTestDB(t)
	seedPlatformAdmin(t, db)
	repo := NewSQLManualProvisioningRepository(db)
	input := manualProvisioningFixture("client_a", "user_rollback", "rollback@example.com")

	if err := repo.ProvisionInvitedClient(context.Background(), input); err == nil {
		t.Fatal("expected duplicate client ID to fail")
	}

	var users int
	if err := db.QueryRow(`SELECT COUNT(*) FROM app_users WHERE id = 'user_rollback'`).Scan(&users); err != nil {
		t.Fatalf("query rolled-back user: %v", err)
	}
	if users != 0 {
		t.Fatalf("expected transaction rollback, found %d user records", users)
	}
}

func manualProvisioningFixture(clientID, userID, email string) ManualClientProvisioning {
	return ManualClientProvisioning{
		InvitationID:     "invitation_new",
		InvitedBy:        "platform_admin",
		UserID:           userID,
		Email:            email,
		UserName:         "New Owner",
		OrganizationID:   "org_new",
		OrganizationName: "New Organization",
		OrganizationSlug: "new-organization",
		ClientID:         clientID,
		ClientName:       "New Assistant",
		Domain:           "https://example.com",
		Provider:         "Gemini",
		Model:            "Gemini 2.0 Flash",
		BillingPlan:      "Basic",
		SystemPersona:    "Helpful assistant",
		WebhookURL:       "https://api.example.com/webhook/wa/v2/client_new",
		InviteExpiresAt:  time.Now().Add(time.Hour),
	}
}

func seedPlatformAdmin(t *testing.T, db *database.DB) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO app_users (id, supabase_user_id, email, name, platform_role)
		VALUES ('platform_admin', 'supabase_admin', 'admin@example.com', 'Admin', 'super_admin')
	`); err != nil {
		t.Fatalf("seed platform admin: %v", err)
	}
}
