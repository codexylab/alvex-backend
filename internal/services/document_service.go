package services

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/codexylab/alvex-backend/internal/models"
	"github.com/codexylab/alvex-backend/internal/repository"
)

// DocumentService processes file uploads and indexes them into the vector knowledge base.
type DocumentService struct {
	DocRepo    repository.DocumentRepository
	RAGService *RAGService
}

// NewDocumentService creates a new DocumentService instance.
func NewDocumentService(docRepo repository.DocumentRepository, ragService *RAGService) *DocumentService {
	return &DocumentService{
		DocRepo:    docRepo,
		RAGService: ragService,
	}
}

// ProcessUpload handles raw file data, extracts text, indexes into RAG, and saves document record.
func (s *DocumentService) ProcessUpload(ctx context.Context, clientID, filename string, fileSize int64, reader io.Reader) (*models.Document, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, reader); err != nil {
		return nil, fmt.Errorf("failed to read uploaded file: %w", err)
	}

	content := buf.String()
	docID := uuid.New().String()

	doc := &models.Document{
		ID:        docID,
		ClientID:  clientID,
		Filename:  filename,
		FileType:  ext,
		FileSize:  fileSize,
		Status:    "processed",
		CreatedAt: time.Now(),
	}

	// 1. Index into RAG vector repository
	if s.RAGService != nil && strings.TrimSpace(content) != "" {
		if err := s.RAGService.IndexContent(ctx, clientID, filename, content); err != nil {
			slog.Error("failed to index uploaded document into RAG", "filename", filename, "error", err)
			doc.Status = "partially_indexed"
		}
	}

	// 2. Save document metadata record
	if err := s.DocRepo.InsertDocument(ctx, doc); err != nil {
		return nil, fmt.Errorf("failed to save document record: %w", err)
	}

	return doc, nil
}

// ListDocuments returns all uploaded knowledge documents for a client.
func (s *DocumentService) ListDocuments(ctx context.Context, clientID string) ([]models.Document, error) {
	return s.DocRepo.GetDocumentsByClient(ctx, clientID)
}

// DeleteDocument removes a document metadata record.
func (s *DocumentService) DeleteDocument(ctx context.Context, id, clientID string) error {
	return s.DocRepo.DeleteDocument(ctx, id, clientID)
}
