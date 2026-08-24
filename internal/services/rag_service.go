package services

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/codexylab/alvex-backend/internal/models"
	"github.com/codexylab/alvex-backend/internal/repository"
)

// RAGService coordinates semantic document chunking, embedding generation,
// and vector retrieval for client chatbots.
type RAGService struct {
	ChunkRepo    repository.ChunkRepository
	EmbeddingSvc *EmbeddingService
}

// NewRAGService creates a new RAGService.
func NewRAGService(chunkRepo repository.ChunkRepository, embeddingSvc *EmbeddingService) *RAGService {
	return &RAGService{
		ChunkRepo:    chunkRepo,
		EmbeddingSvc: embeddingSvc,
	}
}

// IndexContent splits raw content into chunks, generates vector embeddings,
// and saves them to the repository for semantic retrieval.
func (r *RAGService) IndexContent(ctx context.Context, clientID, sourceURL, content string) error {
	if strings.TrimSpace(content) == "" {
		return nil
	}

	// 1. Delete previous chunks for this client & source to prevent duplicates
	if err := r.ChunkRepo.DeleteClientChunks(ctx, clientID); err != nil {
		slog.Warn("failed to delete previous chunks", "client_id", clientID, "error", err)
	}

	// 2. Chunk text (150 words per chunk, 25 words overlap)
	chunks := ChunkText(content, 150, 25)
	if len(chunks) == 0 {
		return nil
	}

	slog.Info("indexing chunks for client", "client_id", clientID, "chunks_count", len(chunks))

	// 3. Generate embeddings and save each chunk
	for i, chunkText := range chunks {
		var emb []float32
		if r.EmbeddingSvc != nil {
			var err error
			emb, err = r.EmbeddingSvc.GenerateEmbedding(ctx, chunkText)
			if err != nil {
				slog.Warn("embedding generation failed for chunk", "index", i, "error", err)
			}
		}

		chunkID := fmt.Sprintf("chk_%s_%d_%d", clientID, time.Now().Unix(), i)
		chunkModel := &models.DocumentChunk{
			ID:         chunkID,
			ClientID:   clientID,
			Content:    chunkText,
			Embedding:  emb,
			SourceURL:  sourceURL,
			ChunkIndex: i,
			CreatedAt:  time.Now(),
		}

		if err := r.ChunkRepo.InsertChunk(ctx, chunkModel); err != nil {
			slog.Error("failed to insert chunk", "chunk_id", chunkID, "error", err)
		}
	}

	return nil
}

// RetrieveRelevant queries the semantic index for the top-K most relevant chunks
// to the user's query and returns formatted text for inclusion in the system prompt.
func (r *RAGService) RetrieveRelevant(ctx context.Context, clientID, query string, topK int) (string, error) {
	if topK <= 0 {
		topK = 4
	}

	var queryEmb []float32
	if r.EmbeddingSvc != nil && strings.TrimSpace(query) != "" {
		var err error
		queryEmb, err = r.EmbeddingSvc.GenerateEmbedding(ctx, query)
		if err != nil {
			slog.Warn("failed to generate query embedding, falling back to top chunks", "error", err)
		}
	}

	chunks, err := r.ChunkRepo.SearchSimilar(ctx, clientID, queryEmb, topK)
	if err != nil {
		return "", err
	}

	if len(chunks) == 0 {
		return "", nil
	}

	var sb strings.Builder
	for i, c := range chunks {
		if i > 0 {
			sb.WriteString("\n---\n")
		}
		sb.WriteString(c.Content)
	}

	return sb.String(), nil
}
