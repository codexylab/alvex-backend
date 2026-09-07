ALTER TABLE webhook_events
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

ALTER TABLE checkout_signups
    ADD COLUMN IF NOT EXISTS organization_id TEXT REFERENCES organizations(id) ON DELETE SET NULL;
ALTER TABLE checkout_signups
    ADD COLUMN IF NOT EXISTS client_id VARCHAR(100) REFERENCES clients(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_webhook_provider_event
    ON webhook_events(provider, provider_event_id, status);
