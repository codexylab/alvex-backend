package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/codexylab/alvex-backend/pkg/models"
	"github.com/codexylab/alvex-backend/pkg/repository"
)

type documentRepositoryStub struct {
	exists        bool
	created       *models.Document
	content       string
	deleted       bool
	stored        repository.DocumentContent
	statusUpdates []string
	reindexCalls  int
}

func (s *documentRepositoryStub) InsertDocument(context.Context, *models.Document) error { return nil }
func (s *documentRepositoryStub) CreateDocumentWithContent(_ context.Context, doc *models.Document, content string) error {
	copy := *doc
	s.created = &copy
	s.content = content
	return nil
}
func (s *documentRepositoryStub) GetDocumentContent(context.Context, string, string) (repository.DocumentContent, error) {
	return s.stored, nil
}
func (s *documentRepositoryStub) GetDocumentsByClient(context.Context, string) ([]models.Document, error) {
	return nil, nil
}
func (s *documentRepositoryStub) DeleteDocument(context.Context, string, string) error {
	s.deleted = true
	return nil
}
func (s *documentRepositoryStub) ClientExists(context.Context, string) (bool, error) {
	return s.exists, nil
}
func (s *documentRepositoryStub) UpdateDocumentStatus(_ context.Context, _, _, status, _ string) error {
	s.statusUpdates = append(s.statusUpdates, status)
	return nil
}
func (s *documentRepositoryStub) QueueDocumentReindex(context.Context, string, string) error {
	s.reindexCalls++
	return nil
}

type chunkRepositorySpy struct {
	replacedDocumentID string
	replaced           []models.DocumentChunk
}

func (s *chunkRepositorySpy) InsertChunk(context.Context, *models.DocumentChunk) error { return nil }
func (s *chunkRepositorySpy) ReplaceSourceChunks(_ context.Context, _, documentID, _ string, chunks []models.DocumentChunk) error {
	s.replacedDocumentID = documentID
	s.replaced = append([]models.DocumentChunk(nil), chunks...)
	return nil
}
func (s *chunkRepositorySpy) GetChunksByClient(context.Context, string) ([]models.DocumentChunk, error) {
	return nil, nil
}
func (s *chunkRepositorySpy) DeleteClientChunks(context.Context, string) error { return nil }
func (s *chunkRepositorySpy) SearchSimilar(context.Context, string, []float32, int) ([]models.DocumentChunk, error) {
	return nil, nil
}

type embeddingGeneratorStub struct {
	err error
}

func (s embeddingGeneratorStub) GenerateEmbedding(context.Context, string) ([]float32, error) {
	if s.err != nil {
		return nil, s.err
	}
	return []float32{1, 0}, nil
}

func TestDocumentUploadPersistsSourceAndQueuesOnlyIdentifiers(t *testing.T) {
	documents := &documentRepositoryStub{exists: true}
	jobs := &whatsAppJobRepositoryStub{}
	service := NewDocumentService(documents, jobs)

	document, err := service.ProcessUpload(
		context.Background(),
		"client_one",
		"../knowledge.md",
		9999,
		strings.NewReader("durable source text"),
	)
	if err != nil {
		t.Fatalf("queue upload: %v", err)
	}
	if document.Status != "queued" || document.Filename != "knowledge.md" || document.FileSize != 19 {
		t.Fatalf("unexpected queued document: %#v", document)
	}
	if documents.content != "durable source text" {
		t.Fatalf("source content was not stored, got %q", documents.content)
	}
	queued := string(jobs.jobs[document.ID])
	if !strings.Contains(queued, document.ID) || strings.Contains(queued, "durable source text") {
		t.Fatalf("job must contain identifiers only, got %s", queued)
	}
}

func TestDocumentUploadRejectsUnsupportedBinaryType(t *testing.T) {
	documents := &documentRepositoryStub{exists: true}
	service := NewDocumentService(documents, &whatsAppJobRepositoryStub{})

	if _, err := service.ProcessUpload(
		context.Background(), "client_one", "unsafe.pdf", 4, strings.NewReader("data"),
	); err == nil {
		t.Fatal("expected unsupported binary type to be rejected")
	}
	if documents.created != nil {
		t.Fatal("rejected upload mutated document storage")
	}
}

func TestDocumentUploadRejectsBinaryContentWithTextExtension(t *testing.T) {
	documents := &documentRepositoryStub{exists: true}
	service := NewDocumentService(documents, &whatsAppJobRepositoryStub{})

	if _, err := service.ProcessUpload(
		context.Background(), "client_one", "unsafe.txt", 4, strings.NewReader("a\x00b"),
	); err == nil {
		t.Fatal("expected binary content to be rejected")
	}
	if documents.created != nil {
		t.Fatal("rejected upload mutated document storage")
	}
}

func TestDocumentUploadRejectsInvalidJSON(t *testing.T) {
	documents := &documentRepositoryStub{exists: true}
	service := NewDocumentService(documents, &whatsAppJobRepositoryStub{})

	if _, err := service.ProcessUpload(
		context.Background(), "client_one", "knowledge.json", 10, strings.NewReader(`{"broken":`),
	); err == nil {
		t.Fatal("expected invalid JSON to be rejected")
	}
}

func TestManualKnowledgeUsesDocumentQueuePipeline(t *testing.T) {
	documents := &documentRepositoryStub{exists: true}
	jobs := &whatsAppJobRepositoryStub{}
	service := NewDocumentService(documents, jobs)

	document, err := service.ProcessText(
		context.Background(),
		"client_one",
		"Returns & Refunds",
		"Customers can request a refund within thirty days.",
	)
	if err != nil {
		t.Fatalf("queue manual knowledge: %v", err)
	}
	if document.Filename != "Returns-Refunds.txt" || document.Status != "queued" {
		t.Fatalf("unexpected manual document: %#v", document)
	}
	if len(jobs.jobs) != 1 || documents.content == "" {
		t.Fatalf("manual knowledge did not use durable pipeline: jobs=%d", len(jobs.jobs))
	}
}

func TestDocumentIndexProcessorMarksRetryableFailureAndThenProcesses(t *testing.T) {
	documents := &documentRepositoryStub{stored: repository.DocumentContent{
		DocumentID: "document_one",
		ClientID:   "client_one",
		Filename:   "knowledge.txt",
		Content:    "knowledge content",
	}}
	chunks := &chunkRepositorySpy{}
	job := repository.BackgroundJob{Payload: []byte(`{"document_id":"document_one","client_id":"client_one"}`)}

	failing := &DocumentIndexJobProcessor{
		Documents: documents,
		RAG:       NewRAGService(chunks, embeddingGeneratorStub{err: errors.New("provider unavailable")}),
	}
	if err := failing.Process(context.Background(), job); err == nil {
		t.Fatal("expected provider failure to be retryable")
	}
	if got := strings.Join(documents.statusUpdates, ","); got != "processing,failed" {
		t.Fatalf("unexpected failure states %q", got)
	}
	if len(chunks.replaced) != 0 {
		t.Fatal("failed embeddings must not replace searchable chunks")
	}

	documents.statusUpdates = nil
	successful := &DocumentIndexJobProcessor{
		Documents: documents,
		RAG:       NewRAGService(chunks, embeddingGeneratorStub{}),
	}
	if err := successful.Process(context.Background(), job); err != nil {
		t.Fatalf("process retry: %v", err)
	}
	if got := strings.Join(documents.statusUpdates, ","); got != "processing,processed" {
		t.Fatalf("unexpected success states %q", got)
	}
	if chunks.replacedDocumentID != "document_one" || len(chunks.replaced) != 1 {
		t.Fatalf("unexpected indexed chunks: document=%q chunks=%d", chunks.replacedDocumentID, len(chunks.replaced))
	}
}

func TestDocumentReindexUsesANewJobGeneration(t *testing.T) {
	documents := &documentRepositoryStub{stored: repository.DocumentContent{
		DocumentID: "document_one",
		ClientID:   "client_one",
		Content:    "stored source",
	}}
	jobs := &websiteJobRepositorySpy{}
	service := NewDocumentService(documents, jobs)

	jobID, err := service.ReindexDocument(context.Background(), "document_one", "client_one")
	if err != nil {
		t.Fatalf("queue re-index: %v", err)
	}
	if documents.reindexCalls != 1 || jobs.jobType != DocumentIndexJobType || jobs.idempotencyKey != jobID {
		t.Fatalf(
			"unexpected re-index job: transitions=%d type=%q key=%q id=%q",
			documents.reindexCalls,
			jobs.jobType,
			jobs.idempotencyKey,
			jobID,
		)
	}
	if !strings.HasPrefix(jobID, "document_one:reindex:") {
		t.Fatalf("expected versioned re-index key, got %q", jobID)
	}
}
