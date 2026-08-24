package services

import (
	"testing"
)

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
