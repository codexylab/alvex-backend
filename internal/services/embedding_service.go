package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

// EmbeddingService handles text embedding generation and vector operations.
type EmbeddingService struct {
	GeminiAPIKey string
	httpClient   *http.Client
}

// NewEmbeddingService creates a new EmbeddingService using Google Gemini's embedding API.
func NewEmbeddingService(geminiKey string) *EmbeddingService {
	return &EmbeddingService{
		GeminiAPIKey: geminiKey,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

// GenerateEmbedding generates a vector embedding for the input text using Gemini text-embedding-004.
// If GeminiAPIKey is empty, returns nil, nil for graceful degradation.
func (e *EmbeddingService) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	if e.GeminiAPIKey == "" || strings.TrimSpace(text) == "" {
		return nil, nil
	}

	url := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/text-embedding-004:embedContent?key=%s",
		e.GeminiAPIKey,
	)

	payload := map[string]interface{}{
		"model": "models/text-embedding-004",
		"content": map[string]interface{}{
			"parts": []map[string]string{{"text": text}},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal embedding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create embedding request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read embedding response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding API error (status %d): %s", resp.StatusCode, string(raw))
	}

	var result struct {
		Embedding struct {
			Values []float32 `json:"values"`
		} `json:"embedding"`
	}

	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal embedding response: %w", err)
	}

	return result.Embedding.Values, nil
}

// ChunkText splits raw text into overlapping word chunks.
// chunkSize: target words per chunk (default 150)
// overlap: overlapping words between consecutive chunks (default 25)
func ChunkText(text string, chunkSize, overlap int) []string {
	if chunkSize <= 0 {
		chunkSize = 150
	}
	if overlap < 0 || overlap >= chunkSize {
		overlap = 25
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	// If text is smaller than chunkSize, return single chunk
	if len(words) <= chunkSize {
		return []string{strings.Join(words, " ")}
	}

	var chunks []string
	step := chunkSize - overlap
	if step <= 0 {
		step = 1
	}

	for i := 0; i < len(words); i += step {
		end := i + chunkSize
		if end > len(words) {
			end = len(words)
		}

		chunkWords := words[i:end]
		if len(chunkWords) > 0 {
			chunks = append(chunks, strings.Join(chunkWords, " "))
		}

		if end >= len(words) {
			break
		}
	}

	return chunks
}

// CosineSimilarity computes the cosine similarity between two float32 vectors.
// Returns a value between -1.0 and 1.0 (higher means more similar).
func CosineSimilarity(a, b []float32) float32 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := 0; i < len(a); i++ {
		valA := float64(a[i])
		valB := float64(b[i])
		dotProduct += valA * valB
		normA += valA * valA
		normB += valB * valB
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return float32(dotProduct / (math.Sqrt(normA) * math.Sqrt(normB)))
}
