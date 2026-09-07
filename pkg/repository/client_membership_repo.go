package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/codexylab/alvex-backend/pkg/database"
)

var (
	// ErrClientMembershipNotFound deliberately hides whether a client exists.
	ErrClientMembershipNotFound = errors.New("active client membership not found")
	ErrClientSelectionRequired  = errors.New("client selection is required")
)

// ClientAccess is the authorization scope for one portal request.
type ClientAccess struct {
	ClientID       string
	OrganizationID string
	UserID         string
	Role           string
}

// ClientMembershipRepository resolves a verified application user to a client.
type ClientMembershipRepository interface {
	ResolveActiveAccess(ctx context.Context, userID, requestedClientID string) (*ClientAccess, error)
}

type SQLClientMembershipRepository struct {
	DB *database.DB
}

func NewSQLClientMembershipRepository(db *database.DB) *SQLClientMembershipRepository {
	return &SQLClientMembershipRepository{DB: db}
}

// ResolveActiveAccess returns exactly one active membership. A requested client
// is always checked in the query so guessed IDs cannot cross tenant boundaries.
func (r *SQLClientMembershipRepository) ResolveActiveAccess(
	ctx context.Context,
	userID string,
	requestedClientID string,
) (*ClientAccess, error) {
	if requestedClientID != "" {
		return r.findAccess(ctx, userID, requestedClientID)
	}

	rows, err := r.DB.QueryContext(ctx, r.DB.Adapt(`
		SELECT cm.client_id, c.organization_id, cm.user_id, cm.role
		FROM client_memberships cm
		JOIN clients c ON c.id = cm.client_id
		JOIN organizations o ON o.id = c.organization_id
		JOIN app_users u ON u.id = cm.user_id
		WHERE cm.user_id = $1
		  AND cm.status = 'active'
		  AND c.status = 'Active'
		  AND o.status = 'active'
		  AND u.status = 'active'
		ORDER BY cm.created_at ASC
		LIMIT 2`), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accesses []ClientAccess
	for rows.Next() {
		var access ClientAccess
		if err := rows.Scan(&access.ClientID, &access.OrganizationID, &access.UserID, &access.Role); err != nil {
			return nil, err
		}
		accesses = append(accesses, access)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(accesses) == 0 {
		return nil, ErrClientMembershipNotFound
	}
	if len(accesses) > 1 {
		return nil, ErrClientSelectionRequired
	}
	return &accesses[0], nil
}

func (r *SQLClientMembershipRepository) findAccess(
	ctx context.Context,
	userID string,
	clientID string,
) (*ClientAccess, error) {
	var access ClientAccess
	err := r.DB.QueryRowContext(ctx, r.DB.Adapt(`
		SELECT cm.client_id, c.organization_id, cm.user_id, cm.role
		FROM client_memberships cm
		JOIN clients c ON c.id = cm.client_id
		JOIN organizations o ON o.id = c.organization_id
		JOIN app_users u ON u.id = cm.user_id
		WHERE cm.user_id = $1
		  AND cm.client_id = $2
		  AND cm.status = 'active'
		  AND c.status = 'Active'
		  AND o.status = 'active'
		  AND u.status = 'active'`), userID, clientID).
		Scan(&access.ClientID, &access.OrganizationID, &access.UserID, &access.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrClientMembershipNotFound
	}
	if err != nil {
		return nil, err
	}
	return &access, nil
}
