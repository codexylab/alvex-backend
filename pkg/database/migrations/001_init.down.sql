DROP INDEX IF EXISTS idx_faqs_client;
DROP INDEX IF EXISTS idx_activity_created;
DROP INDEX IF EXISTS idx_activity_client;
DROP INDEX IF EXISTS idx_invoices_status;
DROP INDEX IF EXISTS idx_invoices_client;
DROP INDEX IF EXISTS idx_clients_billing;
DROP INDEX IF EXISTS idx_clients_owner;
DROP INDEX IF EXISTS idx_clients_status;

DROP TABLE IF EXISTS faqs;
DROP TABLE IF EXISTS activity_logs;
DROP TABLE IF EXISTS invoices;
DROP TABLE IF EXISTS clients;
DROP TABLE IF EXISTS users;
