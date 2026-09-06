package services

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/codexylab/alvex-backend/pkg/repository"
)

const DocumentIndexJobType = "knowledge.document_index"

type DocumentIndexJobPayload struct {
	DocumentID string `json:"document_id"`
	ClientID   string `json:"client_id"`
}

// DocumentIndexJobProcessor owns retry-safe document state transitions and
// delegates atomic chunk replacement to the RAG service.
type DocumentIndexJobProcessor struct {
	Documents repository.DocumentRepository
	RAG       *RAGService
}

func (p *DocumentIndexJobProcessor) Process(ctx context.Context, job repository.BackgroundJob) error {
	var payload DocumentIndexJobPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("decode document index job: %w", err)
	}
	if payload.DocumentID == "" || payload.ClientID == "" {
		return fmt.Errorf("document index job is missing required identifiers")
	}
	if p.Documents == nil || p.RAG == nil {
		return fmt.Errorf("document index processor is not configured")
	}

	document, err := p.Documents.GetDocumentContent(ctx, payload.DocumentID, payload.ClientID)
	if err != nil {
		return fmt.Errorf("load document source: %w", err)
	}
	if err := p.Documents.UpdateDocumentStatus(ctx, payload.DocumentID, payload.ClientID, "processing", ""); err != nil {
		return fmt.Errorf("mark document processing: %w", err)
	}
	if err := p.RAG.IndexDocument(
		ctx,
		payload.ClientID,
		payload.DocumentID,
		document.Filename,
		document.Content,
	); err != nil {
		_ = p.Documents.UpdateDocumentStatus(
			ctx,
			payload.DocumentID,
			payload.ClientID,
			"failed",
			"knowledge indexing failed; retry scheduled",
		)
		return fmt.Errorf("index document: %w", err)
	}
	if err := p.Documents.UpdateDocumentStatus(ctx, payload.DocumentID, payload.ClientID, "processed", ""); err != nil {
		return fmt.Errorf("mark document processed: %w", err)
	}
	return nil
}
