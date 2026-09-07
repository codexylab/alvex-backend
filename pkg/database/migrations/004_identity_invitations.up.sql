CREATE TABLE IF NOT EXISTS identity_invitations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) NOT NULL,
    supabase_user_id TEXT NOT NULL,
    client_id VARCHAR(100) NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    invited_by TEXT REFERENCES app_users(id) ON DELETE SET NULL,
    role VARCHAR(30) NOT NULL DEFAULT 'client_admin',
    status VARCHAR(20) NOT NULL DEFAULT 'sent'
        CHECK (status IN ('sent', 'accepted', 'expired', 'revoked')),
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (email, client_id)
);

CREATE INDEX IF NOT EXISTS idx_identity_invitations_client
    ON identity_invitations(client_id, status);
