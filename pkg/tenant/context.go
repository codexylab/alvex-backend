package tenant

import (
	"context"
	"errors"
)

type scopeKey struct{}

// ErrMissingOrganizationScope indicates that tenant-owned data was accessed
// without an organization selected by trusted middleware.
var ErrMissingOrganizationScope = errors.New("organization scope is required")

// Scope is the trusted tenant identity attached to an authenticated request.
type Scope struct {
	OrganizationID   string
	OrganizationType string
	UserID           string
	Role             string
}

// WithScope attaches a verified organization membership to a context.
func WithScope(ctx context.Context, scope Scope) context.Context {
	return context.WithValue(ctx, scopeKey{}, scope)
}

// FromContext returns the verified organization scope, when present.
func FromContext(ctx context.Context) (Scope, bool) {
	scope, ok := ctx.Value(scopeKey{}).(Scope)
	return scope, ok && scope.OrganizationID != ""
}

// RequireOrganizationID returns the trusted tenant ID or a fail-closed error.
func RequireOrganizationID(ctx context.Context) (string, error) {
	scope, ok := FromContext(ctx)
	if !ok {
		return "", ErrMissingOrganizationScope
	}
	return scope.OrganizationID, nil
}
