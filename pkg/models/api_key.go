package models

import "time"

// APIKeyScope is a stable permission granted to a machine credential.
type APIKeyScope string

const (
	APIKeyScopeClientsRead        APIKeyScope = "clients:read"
	APIKeyScopeClientsWrite       APIKeyScope = "clients:write"
	APIKeyScopeConversationsRead  APIKeyScope = "conversations:read"
	APIKeyScopeConversationsWrite APIKeyScope = "conversations:write"
	APIKeyScopeAnalyticsRead      APIKeyScope = "analytics:read"
)

// APIKey contains only the non-secret metadata of a machine credential.
// SecretHash is intentionally kept in the repository layer and never exposed.
type APIKey struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organization_id"`
	ClientID       *string    `json:"client_id,omitempty"`
	Name           string     `json:"name"`
	KeyPrefix      string     `json:"key_prefix"`
	Scopes         []string   `json:"scopes"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	RevokedAt      *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt     *time.Time `json:"last_used_at,omitempty"`
	CreatedBy      *string    `json:"created_by,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// CreateAPIKeyRequest is the administrator-controlled key creation contract.
type CreateAPIKeyRequest struct {
	Name      string     `json:"name"`
	ClientID  *string    `json:"client_id,omitempty"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// CreatedAPIKey returns a raw key exactly once. Subsequent reads return only
// APIKey metadata and its safe prefix.
type CreatedAPIKey struct {
	APIKey
	Secret string `json:"secret"`
}
