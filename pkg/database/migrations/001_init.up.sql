-- ============================================================
-- ALVEX Backend — Migration 001: Core Tables
-- ============================================================

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS users (
    id          TEXT PRIMARY KEY,
    email       VARCHAR(255) UNIQUE NOT NULL,
    name        VARCHAR(200) NOT NULL DEFAULT '',
    role        VARCHAR(20)  NOT NULL DEFAULT 'admin',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS clients (
    id               VARCHAR(100) PRIMARY KEY,
    name             VARCHAR(255) NOT NULL,
    domain           VARCHAR(255),
    status           VARCHAR(20)  NOT NULL DEFAULT 'Active',
    provider         VARCHAR(50)  NOT NULL DEFAULT 'Gemini',
    model            VARCHAR(100) NOT NULL DEFAULT 'Gemini Pro',
    api_key          TEXT,
    gemini_api_key   TEXT,
    groq_api_key     TEXT,
    groq_fallback_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    system_persona   TEXT,
    webhook_url      VARCHAR(500),
    temperature      DECIMAL(3,1) NOT NULL DEFAULT 0.7,
    strict_adherence BOOLEAN      NOT NULL DEFAULT TRUE,
    billing_plan     VARCHAR(50)  NOT NULL DEFAULT 'Basic',
    custom_rate      DECIMAL(10,2),
    portal_token     VARCHAR(64),
    scraped_content  TEXT,
    scrape_synced_at TIMESTAMPTZ,
    scrape_enabled   BOOLEAN NOT NULL DEFAULT FALSE,
    scrape_interval_hours INTEGER NOT NULL DEFAULT 24,
    widget_chat_enabled   BOOLEAN NOT NULL DEFAULT TRUE,
    widget_ticketing_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    widget_admin_msg_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    widget_image_search_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    widget_ticketing_allowed BOOLEAN NOT NULL DEFAULT TRUE,
    widget_admin_msg_allowed BOOLEAN NOT NULL DEFAULT TRUE,
    widget_image_search_allowed BOOLEAN NOT NULL DEFAULT TRUE,
    widget_brand_name   TEXT,
    widget_logo_url     TEXT,
    widget_primary_color VARCHAR(50),
    widget_secondary_color VARCHAR(50),
    widget_remove_branding BOOLEAN NOT NULL DEFAULT FALSE,
    widget_branding_allowed BOOLEAN NOT NULL DEFAULT TRUE,
    guardrails_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    guardrails_reply   TEXT,
    chat_retention_days INTEGER NOT NULL DEFAULT 30,
    owner_id         TEXT REFERENCES users(id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS invoices (
    id          VARCHAR(50)   PRIMARY KEY,
    client_id   VARCHAR(100)  REFERENCES clients(id) ON DELETE SET NULL,
    client_name VARCHAR(255)  NOT NULL,
    amount      DECIMAL(10,2) NOT NULL,
    status      VARCHAR(20)   NOT NULL DEFAULT 'Pending',
    due_date    DATE,
    paid_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS activity_logs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id   VARCHAR(100) REFERENCES clients(id) ON DELETE SET NULL,
    client_name VARCHAR(255) NOT NULL,
    channel     VARCHAR(20)  NOT NULL DEFAULT 'web',
    user_ref    VARCHAR(100),
    session_id  VARCHAR(100),
    message     TEXT,
    ai_response TEXT,
    status      VARCHAR(30)  NOT NULL DEFAULT 'Handling...',
    latency_ms  INTEGER      NOT NULL DEFAULT 0,
    is_ticket   BOOLEAN      NOT NULL DEFAULT FALSE,
    reaction    TEXT,
    image_data  TEXT,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS faqs (
    id         VARCHAR(100) PRIMARY KEY,
    client_id  VARCHAR(100) REFERENCES clients(id) ON DELETE CASCADE,
    question   TEXT NOT NULL,
    answer     TEXT NOT NULL,
    is_approved BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_clients_status   ON clients(status);
CREATE INDEX IF NOT EXISTS idx_clients_owner    ON clients(owner_id);
CREATE INDEX IF NOT EXISTS idx_clients_billing  ON clients(billing_plan);
CREATE INDEX IF NOT EXISTS idx_invoices_client  ON invoices(client_id);
CREATE INDEX IF NOT EXISTS idx_invoices_status  ON invoices(status);
CREATE INDEX IF NOT EXISTS idx_activity_client  ON activity_logs(client_id);
CREATE INDEX IF NOT EXISTS idx_activity_created ON activity_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_faqs_client      ON faqs(client_id);
