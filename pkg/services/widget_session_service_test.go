package services

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/codexylab/alvex-backend/pkg/repository"
)

type widgetSessionRepositoryStub struct {
	createdSessionID string
	createdHash      string
	createdExpiry    time.Time
	resolvedHash     string
	client           *repository.WidgetBootstrapClient
	session          *repository.WidgetSession
	resolveErr       error
	revokedSessionID string
}

func (s *widgetSessionRepositoryStub) Revoke(_ context.Context, sessionID, _ string, _ time.Time) error {
	s.revokedSessionID = sessionID
	return nil
}

func (s *widgetSessionRepositoryStub) Create(
	_ context.Context,
	sessionID string,
	_ string,
	_ string,
	tokenHash string,
	expiresAt time.Time,
) (*repository.WidgetBootstrapClient, error) {
	s.createdSessionID = sessionID
	s.createdHash = tokenHash
	s.createdExpiry = expiresAt
	return s.client, nil
}

func (s *widgetSessionRepositoryStub) Resolve(
	_ context.Context,
	_ string,
	_ string,
	tokenHash string,
	_ time.Time,
) (*repository.WidgetSession, error) {
	s.resolvedHash = tokenHash
	return s.session, s.resolveErr
}

func TestWidgetSessionBootstrapStoresOnlyTokenHash(t *testing.T) {
	repo := &widgetSessionRepositoryStub{client: &repository.WidgetBootstrapClient{Name: "Example"}}
	service := NewWidgetSessionService(repo)
	fixedNow := time.Date(2026, time.September, 4, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return fixedNow }

	bootstrap, err := service.Bootstrap(context.Background(), "client_one", "https://example.com")
	if err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}
	if !isValidWidgetSessionToken(bootstrap.SessionToken) {
		t.Fatal("expected a cryptographically-shaped widget token")
	}
	if repo.createdHash == bootstrap.SessionToken || repo.createdHash != hashWidgetSessionToken(bootstrap.SessionToken) {
		t.Fatal("repository must receive only the token hash")
	}
	if repo.createdSessionID == "" || bootstrap.SessionID != repo.createdSessionID {
		t.Fatal("expected a server-generated session ID")
	}
	if !repo.createdExpiry.Equal(fixedNow.Add(defaultWidgetSessionTTL)) {
		t.Fatalf("unexpected expiry %s", repo.createdExpiry)
	}
}

func TestWidgetSessionResolveFailsClosed(t *testing.T) {
	repo := &widgetSessionRepositoryStub{resolveErr: sql.ErrNoRows}
	service := NewWidgetSessionService(repo)

	if _, err := service.Resolve(context.Background(), "client_one", "https://example.com", "bad-token"); err != ErrInvalidWidgetSession {
		t.Fatalf("expected invalid session error for malformed token, got %v", err)
	}

	token, err := newWidgetSessionToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	if _, err := service.Resolve(context.Background(), "client_one", "https://example.com", token); err != ErrInvalidWidgetSession {
		t.Fatalf("expected invalid session error for unknown token, got %v", err)
	}
	if repo.resolvedHash != hashWidgetSessionToken(token) {
		t.Fatal("expected repository lookup by token hash")
	}
}

func TestWidgetSessionRevokeUsesTrustedSessionIdentity(t *testing.T) {
	repo := &widgetSessionRepositoryStub{}
	service := NewWidgetSessionService(repo)
	if err := service.Revoke(context.Background(), "session_one", "client_one"); err != nil {
		t.Fatalf("revoke failed: %v", err)
	}
	if repo.revokedSessionID != "session_one" {
		t.Fatalf("expected trusted session to be revoked, got %q", repo.revokedSessionID)
	}
	if err := service.Revoke(context.Background(), "", "client_one"); err != ErrInvalidWidgetSession {
		t.Fatalf("expected invalid session error, got %v", err)
	}
}
