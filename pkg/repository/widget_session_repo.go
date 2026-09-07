package repository

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/codexylab/alvex-backend/pkg/database"
)

var ErrWidgetOriginForbidden = errors.New("widget origin is not allowed")

type WidgetBootstrapClient struct {
	ID                       string
	Name                     string
	Domain                   string
	AllowedOrigins           []string
	WidgetBrandName          string
	WidgetLogoURL            string
	WidgetPrimaryColor       string
	WidgetSecondaryColor     string
	WidgetRemoveBranding     bool
	WidgetBrandingAllowed    bool
	WidgetTicketingAllowed   bool
	WidgetTicketingEnabled   bool
	WidgetAdminMsgAllowed    bool
	WidgetAdminMsgEnabled    bool
	WidgetImageSearchAllowed bool
	WidgetImageSearchEnabled bool
}

type WidgetSession struct {
	ID                 string
	ClientID           string
	Origin             string
	ExpiresAt          time.Time
	TicketingEnabled   bool
	AdminMsgEnabled    bool
	ImageSearchEnabled bool
}

type WidgetSessionRepository interface {
	Create(ctx context.Context, sessionID, clientID, origin, tokenHash string, expiresAt time.Time) (*WidgetBootstrapClient, error)
	Resolve(ctx context.Context, clientID, origin, tokenHash string, now time.Time) (*WidgetSession, error)
	Revoke(ctx context.Context, sessionID, clientID string, revokedAt time.Time) error
}

type SQLWidgetSessionRepository struct {
	DB *database.DB
}

func NewSQLWidgetSessionRepository(db *database.DB) *SQLWidgetSessionRepository {
	return &SQLWidgetSessionRepository{DB: db}
}

// IsOriginAllowed supports CORS preflight validation without creating a
// session. The authenticated request is validated again when resolving its
// origin-bound bearer token.
func (r *SQLWidgetSessionRepository) IsOriginAllowed(ctx context.Context, clientID, origin string) bool {
	client, err := r.getActiveClient(ctx, strings.TrimSpace(clientID))
	return err == nil && isAllowedWidgetOrigin(origin, client.Domain, client.AllowedOrigins)
}

func (r *SQLWidgetSessionRepository) Create(
	ctx context.Context,
	sessionID string,
	clientID string,
	origin string,
	tokenHash string,
	expiresAt time.Time,
) (*WidgetBootstrapClient, error) {
	client, err := r.getActiveClient(ctx, clientID)
	if err != nil {
		return nil, err
	}
	if !isAllowedWidgetOrigin(origin, client.Domain, client.AllowedOrigins) {
		return nil, ErrWidgetOriginForbidden
	}

	_, err = r.DB.ExecContext(ctx, r.DB.Adapt(`
		INSERT INTO widget_sessions (id, client_id, origin, token_hash, expires_at)
		VALUES ($1, $2, $3, $4, $5)`), sessionID, clientID, normalizeWidgetOrigin(origin), tokenHash, expiresAt)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (r *SQLWidgetSessionRepository) Resolve(
	ctx context.Context,
	clientID string,
	origin string,
	tokenHash string,
	now time.Time,
) (*WidgetSession, error) {
	var session WidgetSession
	err := r.DB.QueryRowContext(ctx, r.DB.Adapt(`
		SELECT ws.id, ws.client_id, ws.origin, ws.expires_at,
		       (COALESCE(c.widget_ticketing_allowed, true) AND COALESCE(c.widget_ticketing_enabled, true)),
		       (COALESCE(c.widget_admin_msg_allowed, true) AND COALESCE(c.widget_admin_msg_enabled, true)),
		       (COALESCE(c.widget_image_search_allowed, true) AND COALESCE(c.widget_image_search_enabled, true))
		FROM widget_sessions ws
		JOIN clients c ON c.id = ws.client_id
		WHERE ws.client_id = $1 AND ws.token_hash = $2
		  AND ws.origin = $3 AND ws.expires_at > $4 AND ws.revoked_at IS NULL
		  AND c.status = 'Active'`),
		clientID, tokenHash, normalizeWidgetOrigin(origin), now,
	).Scan(
		&session.ID,
		&session.ClientID,
		&session.Origin,
		&session.ExpiresAt,
		&session.TicketingEnabled,
		&session.AdminMsgEnabled,
		&session.ImageSearchEnabled,
	)
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *SQLWidgetSessionRepository) Revoke(
	ctx context.Context,
	sessionID string,
	clientID string,
	revokedAt time.Time,
) error {
	_, err := r.DB.ExecContext(ctx, r.DB.Adapt(`
		UPDATE widget_sessions
		SET revoked_at = $1
		WHERE id = $2 AND client_id = $3 AND revoked_at IS NULL`), revokedAt, sessionID, clientID)
	return err
}

func (r *SQLWidgetSessionRepository) DeleteExpired(ctx context.Context, now time.Time) (int64, error) {
	result, err := r.DB.ExecContext(ctx, r.DB.Adapt(`
		DELETE FROM widget_sessions
		WHERE expires_at <= $1 OR revoked_at IS NOT NULL`), now)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *SQLWidgetSessionRepository) getActiveClient(ctx context.Context, clientID string) (*WidgetBootstrapClient, error) {
	var client WidgetBootstrapClient
	var originsJSON string
	err := r.DB.QueryRowContext(ctx, r.DB.Adapt(`
		SELECT id, name, COALESCE(domain, ''), COALESCE(allowed_origins, '[]'),
		       COALESCE(widget_brand_name, ''), COALESCE(widget_logo_url, ''),
		       COALESCE(widget_primary_color, ''), COALESCE(widget_secondary_color, ''),
		       COALESCE(widget_remove_branding, false),
		       COALESCE(widget_branding_allowed, true),
		       COALESCE(widget_ticketing_allowed, true),
		       COALESCE(widget_ticketing_enabled, true),
		       COALESCE(widget_admin_msg_allowed, true),
		       COALESCE(widget_admin_msg_enabled, true),
		       COALESCE(widget_image_search_allowed, true),
		       COALESCE(widget_image_search_enabled, true)
		FROM clients WHERE id = $1 AND status = 'Active'`), clientID).Scan(
		&client.ID, &client.Name, &client.Domain, &originsJSON,
		&client.WidgetBrandName, &client.WidgetLogoURL,
		&client.WidgetPrimaryColor, &client.WidgetSecondaryColor,
		&client.WidgetRemoveBranding, &client.WidgetBrandingAllowed,
		&client.WidgetTicketingAllowed, &client.WidgetTicketingEnabled,
		&client.WidgetAdminMsgAllowed, &client.WidgetAdminMsgEnabled,
		&client.WidgetImageSearchAllowed, &client.WidgetImageSearchEnabled,
	)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(originsJSON), &client.AllowedOrigins); err != nil {
		return nil, err
	}
	return &client, nil
}

func isAllowedWidgetOrigin(origin, domain string, allowedOrigins []string) bool {
	normalized := normalizeWidgetOrigin(origin)
	if normalized == "" {
		return false
	}
	for _, allowed := range allowedOrigins {
		if normalizeWidgetOrigin(allowed) == normalized {
			return true
		}
	}
	if len(allowedOrigins) > 0 {
		return false
	}

	originURL, _ := url.Parse(normalized)
	domainValue := strings.TrimSpace(domain)
	domainURL, err := url.Parse(domainValue)
	if err != nil || domainURL.Hostname() == "" {
		domainURL, err = url.Parse("https://" + domainValue)
	}
	if err != nil || originURL.Hostname() == "" || !strings.EqualFold(originURL.Hostname(), domainURL.Hostname()) {
		return false
	}

	// Bare client domains represent their production HTTPS origin. Explicit URLs
	// must match their complete normalized origin, including a non-default port.
	if !strings.Contains(domainValue, "://") {
		return originURL.Scheme == "https"
	}
	return normalized == normalizeWidgetOrigin(domainValue)
}

func normalizeWidgetOrigin(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return ""
	}
	return strings.ToLower(parsed.Scheme + "://" + parsed.Host)
}
