package repository

import (
	"context"

	"github.com/codexylab/alvex-backend/pkg/database"
	"github.com/codexylab/alvex-backend/pkg/models"
)

// OrganizationRepository defines tenant-membership persistence operations.
type OrganizationRepository interface {
	ListActiveMemberships(ctx context.Context, userID string) ([]models.OrganizationMembership, error)
}

// SQLOrganizationRepository stores organizations in the application database.
type SQLOrganizationRepository struct {
	DB *database.DB
}

func NewSQLOrganizationRepository(db *database.DB) *SQLOrganizationRepository {
	return &SQLOrganizationRepository{DB: db}
}

// ListActiveMemberships returns only active memberships and active organizations.
func (r *SQLOrganizationRepository) ListActiveMemberships(ctx context.Context, userID string) ([]models.OrganizationMembership, error) {
	rows, err := r.DB.QueryContext(ctx, r.DB.Adapt(`
		SELECT o.id, o.name, o.slug, o.type, o.parent_organization_id,
		       o.status, o.created_at, o.updated_at, m.role, m.status
		FROM organization_memberships m
		JOIN organizations o ON o.id = m.organization_id
		JOIN app_users u ON u.id = m.user_id
		WHERE m.user_id = $1 AND m.status = 'active' AND o.status = 'active' AND u.status = 'active'
		ORDER BY m.created_at ASC, o.id ASC
	`), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	memberships := make([]models.OrganizationMembership, 0)
	for rows.Next() {
		var membership models.OrganizationMembership
		if err := rows.Scan(
			&membership.Organization.ID,
			&membership.Organization.Name,
			&membership.Organization.Slug,
			&membership.Organization.Type,
			&membership.Organization.ParentOrganizationID,
			&membership.Organization.Status,
			&membership.Organization.CreatedAt,
			&membership.Organization.UpdatedAt,
			&membership.Role,
			&membership.Status,
		); err != nil {
			return nil, err
		}
		memberships = append(memberships, membership)
	}
	return memberships, rows.Err()
}
