-- Replaces long-lived shared portal tokens with user-specific Supabase access.
-- The legacy portal_token column remains during the expand/deploy/contract window,
-- but application code no longer authenticates with it.

CREATE TABLE IF NOT EXISTS client_memberships (
    client_id VARCHAR(100) NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
    role VARCHAR(30) NOT NULL DEFAULT 'client_admin'
        CHECK (role IN ('client_admin', 'client_agent', 'client_viewer')),
    status VARCHAR(20) NOT NULL DEFAULT 'active'
        CHECK (status IN ('invited', 'active', 'disabled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (client_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_client_memberships_user
    ON client_memberships(user_id, status);

-- Existing clients with a mapped owner retain access after the authentication
-- cutover. Token-only portal users require a Supabase invitation before rollout.
INSERT INTO client_memberships (client_id, user_id, role, status)
SELECT id, owner_id, 'client_admin', 'active'
FROM clients
WHERE owner_id IS NOT NULL
ON CONFLICT (client_id, user_id) DO NOTHING;
