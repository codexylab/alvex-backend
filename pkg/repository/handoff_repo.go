package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/codexylab/alvex-backend/pkg/database"
	"github.com/codexylab/alvex-backend/pkg/models"
	"github.com/codexylab/alvex-backend/pkg/tenant"
)

// HandoffRepository persists tenant-scoped human-intervention workflows.
type HandoffRepository interface {
	ListNeedsAttention(ctx context.Context, limit int) ([]models.ActivityLog, error)
	Reply(ctx context.Context, id, reply string, repliedAt time.Time) (int64, error)
	Resolve(ctx context.Context, id string) (int64, error)
}

type SQLHandoffRepository struct {
	DB *database.DB
}

func NewSQLHandoffRepository(db *database.DB) *SQLHandoffRepository {
	return &SQLHandoffRepository{DB: db}
}

func (r *SQLHandoffRepository) ListNeedsAttention(ctx context.Context, limit int) ([]models.ActivityLog, error) {
	organizationID, err := tenant.RequireOrganizationID(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := r.DB.QueryContext(ctx, r.DB.Adapt(`
		SELECT a.id, a.client_id, a.client_name, a.channel, a.user_ref, a.session_id, a.message,
		       COALESCE(a.ai_response,''), a.status, a.latency_ms, COALESCE(a.needs_human,false),
		       COALESCE(a.human_reply,''), a.replied_at, COALESCE(a.handoff_reason,''), a.created_at
		FROM activity_logs a
		JOIN clients c ON c.id = a.client_id
		WHERE c.organization_id = $1
		  AND (a.needs_human = true OR a.status = 'Needs Human')
		  AND a.status != 'Resolved'
		ORDER BY a.created_at DESC
		LIMIT $2`), organizationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logs := make([]models.ActivityLog, 0)
	for rows.Next() {
		var log models.ActivityLog
		var clientID, aiResponse, humanReply, handoffReason sql.NullString
		var repliedAt sql.NullTime
		var needsHuman interface{}
		if err := rows.Scan(
			&log.ID, &clientID, &log.ClientName, &log.Channel, &log.UserRef, &log.SessionID, &log.Message,
			&aiResponse, &log.Status, &log.LatencyMs, &needsHuman,
			&humanReply, &repliedAt, &handoffReason, &log.CreatedAt,
		); err != nil {
			return nil, err
		}
		if clientID.Valid {
			log.ClientID = &clientID.String
		}
		if aiResponse.Valid {
			log.AIResponse = aiResponse.String
		}
		if humanReply.Valid {
			log.HumanReply = humanReply.String
		}
		if repliedAt.Valid {
			log.RepliedAt = &repliedAt.Time
		}
		if handoffReason.Valid {
			log.HandoffReason = handoffReason.String
		}
		log.NeedsHuman = parseBoolValue(needsHuman)
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

func (r *SQLHandoffRepository) Reply(ctx context.Context, id, reply string, repliedAt time.Time) (int64, error) {
	organizationID, err := tenant.RequireOrganizationID(ctx)
	if err != nil {
		return 0, err
	}
	result, err := r.DB.ExecContext(ctx, r.DB.Adapt(`
		UPDATE activity_logs
		SET human_reply = $1, replied_at = $2, status = 'Resolved', needs_human = false
		WHERE id = $3
		  AND EXISTS (SELECT 1 FROM clients c WHERE c.id = activity_logs.client_id AND c.organization_id = $4)`),
		reply, repliedAt, id, organizationID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *SQLHandoffRepository) Resolve(ctx context.Context, id string) (int64, error) {
	organizationID, err := tenant.RequireOrganizationID(ctx)
	if err != nil {
		return 0, err
	}
	result, err := r.DB.ExecContext(ctx, r.DB.Adapt(`
		UPDATE activity_logs SET status = 'Resolved', needs_human = false
		WHERE id = $1
		  AND EXISTS (SELECT 1 FROM clients c WHERE c.id = activity_logs.client_id AND c.organization_id = $2)`),
		id, organizationID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
