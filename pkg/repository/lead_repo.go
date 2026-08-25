package repository

import (
	"context"
	"time"

	"github.com/codexylab/alvex-backend/pkg/database"
	"github.com/codexylab/alvex-backend/pkg/models"
)

// LeadRepository manages CRM leads captured from the widget.
type LeadRepository interface {
	InsertLead(ctx context.Context, lead *models.Lead) error
	GetLeadsByClient(ctx context.Context, clientID string, limit, offset int) ([]models.Lead, int, error)
}

// SQLLeadRepository implements LeadRepository.
type SQLLeadRepository struct {
	DB *database.DB
}

// NewSQLLeadRepository creates a new SQLLeadRepository instance.
func NewSQLLeadRepository(db *database.DB) *SQLLeadRepository {
	return &SQLLeadRepository{DB: db}
}

// InsertLead inserts a newly captured lead record.
func (r *SQLLeadRepository) InsertLead(ctx context.Context, lead *models.Lead) error {
	if lead.CreatedAt.IsZero() {
		lead.CreatedAt = time.Now()
	}

	query := `
		INSERT INTO leads (id, client_id, name, email, phone, session_id, source, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := r.DB.ExecContext(
		ctx,
		r.DB.Adapt(query),
		lead.ID,
		lead.ClientID,
		lead.Name,
		lead.Email,
		lead.Phone,
		lead.SessionID,
		lead.Source,
		lead.CreatedAt,
	)
	return err
}

// GetLeadsByClient lists leads for a specific client with pagination.
func (r *SQLLeadRepository) GetLeadsByClient(ctx context.Context, clientID string, limit, offset int) ([]models.Lead, int, error) {
	var total int
	countQuery := `SELECT COUNT(*) FROM leads WHERE client_id = $1`
	if err := r.DB.QueryRowContext(ctx, r.DB.Adapt(countQuery), clientID).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT id, client_id, name, COALESCE(email,''), COALESCE(phone,''), COALESCE(session_id,''), source, created_at
		FROM leads
		WHERE client_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.DB.QueryContext(ctx, r.DB.Adapt(query), clientID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var leads []models.Lead
	for rows.Next() {
		var l models.Lead
		if err := rows.Scan(&l.ID, &l.ClientID, &l.Name, &l.Email, &l.Phone, &l.SessionID, &l.Source, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		leads = append(leads, l)
	}

	return leads, total, nil
}
