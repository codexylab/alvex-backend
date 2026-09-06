ALTER TABLE clients
    ADD COLUMN IF NOT EXISTS allowed_origins JSONB NOT NULL DEFAULT '[]'::jsonb;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'clients_allowed_origins_array'
    ) THEN
        ALTER TABLE clients
            ADD CONSTRAINT clients_allowed_origins_array
            CHECK (jsonb_typeof(allowed_origins) = 'array');
    END IF;
END
$$;

CREATE INDEX IF NOT EXISTS idx_widget_sessions_token_active
    ON widget_sessions(token_hash, expires_at)
    WHERE revoked_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_widget_sessions_expiry
    ON widget_sessions(expires_at);
