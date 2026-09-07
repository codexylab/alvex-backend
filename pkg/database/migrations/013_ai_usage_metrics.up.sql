CREATE TABLE IF NOT EXISTS ai_usage_events (
    id                TEXT PRIMARY KEY,
    organization_id   TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    client_id         TEXT NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    provider          TEXT NOT NULL,
    model             TEXT NOT NULL,
    operation         TEXT NOT NULL,
    prompt_tokens     INTEGER NOT NULL DEFAULT 0 CHECK (prompt_tokens >= 0),
    completion_tokens INTEGER NOT NULL DEFAULT 0 CHECK (completion_tokens >= 0),
    total_tokens      INTEGER NOT NULL DEFAULT 0 CHECK (total_tokens >= 0),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ai_usage_organization_created
    ON ai_usage_events(organization_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_ai_usage_client_created
    ON ai_usage_events(client_id, created_at DESC);
