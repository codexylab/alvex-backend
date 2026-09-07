package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"github.com/codexylab/alvex-backend/pkg/database"
	"github.com/codexylab/alvex-backend/pkg/models"
	"github.com/codexylab/alvex-backend/pkg/tenant"
)

var ErrDocumentAlreadyQueued = errors.New("document is already queued or processing")

// DocumentRepository manages document metadata records.
type DocumentRepository interface {
	InsertDocument(ctx context.Context, doc *models.Document) error
	CreateDocumentWithContent(ctx context.Context, doc *models.Document, content string) error
	GetDocumentContent(ctx context.Context, id, clientID string) (DocumentContent, error)
	GetDocumentsByClient(ctx context.Context, clientID string) ([]models.Document, error)
	DeleteDocument(ctx context.Context, id, clientID string) error
	ClientExists(ctx context.Context, clientID string) (bool, error)
	UpdateDocumentStatus(ctx context.Context, id, clientID, status, errorMessage string) error
	QueueDocumentReindex(ctx context.Context, id, clientID string) error
}

type DocumentContent struct {
	DocumentID string
	ClientID   string
	Filename   string
	Content    string
}

// SQLDocumentRepository implements DocumentRepository.
type SQLDocumentRepository struct {
	DB                  *database.DB
	requireOrganization bool
}

// NewSQLDocumentRepository creates a SQLDocumentRepository instance.
func NewSQLDocumentRepository(db *database.DB) *SQLDocumentRepository {
	return &SQLDocumentRepository{DB: db}
}

func NewTenantSQLDocumentRepository(db *database.DB) *SQLDocumentRepository {
	return &SQLDocumentRepository{DB: db, requireOrganization: true}
}

func (r *SQLDocumentRepository) organizationID(ctx context.Context) (string, error) {
	if !r.requireOrganization {
		return "", nil
	}
	return tenant.RequireOrganizationID(ctx)
}

// InsertDocument creates a new document record.
func (r *SQLDocumentRepository) InsertDocument(ctx context.Context, doc *models.Document) error {
	if doc.CreatedAt.IsZero() {
		doc.CreatedAt = time.Now()
	}
	if doc.UpdatedAt.IsZero() {
		doc.UpdatedAt = doc.CreatedAt
	}

	organizationID, err := r.organizationID(ctx)
	if err != nil {
		return err
	}
	query := `INSERT INTO documents
		(id, client_id, filename, file_type, file_size, status, error_message, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	args := []interface{}{doc.ID, doc.ClientID, doc.Filename, doc.FileType, doc.FileSize, doc.Status, doc.ErrorMessage, doc.CreatedAt, doc.UpdatedAt}
	if r.requireOrganization {
		query = `INSERT INTO documents
			(id, client_id, filename, file_type, file_size, status, error_message, created_at, updated_at)
			SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9
			WHERE EXISTS (SELECT 1 FROM clients WHERE id = $2 AND organization_id = $10)`
		args = append(args, organizationID)
	}
	result, err := r.DB.ExecContext(
		ctx,
		r.DB.Adapt(query),
		args...,
	)
	if err == nil && r.requireOrganization {
		if affected, rowsErr := result.RowsAffected(); rowsErr != nil || affected == 0 {
			return sql.ErrNoRows
		}
	}
	return err
}

// CreateDocumentWithContent atomically persists both upload metadata and its
// extracted text so a background worker never observes a partial upload.
func (r *SQLDocumentRepository) CreateDocumentWithContent(
	ctx context.Context,
	doc *models.Document,
	content string,
) error {
	if doc.CreatedAt.IsZero() {
		doc.CreatedAt = time.Now().UTC()
	}
	if doc.UpdatedAt.IsZero() {
		doc.UpdatedAt = doc.CreatedAt
	}
	organizationID, err := r.organizationID(ctx)
	if err != nil {
		return err
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	query := `INSERT INTO documents
		(id, client_id, filename, file_type, file_size, status, error_message, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	args := []interface{}{
		doc.ID, doc.ClientID, doc.Filename, doc.FileType, doc.FileSize,
		doc.Status, doc.ErrorMessage, doc.CreatedAt, doc.UpdatedAt,
	}
	if r.requireOrganization {
		query = `INSERT INTO documents
			(id, client_id, filename, file_type, file_size, status, error_message, created_at, updated_at)
			SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9
			WHERE EXISTS (SELECT 1 FROM clients WHERE id = $2 AND organization_id = $10)`
		args = append(args, organizationID)
	}
	result, err := tx.ExecContext(ctx, r.DB.Adapt(query), args...)
	if err != nil {
		return err
	}
	if affected, rowsErr := result.RowsAffected(); rowsErr != nil || affected != 1 {
		return sql.ErrNoRows
	}

	digest := sha256.Sum256([]byte(content))
	if _, err = tx.ExecContext(ctx, r.DB.Adapt(`
		INSERT INTO document_contents
		  (document_id, client_id, content, content_sha256, created_at)
		VALUES ($1, $2, $3, $4, $5)`),
		doc.ID,
		doc.ClientID,
		content,
		hex.EncodeToString(digest[:]),
		doc.CreatedAt,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// GetDocumentContent scopes source retrieval to both the document and client.
func (r *SQLDocumentRepository) GetDocumentContent(
	ctx context.Context,
	id string,
	clientID string,
) (DocumentContent, error) {
	organizationID, err := r.organizationID(ctx)
	if err != nil {
		return DocumentContent{}, err
	}
	query := `
		SELECT d.id, d.client_id, d.filename, dc.content
		FROM documents d
		JOIN document_contents dc ON dc.document_id = d.id AND dc.client_id = d.client_id
		WHERE d.id = $1 AND d.client_id = $2`
	args := []interface{}{id, clientID}
	if r.requireOrganization {
		query += ` AND EXISTS (
			SELECT 1 FROM clients c WHERE c.id = d.client_id AND c.organization_id = $3)`
		args = append(args, organizationID)
	}
	var result DocumentContent
	err = r.DB.QueryRowContext(ctx, r.DB.Adapt(query), args...).Scan(
		&result.DocumentID,
		&result.ClientID,
		&result.Filename,
		&result.Content,
	)
	return result, err
}

// GetDocumentsByClient returns all uploaded documents for a client.
func (r *SQLDocumentRepository) GetDocumentsByClient(ctx context.Context, clientID string) ([]models.Document, error) {
	organizationID, err := r.organizationID(ctx)
	if err != nil {
		return nil, err
	}
	query := `
		SELECT d.id, d.client_id, d.filename, d.file_type, d.file_size, d.status,
		       COALESCE(d.error_message,''), d.created_at, d.updated_at
		FROM documents d
		WHERE d.client_id = $1
	`
	args := []interface{}{clientID}
	if r.requireOrganization {
		query += ` AND EXISTS (SELECT 1 FROM clients c WHERE c.id = d.client_id AND c.organization_id = $2)`
		args = append(args, organizationID)
	}
	query += ` ORDER BY d.created_at DESC`
	rows, err := r.DB.QueryContext(ctx, r.DB.Adapt(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []models.Document
	for rows.Next() {
		var d models.Document
		if err := rows.Scan(&d.ID, &d.ClientID, &d.Filename, &d.FileType, &d.FileSize, &d.Status, &d.ErrorMessage, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		docs = append(docs, d)
	}
	return docs, nil
}

// DeleteDocument removes a document metadata record.
func (r *SQLDocumentRepository) DeleteDocument(ctx context.Context, id, clientID string) error {
	organizationID, err := r.organizationID(ctx)
	if err != nil {
		return err
	}
	query := `DELETE FROM documents WHERE id = $1 AND client_id = $2`
	args := []interface{}{id, clientID}
	if r.requireOrganization {
		query += ` AND EXISTS (SELECT 1 FROM clients c WHERE c.id = documents.client_id AND c.organization_id = $3)`
		args = append(args, organizationID)
	}
	_, err = r.DB.ExecContext(ctx, r.DB.Adapt(query), args...)
	return err
}

func (r *SQLDocumentRepository) ClientExists(ctx context.Context, clientID string) (bool, error) {
	organizationID, err := r.organizationID(ctx)
	if err != nil {
		return false, err
	}
	query := `SELECT EXISTS(SELECT 1 FROM clients WHERE id = $1`
	args := []interface{}{clientID}
	if r.requireOrganization {
		query += ` AND organization_id = $2`
		args = append(args, organizationID)
	}
	query += `)`
	var exists bool
	err = r.DB.QueryRowContext(ctx, r.DB.Adapt(query), args...).Scan(&exists)
	return exists, err
}

func (r *SQLDocumentRepository) UpdateDocumentStatus(ctx context.Context, id, clientID, status, errorMessage string) error {
	organizationID, err := r.organizationID(ctx)
	if err != nil {
		return err
	}
	query := `UPDATE documents SET status = $1, error_message = $2, updated_at = $3 WHERE id = $4 AND client_id = $5`
	args := []interface{}{status, errorMessage, time.Now().UTC(), id, clientID}
	if r.requireOrganization {
		query += ` AND EXISTS (SELECT 1 FROM clients c WHERE c.id = documents.client_id AND c.organization_id = $6)`
		args = append(args, organizationID)
	}
	_, err = r.DB.ExecContext(ctx, r.DB.Adapt(query), args...)
	return err
}

// QueueDocumentReindex uses a compare-and-set transition to prevent repeated
// user clicks from creating concurrent indexing work for one document.
func (r *SQLDocumentRepository) QueueDocumentReindex(ctx context.Context, id, clientID string) error {
	organizationID, err := r.organizationID(ctx)
	if err != nil {
		return err
	}
	query := `
		UPDATE documents SET status = 'queued', error_message = '', updated_at = $1
		WHERE id = $2 AND client_id = $3 AND status IN ('processed', 'failed')
		  AND EXISTS (SELECT 1 FROM document_contents dc
		              WHERE dc.document_id = documents.id AND dc.client_id = documents.client_id)`
	args := []interface{}{time.Now().UTC(), id, clientID}
	if r.requireOrganization {
		query += ` AND EXISTS (
			SELECT 1 FROM clients c WHERE c.id = documents.client_id AND c.organization_id = $4)`
		args = append(args, organizationID)
	}
	result, err := r.DB.ExecContext(ctx, r.DB.Adapt(query), args...)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 1 {
		return nil
	}

	var status string
	lookup := `SELECT status FROM documents WHERE id = $1 AND client_id = $2`
	lookupArgs := []interface{}{id, clientID}
	if r.requireOrganization {
		lookup += ` AND EXISTS (
			SELECT 1 FROM clients c WHERE c.id = documents.client_id AND c.organization_id = $3)`
		lookupArgs = append(lookupArgs, organizationID)
	}
	if err := r.DB.QueryRowContext(ctx, r.DB.Adapt(lookup), lookupArgs...).Scan(&status); err != nil {
		return err
	}
	return ErrDocumentAlreadyQueued
}
