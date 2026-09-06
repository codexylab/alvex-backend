-- Portal authentication now uses Supabase identities plus client_memberships.
-- Erase the retired shared bearer credentials while retaining the nullable
-- column for a safe contract rollout across independently deployed versions.
UPDATE clients SET portal_token = NULL WHERE portal_token IS NOT NULL;

COMMENT ON COLUMN clients.portal_token IS
    'Deprecated. Portal access is authorized through client_memberships.';
