package services

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/codexylab/alvex-backend/pkg/models"
	"github.com/codexylab/alvex-backend/pkg/repository"
)

// RAGService coordinates semantic document chunking, embedding generation,
// and vector retrieval for client chatbots.
type RAGService struct {
	ChunkRepo    repository.ChunkRepository
	EmbeddingSvc EmbeddingGenerator
}

type EmbeddingGenerator interface {
	GenerateEmbedding(ctx context.Context, text string) ([]float32, error)
}

// NewRAGService creates a new RAGService.
func NewRAGService(chunkRepo repository.ChunkRepository, embeddingSvc EmbeddingGenerator) *RAGService {
	return &RAGService{
		ChunkRepo:    chunkRepo,
		EmbeddingSvc: embeddingSvc,
	}
}

// IndexContent splits raw content into chunks, generates vector embeddings,
// and saves them to the repository for semantic retrieval.
func (r *RAGService) IndexContent(ctx context.Context, clientID, sourceURL, content string) error {
	return r.indexSource(ctx, clientID, "", sourceURL, content)
}

// IndexDocument indexes one uploaded document without deleting chunks that
// belong to other documents or to the client's website.
func (r *RAGService) IndexDocument(
	ctx context.Context,
	clientID string,
	documentID string,
	filename string,
	content string,
) error {
	if documentID == "" {
		return fmt.Errorf("document ID is required")
	}
	return r.indexSource(ctx, clientID, documentID, filename, content)
}

func (r *RAGService) indexSource(
	ctx context.Context,
	clientID string,
	documentID string,
	sourceURL string,
	content string,
) error {
	if strings.TrimSpace(content) == "" {
		return nil
	}

	chunks := ChunkText(content, 150, 25)
	if len(chunks) == 0 {
		return nil
	}

	slog.Info("indexing chunks for client", "client_id", clientID, "chunks_count", len(chunks))

	indexedChunks := make([]models.DocumentChunk, 0, len(chunks))
	for i, chunkText := range chunks {
		var emb []float32
		if r.EmbeddingSvc != nil {
			generated, err := r.EmbeddingSvc.GenerateEmbedding(ctx, chunkText)
			if err != nil {
				return fmt.Errorf("generate embedding for chunk %d: %w", i, err)
			}
			emb = generated
		}

		indexedChunks = append(indexedChunks, models.DocumentChunk{
			ID:         uuid.NewString(),
			DocumentID: documentID,
			ClientID:   clientID,
			Content:    chunkText,
			Embedding:  emb,
			SourceURL:  sourceURL,
			ChunkIndex: i,
			CreatedAt:  time.Now().UTC(),
		})
	}

	if err := r.ChunkRepo.ReplaceSourceChunks(ctx, clientID, documentID, sourceURL, indexedChunks); err != nil {
		return fmt.Errorf("replace indexed chunks: %w", err)
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
	written := 0
	for _, c := range chunks {
		if len(queryEmb) > 0 {
			if len(c.Embedding) == 0 || CosineSimilarity(queryEmb, c.Embedding) < minimumKnowledgeSimilarity {
				continue
			}
		}
		if written > 0 {
			sb.WriteString("\n---\n")
		}
		sb.WriteString(c.Content)
		written++
	}

	return sb.String(), nil
}

const minimumKnowledgeSimilarity float32 = 0.15
