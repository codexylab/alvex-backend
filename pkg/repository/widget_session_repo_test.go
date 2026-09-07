package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/codexylab/alvex-backend/pkg/database"
)

func TestWidgetOriginPolicy(t *testing.T) {
	tests := []struct {
		name    string
		origin  string
		domain  string
		allowed []string
		want    bool
	}{
		{name: "exact allowlist entry", origin: "https://shop.example.com", allowed: []string{"https://shop.example.com"}, want: true},
		{name: "allowlist includes port", origin: "https://shop.example.com:8443", allowed: []string{"https://shop.example.com:8443"}, want: true},
		{name: "different port rejected", origin: "https://shop.example.com:8443", allowed: []string{"https://shop.example.com"}, want: false},
		{name: "non-empty allowlist overrides domain", origin: "https://example.com", domain: "example.com", allowed: []string{"https://shop.example.com"}, want: false},
		{name: "bare domain permits production https", origin: "https://example.com", domain: "example.com", want: true},
		{name: "bare domain rejects insecure http", origin: "http://example.com", domain: "example.com", want: false},
		{name: "explicit local development origin", origin: "http://localhost:3000", domain: "http://localhost:3000", want: true},
		{name: "subdomain is not implicit", origin: "https://app.example.com", domain: "example.com", want: false},
		{name: "missing origin", domain: "example.com", want: false},
		{name: "unsupported scheme", origin: "javascript:alert(1)", allowed: []string{"javascript:alert(1)"}, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isAllowedWidgetOrigin(test.origin, test.domain, test.allowed); got != test.want {
				t.Fatalf("expected %v, got %v", test.want, got)
			}
		})
	}
}

func TestWidgetSessionRepositoryBindsTokenToClientAndOrigin(t *testing.T) {
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	_, err = sqlDB.Exec(`
		CREATE TABLE clients (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			domain TEXT,
			allowed_origins TEXT NOT NULL DEFAULT '[]',
			status TEXT NOT NULL,
			widget_brand_name TEXT,
			widget_logo_url TEXT,
			widget_primary_color TEXT,
			widget_secondary_color TEXT,
			widget_remove_branding INTEGER,
			widget_branding_allowed INTEGER,
			widget_ticketing_allowed INTEGER,
			widget_ticketing_enabled INTEGER,
			widget_admin_msg_allowed INTEGER,
			widget_admin_msg_enabled INTEGER,
			widget_image_search_allowed INTEGER,
			widget_image_search_enabled INTEGER
		);
		CREATE TABLE widget_sessions (
			id TEXT PRIMARY KEY,
			client_id TEXT NOT NULL,
			origin TEXT NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			expires_at DATETIME NOT NULL,
			revoked_at DATETIME
		);
		INSERT INTO clients (
			id, name, domain, allowed_origins, status,
			widget_branding_allowed, widget_ticketing_allowed, widget_ticketing_enabled,
			widget_admin_msg_allowed, widget_admin_msg_enabled,
			widget_image_search_allowed, widget_image_search_enabled
		) VALUES (
			'client_one', 'Example', 'example.com', '["https://example.com"]', 'Active',
			1, 1, 1, 1, 1, 1, 1
		);`)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}

	repo := NewSQLWidgetSessionRepository(database.NewDB(sqlDB, "sqlite"))
	expiresAt := time.Now().UTC().Add(time.Hour)
	if _, err := repo.Create(
		context.Background(),
		"29fbd908-63ad-440a-b891-c5176d77ad76",
		"client_one",
		"https://example.com",
		"hash_one",
		expiresAt,
	); err != nil {
		t.Fatalf("create session: %v", err)
	}

	session, err := repo.Resolve(context.Background(), "client_one", "https://example.com", "hash_one", time.Now().UTC())
	if err != nil {
		t.Fatalf("resolve session: %v", err)
	}
	if session.ClientID != "client_one" || !session.TicketingEnabled || !session.AdminMsgEnabled || !session.ImageSearchEnabled {
		t.Fatalf("unexpected session: %#v", session)
	}

	if _, err := repo.Resolve(context.Background(), "client_one", "https://evil.example", "hash_one", time.Now().UTC()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected wrong origin to fail closed, got %v", err)
	}
	if _, err := repo.Resolve(context.Background(), "other_client", "https://example.com", "hash_one", time.Now().UTC()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected wrong client to fail closed, got %v", err)
	}
	if err := repo.Revoke(context.Background(), session.ID, session.ClientID, time.Now().UTC()); err != nil {
		t.Fatalf("revoke session: %v", err)
	}
	if _, err := repo.Resolve(context.Background(), "client_one", "https://example.com", "hash_one", time.Now().UTC()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected revoked session to fail closed, got %v", err)
	}

	if _, err := sqlDB.Exec(`
		INSERT INTO widget_sessions (id, client_id, origin, token_hash, expires_at)
		VALUES ('expired', 'client_one', 'https://example.com', 'hash_expired', ?)`, time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatalf("insert expired session: %v", err)
	}
	deleted, err := repo.DeleteExpired(context.Background(), time.Now().UTC())
	if err != nil || deleted != 2 {
		t.Fatalf("expected expired and revoked sessions deleted, deleted=%d err=%v", deleted, err)
	}
}
