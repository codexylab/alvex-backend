package repository

import (
	"context"
	"time"

	"github.com/codexylab/alvex-backend/internal/database"
	"github.com/codexylab/alvex-backend/internal/models"
)

// DocumentRepository manages document metadata records.
type DocumentRepository interface {
	InsertDocument(ctx context.Context, doc *models.Document) error
	GetDocumentsByClient(ctx context.Context, clientID string) ([]models.Document, error)
	DeleteDocument(ctx context.Context, id, clientID string) error
}

// SQLDocumentRepository implements DocumentRepository.
type SQLDocumentRepository struct {
	DB *database.DB
}

// NewSQLDocumentRepository creates a SQLDocumentRepository instance.
func NewSQLDocumentRepository(db *database.DB) *SQLDocumentRepository {
	return &SQLDocumentRepository{DB: db}
}

// InsertDocument creates a new document record.
func (r *SQLDocumentRepository) InsertDocument(ctx context.Context, doc *models.Document) error {
	if doc.CreatedAt.IsZero() {
		doc.CreatedAt = time.Now()
	}

	query := `
		INSERT INTO documents (id, client_id, filename, file_type, file_size, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err := r.DB.ExecContext(
		ctx,
		r.DB.Adapt(query),
		doc.ID,
		doc.ClientID,
		doc.Filename,
		doc.FileType,
		doc.FileSize,
		doc.Status,
		doc.CreatedAt,
	)
	return err
}

// GetDocumentsByClient returns all uploaded documents for a client.
func (r *SQLDocumentRepository) GetDocumentsByClient(ctx context.Context, clientID string) ([]models.Document, error) {
	query := `
		SELECT id, client_id, filename, file_type, file_size, status, created_at
		FROM documents
		WHERE client_id = $1
		ORDER BY created_at DESC
	`
	rows, err := r.DB.QueryContext(ctx, r.DB.Adapt(query), clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []models.Document
	for rows.Next() {
		var d models.Document
		if err := rows.Scan(&d.ID, &d.ClientID, &d.Filename, &d.FileType, &d.FileSize, &d.Status, &d.CreatedAt); err != nil {
			return nil, err
		}
		docs = append(docs, d)
	}
	return docs, nil
}

// DeleteDocument removes a document metadata record.
func (r *SQLDocumentRepository) DeleteDocument(ctx context.Context, id, clientID string) error {
	query := `DELETE FROM documents WHERE id = $1 AND client_id = $2`
	_, err := r.DB.ExecContext(ctx, r.DB.Adapt(query), id, clientID)
	return err
}
