package repository

import (
	"context"
	"encoding/json"
	"math"
	"sort"
	"time"

	"github.com/codexylab/alvex-backend/internal/database"
	"github.com/codexylab/alvex-backend/internal/models"
)

// ChunkRepository manages storage and semantic retrieval of document chunks.
type ChunkRepository interface {
	InsertChunk(ctx context.Context, chunk *models.DocumentChunk) error
	GetChunksByClient(ctx context.Context, clientID string) ([]models.DocumentChunk, error)
	DeleteClientChunks(ctx context.Context, clientID string) error
	SearchSimilar(ctx context.Context, clientID string, queryEmbedding []float32, limit int) ([]models.DocumentChunk, error)
}

// SQLChunkRepository implements ChunkRepository using database/sql.
type SQLChunkRepository struct {
	DB *database.DB
}

// NewSQLChunkRepository creates a new SQLChunkRepository instance.
func NewSQLChunkRepository(db *database.DB) *SQLChunkRepository {
	return &SQLChunkRepository{DB: db}
}

// InsertChunk saves a single chunk with JSON-serialized embedding into the database.
func (r *SQLChunkRepository) InsertChunk(ctx context.Context, chunk *models.DocumentChunk) error {
	embJSON, err := json.Marshal(chunk.Embedding)
	if err != nil {
		return err
	}

	if chunk.CreatedAt.IsZero() {
		chunk.CreatedAt = time.Now()
	}

	query := `
		INSERT INTO document_chunks (id, client_id, content, embedding, source_url, chunk_index, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err = r.DB.ExecContext(
		ctx,
		r.DB.Adapt(query),
		chunk.ID,
		chunk.ClientID,
		chunk.Content,
		string(embJSON),
		chunk.SourceURL,
		chunk.ChunkIndex,
		chunk.CreatedAt,
	)
	return err
}

// GetChunksByClient retrieves all chunks belonging to a client.
func (r *SQLChunkRepository) GetChunksByClient(ctx context.Context, clientID string) ([]models.DocumentChunk, error) {
	query := `
		SELECT id, client_id, content, embedding, source_url, chunk_index, created_at
		FROM document_chunks
		WHERE client_id = $1
		ORDER BY chunk_index ASC
	`

	rows, err := r.DB.QueryContext(ctx, r.DB.Adapt(query), clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chunks []models.DocumentChunk
	for rows.Next() {
		var c models.DocumentChunk
		var embStr string
		if err := rows.Scan(&c.ID, &c.ClientID, &c.Content, &embStr, &c.SourceURL, &c.ChunkIndex, &c.CreatedAt); err != nil {
			return nil, err
		}
		if embStr != "" {
			_ = json.Unmarshal([]byte(embStr), &c.Embedding)
		}
		chunks = append(chunks, c)
	}

	return chunks, nil
}

// DeleteClientChunks removes all indexed chunks for a given client.
func (r *SQLChunkRepository) DeleteClientChunks(ctx context.Context, clientID string) error {
	query := `DELETE FROM document_chunks WHERE client_id = $1`
	_, err := r.DB.ExecContext(ctx, r.DB.Adapt(query), clientID)
	return err
}

// scoredChunk is an internal helper for ranking chunks by cosine similarity.
type scoredChunk struct {
	chunk      models.DocumentChunk
	similarity float32
}

// SearchSimilar computes cosine similarity in Go across all stored client chunks and returns top K.
func (r *SQLChunkRepository) SearchSimilar(ctx context.Context, clientID string, queryEmbedding []float32, limit int) ([]models.DocumentChunk, error) {
	if limit <= 0 {
		limit = 5
	}

	chunks, err := r.GetChunksByClient(ctx, clientID)
	if err != nil {
		return nil, err
	}

	if len(chunks) == 0 {
		return nil, nil
	}

	// If no query embedding, return first K chunks
	if len(queryEmbedding) == 0 {
		if len(chunks) > limit {
			return chunks[:limit], nil
		}
		return chunks, nil
	}

	var scored []scoredChunk
	for _, c := range chunks {
		sim := calculateCosineSim(queryEmbedding, c.Embedding)
		scored = append(scored, scoredChunk{
			chunk:      c,
			similarity: sim,
		})
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].similarity > scored[j].similarity
	})

	resultCount := limit
	if len(scored) < resultCount {
		resultCount = len(scored)
	}

	results := make([]models.DocumentChunk, resultCount)
	for i := 0; i < resultCount; i++ {
		results[i] = scored[i].chunk
	}

	return results, nil
}

func calculateCosineSim(a, b []float32) float32 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := 0; i < len(a); i++ {
		valA := float64(a[i])
		valB := float64(b[i])
		dot += valA * valB
		normA += valA * valA
		normB += valB * valB
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
}
