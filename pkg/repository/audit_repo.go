package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/codexylab/alvex-backend/pkg/database"
)

// AuditEntry is an immutable record of a tenant mutation.
type AuditEntry struct {
	ID             string
	OrganizationID string
	ActorUserID    string
	Action         string
	ResourceType   string
	ResourceID     string
	Metadata       map[string]interface{}
	IPAddress      string
	CreatedAt      time.Time
}

type AuditRepository interface {
	InsertAuditEntry(ctx context.Context, entry AuditEntry) error
}

type SQLAuditRepository struct {
	DB *database.DB
}

func NewSQLAuditRepository(db *database.DB) *SQLAuditRepository {
	return &SQLAuditRepository{DB: db}
}

func (r *SQLAuditRepository) InsertAuditEntry(ctx context.Context, entry AuditEntry) error {
	metadata, err := json.Marshal(entry.Metadata)
	if err != nil {
		return err
	}
	_, err = r.DB.ExecContext(ctx, r.DB.Adapt(`
		INSERT INTO audit_logs
		  (id, organization_id, actor_user_id, action, resource_type, resource_id, metadata, ip_address, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`),
		entry.ID, entry.OrganizationID, entry.ActorUserID, entry.Action,
		entry.ResourceType, entry.ResourceID, string(metadata), entry.IPAddress, entry.CreatedAt,
	)
	return err
}
