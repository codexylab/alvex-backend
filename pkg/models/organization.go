package models

import "time"

type OrganizationType string

const (
	OrganizationPlatform         OrganizationType = "platform"
	OrganizationDirectCustomer   OrganizationType = "direct_customer"
	OrganizationReseller         OrganizationType = "reseller"
	OrganizationResellerCustomer OrganizationType = "reseller_customer"
)

type OrganizationRole string

const (
	OrganizationOwner          OrganizationRole = "owner"
	OrganizationAdmin          OrganizationRole = "admin"
	OrganizationAgent          OrganizationRole = "agent"
	OrganizationBillingManager OrganizationRole = "billing_manager"
	OrganizationViewer         OrganizationRole = "viewer"
)

// Organization is the top-level tenant boundary for direct and reseller accounts.
type Organization struct {
	ID                   string           `json:"id"`
	Name                 string           `json:"name"`
	Slug                 string           `json:"slug"`
	Type                 OrganizationType `json:"type"`
	ParentOrganizationID *string          `json:"parent_organization_id,omitempty"`
	Status               string           `json:"status"`
	CreatedAt            time.Time        `json:"created_at"`
	UpdatedAt            time.Time        `json:"updated_at"`
}

// OrganizationMembership represents one user's role inside one tenant.
type OrganizationMembership struct {
	Organization Organization     `json:"organization"`
	Role         OrganizationRole `json:"role"`
	Status       string           `json:"status"`
}
