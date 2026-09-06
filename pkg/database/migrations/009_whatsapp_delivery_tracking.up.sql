ALTER TABLE clients
    ADD COLUMN IF NOT EXISTS whatsapp_phone_number_id VARCHAR(80);

CREATE UNIQUE INDEX IF NOT EXISTS idx_clients_whatsapp_phone_number
    ON clients(whatsapp_phone_number_id)
    WHERE whatsapp_phone_number_id IS NOT NULL AND whatsapp_phone_number_id <> '';

CREATE TABLE IF NOT EXISTS whatsapp_deliveries (
    provider_message_id VARCHAR(255) PRIMARY KEY,
    client_id VARCHAR(100) NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    recipient VARCHAR(80) NOT NULL,
    direction VARCHAR(20) NOT NULL,
    status VARCHAR(40) NOT NULL,
    provider_timestamp TIMESTAMPTZ,
    error_code VARCHAR(80),
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (direction IN ('inbound', 'outbound'))
);

CREATE INDEX IF NOT EXISTS idx_whatsapp_deliveries_client
    ON whatsapp_deliveries(client_id, updated_at DESC);
