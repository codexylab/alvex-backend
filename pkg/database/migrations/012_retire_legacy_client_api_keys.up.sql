ALTER TABLE clients
    ADD COLUMN IF NOT EXISTS openai_api_key TEXT;

-- clients.api_key historically mixed generated ALVX integration credentials
-- with provider secrets. Preserve every legacy value in the selected
-- provider's write-only column, then retire the ambiguous authentication path.
-- Generated ALVX values are recognized and rejected by provider resolution.
UPDATE clients
SET openai_api_key = api_key
WHERE provider = 'OpenAI'
  AND COALESCE(openai_api_key, '') = ''
  AND COALESCE(api_key, '') <> '';

UPDATE clients
SET gemini_api_key = api_key
WHERE provider = 'Gemini'
  AND COALESCE(gemini_api_key, '') = ''
  AND COALESCE(api_key, '') <> '';

UPDATE clients
SET groq_api_key = api_key
WHERE provider = 'Groq'
  AND COALESCE(groq_api_key, '') = ''
  AND COALESCE(api_key, '') <> '';

UPDATE clients SET api_key = NULL WHERE api_key IS NOT NULL;

COMMENT ON COLUMN clients.api_key IS
    'Deprecated. Machine credentials are stored as hashes in api_keys.';
