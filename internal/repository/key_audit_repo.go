package repository

import (
	"context"
	"time"

	"github.com/codexylab/alvex-backend/internal/database"
	"github.com/codexylab/alvex-backend/internal/models"
)

// KeyAuditRepository manages audit logs for security credential rotations.
type KeyAuditRepository interface {
	InsertAuditLog(ctx context.Context, log *models.KeyAuditLog) error
	GetAuditLogsByClient(ctx context.Context, clientID string) ([]models.KeyAuditLog, error)
}

// SQLKeyAuditRepository implements KeyAuditRepository.
type SQLKeyAuditRepository struct {
	DB *database.DB
}

// NewSQLKeyAuditRepository creates a new SQLKeyAuditRepository.
func NewSQLKeyAuditRepository(db *database.DB) *SQLKeyAuditRepository {
	return &SQLKeyAuditRepository{DB: db}
}

// InsertAuditLog logs a key rotation event.
func (r *SQLKeyAuditRepository) InsertAuditLog(ctx context.Context, log *models.KeyAuditLog) error {
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now()
	}

	query := `
		INSERT INTO key_audit_logs (id, client_id, key_type, rotated_by, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err := r.DB.ExecContext(ctx, r.DB.Adapt(query), log.ID, log.ClientID, log.KeyType, log.RotatedBy, log.CreatedAt)
	return err
}

// GetAuditLogsByClient lists rotation history for a client.
func (r *SQLKeyAuditRepository) GetAuditLogsByClient(ctx context.Context, clientID string) ([]models.KeyAuditLog, error) {
	query := `
		SELECT id, client_id, key_type, rotated_by, created_at
		FROM key_audit_logs
		WHERE client_id = $1
		ORDER BY created_at DESC
		LIMIT 50
	`
	rows, err := r.DB.QueryContext(ctx, r.DB.Adapt(query), clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.KeyAuditLog
	for rows.Next() {
		var l models.KeyAuditLog
		if err := rows.Scan(&l.ID, &l.ClientID, &l.KeyType, &l.RotatedBy, &l.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, nil
}
