-- Expand-only migration: adds tenant ownership and operational ledgers while
-- preserving all legacy columns for a backward-compatible rollout.

CREATE TABLE IF NOT EXISTS app_users (
    id TEXT PRIMARY KEY,
    supabase_user_id TEXT NOT NULL UNIQUE,
    email VARCHAR(255) NOT NULL UNIQUE,
    name VARCHAR(200) NOT NULL DEFAULT '',
    platform_role VARCHAR(30) NOT NULL DEFAULT 'platform_user'
        CHECK (platform_role IN ('platform_user', 'super_admin')),
    status VARCHAR(20) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'disabled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO app_users (id, supabase_user_id, email, name, platform_role, created_at, updated_at)
SELECT id, id, email, name,
       CASE WHEN role = 'admin' THEN 'super_admin' ELSE 'platform_user' END,
       created_at, updated_at
FROM users
ON CONFLICT (id) DO UPDATE
SET supabase_user_id = EXCLUDED.supabase_user_id,
    email = EXCLUDED.email,
    name = EXCLUDED.name,
    updated_at = EXCLUDED.updated_at;

CREATE TABLE IF NOT EXISTS organizations (
    id TEXT PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(120) NOT NULL UNIQUE,
    type VARCHAR(30) NOT NULL DEFAULT 'direct_customer'
        CHECK (type IN ('platform', 'direct_customer', 'reseller', 'reseller_customer')),
    parent_organization_id TEXT REFERENCES organizations(id) ON DELETE RESTRICT,
    status VARCHAR(20) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'suspended', 'archived')),
    branding JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS organization_memberships (
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
    role VARCHAR(30) NOT NULL
        CHECK (role IN ('owner', 'admin', 'agent', 'billing_manager', 'viewer')),
    status VARCHAR(20) NOT NULL DEFAULT 'active'
        CHECK (status IN ('invited', 'active', 'disabled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (organization_id, user_id)
);

ALTER TABLE clients ADD COLUMN IF NOT EXISTS organization_id TEXT REFERENCES organizations(id) ON DELETE RESTRICT;
ALTER TABLE clients DROP CONSTRAINT IF EXISTS clients_owner_id_fkey;
ALTER TABLE clients ADD CONSTRAINT clients_owner_id_fkey
    FOREIGN KEY (owner_id) REFERENCES app_users(id) ON DELETE SET NULL;
ALTER TABLE clients ADD COLUMN IF NOT EXISTS daily_msg_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE clients ADD COLUMN IF NOT EXISTS msg_count_date VARCHAR(20);
ALTER TABLE clients ADD COLUMN IF NOT EXISTS stripe_customer_id VARCHAR(100);
ALTER TABLE clients ADD COLUMN IF NOT EXISTS stripe_subscription_id VARCHAR(100);
ALTER TABLE clients ADD COLUMN IF NOT EXISTS onboarding_status VARCHAR(50) NOT NULL DEFAULT 'complete';
ALTER TABLE activity_logs ADD COLUMN IF NOT EXISTS needs_human BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE activity_logs ADD COLUMN IF NOT EXISTS human_reply TEXT;
ALTER TABLE activity_logs ADD COLUMN IF NOT EXISTS replied_at TIMESTAMPTZ;
ALTER TABLE activity_logs ADD COLUMN IF NOT EXISTS handoff_reason TEXT;

CREATE TABLE IF NOT EXISTS documents (
    id VARCHAR(100) PRIMARY KEY,
    client_id VARCHAR(100) NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    filename VARCHAR(255) NOT NULL,
    file_type VARCHAR(50) NOT NULL,
    file_size BIGINT NOT NULL DEFAULT 0,
    status VARCHAR(30) NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'processing', 'processed', 'failed')),
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS document_chunks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id VARCHAR(100) REFERENCES documents(id) ON DELETE CASCADE,
    client_id VARCHAR(100) NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    embedding TEXT NOT NULL,
    source_url VARCHAR(500) NOT NULL DEFAULT '',
    chunk_index INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (document_id, chunk_index)
);

ALTER TABLE documents ADD COLUMN IF NOT EXISTS error_message TEXT;
ALTER TABLE documents ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE document_chunks ADD COLUMN IF NOT EXISTS document_id VARCHAR(100) REFERENCES documents(id) ON DELETE CASCADE;

CREATE TABLE IF NOT EXISTS leads (
    id VARCHAR(100) PRIMARY KEY,
    client_id VARCHAR(100) NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255),
    phone VARCHAR(50),
    session_id VARCHAR(100),
    source VARCHAR(50) NOT NULL DEFAULT 'widget',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS key_audit_logs (
    id VARCHAR(100) PRIMARY KEY,
    client_id VARCHAR(100) NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    key_type VARCHAR(50) NOT NULL,
    rotated_by VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id TEXT REFERENCES organizations(id) ON DELETE SET NULL,
    actor_user_id TEXT REFERENCES app_users(id) ON DELETE SET NULL,
    action VARCHAR(120) NOT NULL,
    resource_type VARCHAR(80) NOT NULL,
    resource_id TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    ip_address INET,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS webhook_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider VARCHAR(30) NOT NULL,
    provider_event_id VARCHAR(255) NOT NULL,
    event_type VARCHAR(120) NOT NULL,
    status VARCHAR(30) NOT NULL DEFAULT 'received',
    payload_sha256 CHAR(64) NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    processed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (provider, provider_event_id)
);

CREATE TABLE IF NOT EXISTS api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    client_id VARCHAR(100) REFERENCES clients(id) ON DELETE CASCADE,
    name VARCHAR(120) NOT NULL,
    key_prefix VARCHAR(24) NOT NULL,
    secret_hash CHAR(64) NOT NULL UNIQUE,
    scopes TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    expires_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    created_by TEXT REFERENCES app_users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS widget_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id VARCHAR(100) NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    origin VARCHAR(500) NOT NULL,
    token_hash CHAR(64) NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_organizations_parent ON organizations(parent_organization_id);
CREATE INDEX IF NOT EXISTS idx_app_users_supabase ON app_users(supabase_user_id);
CREATE INDEX IF NOT EXISTS idx_memberships_user ON organization_memberships(user_id, status);
CREATE INDEX IF NOT EXISTS idx_clients_organization ON clients(organization_id, status);
CREATE INDEX IF NOT EXISTS idx_documents_client ON documents(client_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_chunks_document ON document_chunks(document_id, chunk_index);
CREATE INDEX IF NOT EXISTS idx_chunks_client ON document_chunks(client_id);
CREATE INDEX IF NOT EXISTS idx_leads_client ON leads(client_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_org_created ON audit_logs(organization_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_webhook_status_created ON webhook_events(status, created_at);
CREATE INDEX IF NOT EXISTS idx_api_keys_org ON api_keys(organization_id, revoked_at);
CREATE INDEX IF NOT EXISTS idx_widget_sessions_client ON widget_sessions(client_id, expires_at);

-- Deterministic legacy tenant keeps this migration backward compatible. A
-- later data migration can split clients into their final organizations.
INSERT INTO organizations (id, name, slug, type)
VALUES ('org_legacy', 'Legacy Workspace', 'legacy-workspace', 'direct_customer')
ON CONFLICT (id) DO NOTHING;

UPDATE clients
SET organization_id = 'org_legacy'
WHERE organization_id IS NULL;

INSERT INTO organization_memberships (organization_id, user_id, role, status)
SELECT 'org_legacy', id, 'owner', 'active'
FROM app_users
ON CONFLICT (organization_id, user_id) DO NOTHING;

ALTER TABLE clients ALTER COLUMN organization_id SET NOT NULL;
