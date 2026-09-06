ALTER TABLE document_chunks
    ADD COLUMN IF NOT EXISTS document_id VARCHAR(100) REFERENCES documents(id) ON DELETE CASCADE;

CREATE TABLE IF NOT EXISTS document_contents (
    document_id VARCHAR(100) PRIMARY KEY REFERENCES documents(id) ON DELETE CASCADE,
    client_id VARCHAR(100) NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    content_sha256 CHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_document_contents_client
    ON document_contents(client_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_chunks_document_position
    ON document_chunks(document_id, chunk_index)
    WHERE document_id IS NOT NULL;
