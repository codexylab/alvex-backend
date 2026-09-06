CREATE TABLE IF NOT EXISTS checkout_signups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    app_user_id TEXT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
    email VARCHAR(255) NOT NULL,
    organization_name VARCHAR(255) NOT NULL,
    client_name VARCHAR(255) NOT NULL,
    domain VARCHAR(500) NOT NULL,
    provider VARCHAR(50) NOT NULL,
    model VARCHAR(100) NOT NULL,
    billing_plan VARCHAR(50) NOT NULL,
    stripe_session_id VARCHAR(255) UNIQUE,
    status VARCHAR(30) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'checkout_created', 'paid', 'provisioned', 'failed', 'expired')),
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS subscriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    client_id VARCHAR(100) NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    provider VARCHAR(30) NOT NULL DEFAULT 'stripe',
    provider_customer_id VARCHAR(255) NOT NULL,
    provider_subscription_id VARCHAR(255) NOT NULL UNIQUE,
    plan VARCHAR(50) NOT NULL,
    status VARCHAR(30) NOT NULL,
    current_period_end TIMESTAMPTZ,
    cancel_at_period_end BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_checkout_signups_user
    ON checkout_signups(app_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_subscriptions_organization
    ON subscriptions(organization_id, status);
CREATE INDEX IF NOT EXISTS idx_subscriptions_customer
    ON subscriptions(provider_customer_id);
