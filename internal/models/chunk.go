package models

import "time"

// DocumentChunk represents a text chunk extracted from scraped content or uploaded documents,
// stored with its vector embedding for semantic similarity search.
type DocumentChunk struct {
	ID         string    `json:"id"`
	ClientID   string    `json:"client_id"`
	Content    string    `json:"content"`
	Embedding  []float32 `json:"embedding"`
	SourceURL  string    `json:"source_url"`
	ChunkIndex int       `json:"chunk_index"`
	CreatedAt  time.Time `json:"created_at"`
}
