ALTER TABLE invoices
    ADD COLUMN IF NOT EXISTS provider_invoice_id VARCHAR(255);
ALTER TABLE invoices
    ADD COLUMN IF NOT EXISTS provider_status VARCHAR(40);
ALTER TABLE invoices
    ADD COLUMN IF NOT EXISTS currency CHAR(3);

CREATE UNIQUE INDEX IF NOT EXISTS idx_invoices_provider_invoice
    ON invoices(provider_invoice_id)
    WHERE provider_invoice_id IS NOT NULL;
