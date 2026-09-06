package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/codexylab/alvex-backend/pkg/repository"
	"github.com/google/uuid"
)

const (
	widgetSessionTokenPrefix = "wst_"
	widgetSessionTokenBytes  = 32
	defaultWidgetSessionTTL  = 24 * time.Hour
)

var ErrInvalidWidgetSession = errors.New("invalid widget session")

// WidgetBootstrap is the only response that contains the raw session token.
// Persistence receives only its SHA-256 digest.
type WidgetBootstrap struct {
	SessionID    string             `json:"session_id"`
	SessionToken string             `json:"session_token"`
	ExpiresAt    time.Time          `json:"expires_at"`
	Client       WidgetClientConfig `json:"client"`
}

// WidgetClientConfig exposes presentation and licensed feature flags only.
// Provider credentials and internal tenant identifiers are deliberately absent.
type WidgetClientConfig struct {
	Name                     string `json:"name"`
	WidgetBrandName          string `json:"widget_brand_name"`
	WidgetLogoURL            string `json:"widget_logo_url"`
	WidgetPrimaryColor       string `json:"widget_primary_color"`
	WidgetSecondaryColor     string `json:"widget_secondary_color"`
	WidgetRemoveBranding     bool   `json:"widget_remove_branding"`
	WidgetBrandingAllowed    bool   `json:"widget_branding_allowed"`
	WidgetTicketingAllowed   bool   `json:"widget_ticketing_allowed"`
	WidgetTicketingEnabled   bool   `json:"widget_ticketing_enabled"`
	WidgetAdminMsgAllowed    bool   `json:"widget_admin_msg_allowed"`
	WidgetAdminMsgEnabled    bool   `json:"widget_admin_msg_enabled"`
	WidgetImageSearchAllowed bool   `json:"widget_image_search_allowed"`
	WidgetImageSearchEnabled bool   `json:"widget_image_search_enabled"`
}

// WidgetSessionService creates and validates short-lived, origin-bound widget
// sessions. The raw bearer token is never written to the database or logs.
type WidgetSessionService struct {
	repository repository.WidgetSessionRepository
	ttl        time.Duration
	now        func() time.Time
}

func NewWidgetSessionService(repository repository.WidgetSessionRepository) *WidgetSessionService {
	return &WidgetSessionService{
		repository: repository,
		ttl:        defaultWidgetSessionTTL,
		now:        time.Now,
	}
}

func (s *WidgetSessionService) Bootstrap(ctx context.Context, clientID, origin string) (*WidgetBootstrap, error) {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" || strings.TrimSpace(origin) == "" {
		return nil, repository.ErrWidgetOriginForbidden
	}

	token, err := newWidgetSessionToken()
	if err != nil {
		return nil, fmt.Errorf("generate widget session token: %w", err)
	}
	expiresAt := s.now().UTC().Add(s.ttl)
	sessionID := uuid.NewString()
	client, err := s.repository.Create(ctx, sessionID, clientID, origin, hashWidgetSessionToken(token), expiresAt)
	if err != nil {
		return nil, err
	}

	return &WidgetBootstrap{
		SessionID:    sessionID,
		SessionToken: token,
		ExpiresAt:    expiresAt,
		Client:       mapWidgetClientConfig(client),
	}, nil
}

func (s *WidgetSessionService) Resolve(
	ctx context.Context,
	clientID string,
	origin string,
	token string,
) (*repository.WidgetSession, error) {
	if !isValidWidgetSessionToken(token) || strings.TrimSpace(origin) == "" {
		return nil, ErrInvalidWidgetSession
	}

	session, err := s.repository.Resolve(
		ctx,
		strings.TrimSpace(clientID),
		origin,
		hashWidgetSessionToken(token),
		s.now().UTC(),
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidWidgetSession
	}
	if err != nil {
		return nil, err
	}
	return session, nil
}

func (s *WidgetSessionService) Revoke(ctx context.Context, sessionID, clientID string) error {
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(clientID) == "" {
		return ErrInvalidWidgetSession
	}
	return s.repository.Revoke(ctx, sessionID, clientID, s.now().UTC())
}

func newWidgetSessionToken() (string, error) {
	randomBytes := make([]byte, widgetSessionTokenBytes)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}
	return widgetSessionTokenPrefix + base64.RawURLEncoding.EncodeToString(randomBytes), nil
}

func hashWidgetSessionToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func isValidWidgetSessionToken(token string) bool {
	if !strings.HasPrefix(token, widgetSessionTokenPrefix) {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(token, widgetSessionTokenPrefix))
	return err == nil && len(decoded) == widgetSessionTokenBytes
}

func mapWidgetClientConfig(client *repository.WidgetBootstrapClient) WidgetClientConfig {
	return WidgetClientConfig{
		Name:                     client.Name,
		WidgetBrandName:          client.WidgetBrandName,
		WidgetLogoURL:            client.WidgetLogoURL,
		WidgetPrimaryColor:       client.WidgetPrimaryColor,
		WidgetSecondaryColor:     client.WidgetSecondaryColor,
		WidgetRemoveBranding:     client.WidgetRemoveBranding,
		WidgetBrandingAllowed:    client.WidgetBrandingAllowed,
		WidgetTicketingAllowed:   client.WidgetTicketingAllowed,
		WidgetTicketingEnabled:   client.WidgetTicketingEnabled,
		WidgetAdminMsgAllowed:    client.WidgetAdminMsgAllowed,
		WidgetAdminMsgEnabled:    client.WidgetAdminMsgEnabled,
		WidgetImageSearchAllowed: client.WidgetImageSearchAllowed,
		WidgetImageSearchEnabled: client.WidgetImageSearchEnabled,
	}
}
