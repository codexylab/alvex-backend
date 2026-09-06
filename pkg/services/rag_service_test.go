package services

import (
	"context"
	"strings"
	"testing"

	"github.com/codexylab/alvex-backend/pkg/models"
)

type retrievalChunkRepositoryStub struct {
	results []models.DocumentChunk
}

func (s retrievalChunkRepositoryStub) InsertChunk(context.Context, *models.DocumentChunk) error {
	return nil
}
func (s retrievalChunkRepositoryStub) ReplaceSourceChunks(context.Context, string, string, string, []models.DocumentChunk) error {
	return nil
}
func (s retrievalChunkRepositoryStub) GetChunksByClient(context.Context, string) ([]models.DocumentChunk, error) {
	return s.results, nil
}
func (s retrievalChunkRepositoryStub) DeleteClientChunks(context.Context, string) error { return nil }
func (s retrievalChunkRepositoryStub) SearchSimilar(context.Context, string, []float32, int) ([]models.DocumentChunk, error) {
	return s.results, nil
}

func TestChunkText(t *testing.T) {
	text := "One two three four five six seven eight nine ten eleven twelve thirteen fourteen fifteen sixteen seventeen eighteen nineteen twenty"

	// Chunk size 10, overlap 2
	chunks := ChunkText(text, 10, 2)
	if len(chunks) == 0 {
		t.Fatalf("expected non-empty chunks, got 0")
	}

	// Tiny text test
	shortText := "A short test"
	shortChunks := ChunkText(shortText, 50, 10)
	if len(shortChunks) != 1 {
		t.Fatalf("expected 1 chunk for short text, got %d", len(shortChunks))
	}
}

func TestCosineSimilarity(t *testing.T) {
	v1 := []float32{1.0, 0.0, 0.0}
	v2 := []float32{1.0, 0.0, 0.0}
	v3 := []float32{0.0, 1.0, 0.0}

	// Exact match should have similarity ~1.0
	simIdentical := CosineSimilarity(v1, v2)
	if simIdentical < 0.99 {
		t.Errorf("expected similarity near 1.0 for identical vectors, got %f", simIdentical)
	}

	// Orthogonal vectors should have similarity 0.0
	simOrthogonal := CosineSimilarity(v1, v3)
	if simOrthogonal > 0.01 {
		t.Errorf("expected similarity near 0.0 for orthogonal vectors, got %f", simOrthogonal)
	}

	// Empty vectors should return 0
	if CosineSimilarity(nil, nil) != 0 {
		t.Errorf("expected 0 for nil vectors")
	}
}

func TestRetrieveRelevantDropsUnrelatedAndUnembeddedChunks(t *testing.T) {
	repository := retrievalChunkRepositoryStub{results: []models.DocumentChunk{
		{Content: "relevant", Embedding: []float32{1, 0}},
		{Content: "unrelated", Embedding: []float32{0, 1}},
		{Content: "missing embedding"},
	}}
	service := NewRAGService(repository, embeddingGeneratorStub{})

	result, err := service.RetrieveRelevant(context.Background(), "client_one", "question", 4)
	if err != nil {
		t.Fatalf("retrieve knowledge: %v", err)
	}
	if strings.TrimSpace(result) != "relevant" {
		t.Fatalf("unexpected retrieved knowledge %q", result)
	}
}
