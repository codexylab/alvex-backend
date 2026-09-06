-- Intentionally irreversible. This expand migration backfills tenant ownership
-- and creates security/audit records that must never be discarded by rollback.
SELECT 1;
