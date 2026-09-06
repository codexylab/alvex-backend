package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/codexylab/alvex-backend/pkg/models"
	"github.com/codexylab/alvex-backend/pkg/repository"
)

// DocumentService processes file uploads and indexes them into the vector knowledge base.
type DocumentService struct {
	DocRepo repository.DocumentRepository
	Jobs    repository.BackgroundJobRepository
}

// NewDocumentService creates a new DocumentService instance.
func NewDocumentService(
	docRepo repository.DocumentRepository,
	jobs repository.BackgroundJobRepository,
) *DocumentService {
	return &DocumentService{
		DocRepo: docRepo,
		Jobs:    jobs,
	}
}

// ProcessUpload validates and durably queues extracted text for RAG indexing.
func (s *DocumentService) ProcessUpload(ctx context.Context, clientID, filename string, declaredSize int64, reader io.Reader) (*models.Document, error) {
	filename = filepath.Base(strings.TrimSpace(filename))
	ext := strings.ToLower(filepath.Ext(filename))
	if !isSupportedKnowledgeFile(ext) {
		return nil, fmt.Errorf("unsupported file type; allowed types are .txt, .md, .csv, and .json")
	}
	if declaredSize > maxKnowledgeDocumentBytes {
		return nil, fmt.Errorf("file exceeds the 15 MiB limit")
	}
	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, io.LimitReader(reader, maxKnowledgeDocumentBytes+1)); err != nil {
		return nil, fmt.Errorf("failed to read uploaded file: %w", err)
	}
	if int64(buf.Len()) > maxKnowledgeDocumentBytes {
		return nil, fmt.Errorf("file exceeds the 15 MiB limit")
	}

	content := buf.String()
	if !isPlainTextSignature(buf.Bytes()) || !utf8.ValidString(content) || strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("file must contain non-empty UTF-8 text")
	}
	if ext == ".json" && !json.Valid(buf.Bytes()) {
		return nil, fmt.Errorf("JSON knowledge files must contain valid JSON")
	}
	return s.createQueuedDocument(ctx, clientID, filename, ext, content)
}

// ProcessText sends manually entered knowledge through the same durable
// document pipeline as uploaded files.
func (s *DocumentService) ProcessText(
	ctx context.Context,
	clientID string,
	title string,
	content string,
) (*models.Document, error) {
	title = strings.TrimSpace(title)
	content = strings.TrimSpace(content)
	if title == "" || len([]rune(title)) > 200 {
		return nil, fmt.Errorf("title is required and must not exceed 200 characters")
	}
	if content == "" || !utf8.ValidString(content) {
		return nil, fmt.Errorf("content must contain non-empty UTF-8 text")
	}
	if int64(len([]byte(content))) > maxManualKnowledgeBytes {
		return nil, fmt.Errorf("manual knowledge exceeds the 512 KiB limit")
	}
	filename := sanitizeKnowledgeTitle(title) + ".txt"
	return s.createQueuedDocument(ctx, clientID, filename, ".txt", content)
}

func (s *DocumentService) createQueuedDocument(
	ctx context.Context,
	clientID string,
	filename string,
	fileType string,
	content string,
) (*models.Document, error) {
	exists, err := s.DocRepo.ClientExists(ctx, clientID)
	if err != nil {
		return nil, fmt.Errorf("failed to validate client ownership: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("client not found")
	}
	docID := uuid.New().String()

	doc := &models.Document{
		ID:        docID,
		ClientID:  clientID,
		Filename:  filename,
		FileType:  fileType,
		FileSize:  int64(len([]byte(content))),
		Status:    "queued",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.DocRepo.CreateDocumentWithContent(ctx, doc, content); err != nil {
		return nil, fmt.Errorf("failed to save document record: %w", err)
	}
	if s.Jobs == nil {
		_ = s.DocRepo.DeleteDocument(ctx, doc.ID, clientID)
		return nil, fmt.Errorf("document processing queue is not configured")
	}
	payload, err := json.Marshal(DocumentIndexJobPayload{DocumentID: doc.ID, ClientID: clientID})
	if err != nil {
		_ = s.DocRepo.DeleteDocument(ctx, doc.ID, clientID)
		return nil, fmt.Errorf("encode document processing job: %w", err)
	}
	if _, err := s.Jobs.Enqueue(ctx, DocumentIndexJobType, doc.ID, payload, 6); err != nil {
		_ = s.DocRepo.DeleteDocument(ctx, doc.ID, clientID)
		return nil, fmt.Errorf("queue document processing: %w", err)
	}

	return doc, nil
}

const maxKnowledgeDocumentBytes int64 = 15 << 20
const maxManualKnowledgeBytes int64 = 512 << 10

func isSupportedKnowledgeFile(extension string) bool {
	switch extension {
	case ".txt", ".md", ".csv", ".json":
		return true
	default:
		return false
	}
}

func isPlainTextSignature(content []byte) bool {
	if bytes.IndexByte(content, 0) >= 0 {
		return false
	}
	sample := content
	if len(sample) > 512 {
		sample = sample[:512]
	}
	detectedType := http.DetectContentType(sample)
	return strings.HasPrefix(detectedType, "text/plain") || detectedType == "application/json"
}

func sanitizeKnowledgeTitle(title string) string {
	var builder strings.Builder
	pendingSeparator := false
	for _, value := range title {
		switch {
		case value >= 'a' && value <= 'z', value >= 'A' && value <= 'Z', value >= '0' && value <= '9':
			if pendingSeparator && builder.Len() > 0 {
				builder.WriteRune('-')
			}
			builder.WriteRune(value)
			pendingSeparator = false
		default:
			pendingSeparator = true
		}
	}
	result := builder.String()
	if result == "" {
		return "manual-knowledge"
	}
	return result
}

// ListDocuments returns all uploaded knowledge documents for a client.
func (s *DocumentService) ListDocuments(ctx context.Context, clientID string) ([]models.Document, error) {
	return s.DocRepo.GetDocumentsByClient(ctx, clientID)
}

// DeleteDocument removes a document metadata record.
func (s *DocumentService) DeleteDocument(ctx context.Context, id, clientID string) error {
	return s.DocRepo.DeleteDocument(ctx, id, clientID)
}

// ReindexDocument queues a new generation while preserving currently
// searchable chunks until the replacement succeeds.
func (s *DocumentService) ReindexDocument(ctx context.Context, id, clientID string) (string, error) {
	if s.Jobs == nil {
		return "", fmt.Errorf("document processing queue is not configured")
	}
	if _, err := s.DocRepo.GetDocumentContent(ctx, id, clientID); err != nil {
		return "", err
	}
	if err := s.DocRepo.QueueDocumentReindex(ctx, id, clientID); err != nil {
		return "", err
	}
	payload, err := json.Marshal(DocumentIndexJobPayload{DocumentID: id, ClientID: clientID})
	if err != nil {
		_ = s.DocRepo.UpdateDocumentStatus(ctx, id, clientID, "failed", "could not queue re-index")
		return "", fmt.Errorf("encode document re-index job: %w", err)
	}
	jobID := id + ":reindex:" + uuid.NewString()
	if _, err := s.Jobs.Enqueue(ctx, DocumentIndexJobType, jobID, payload, 6); err != nil {
		_ = s.DocRepo.UpdateDocumentStatus(ctx, id, clientID, "failed", "could not queue re-index")
		return "", fmt.Errorf("queue document re-index: %w", err)
	}
	return jobID, nil
}
