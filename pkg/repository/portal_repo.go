package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"github.com/codexylab/alvex-backend/pkg/database"
	"github.com/codexylab/alvex-backend/pkg/models"
)

// PortalStatsData represents stats query fields.
type PortalStatsData struct {
	TotalConversations int
	FailedConversations int
	AvgLatency          float64
	ThisMonthConvs      int
	PendingInvoice      float64
	ActiveSince         time.Time
}

// PortalRepository defines database operations for the client portal.
type PortalRepository interface {
	GetClientProfile(ctx context.Context, id string) (*models.Client, error)
	GetPortalStats(ctx context.Context, clientID string, isSQLite bool) (*PortalStatsData, error)
	GetConversations(ctx context.Context, clientID string, page, limit int) ([]models.ActivityLog, int, error)
	GetFAQs(ctx context.Context, clientID string) ([]models.FAQ, error)
	CreateFAQ(ctx context.Context, id, clientID, question, answer string, approved bool) error
	UpdateFAQ(ctx context.Context, id, clientID, question, answer string, approved bool) (int64, error)
	DeleteFAQ(ctx context.Context, id, clientID string) (int64, error)
	UpdatePortalConfig(ctx context.Context, query string, args ...interface{}) (int64, error)
	ReplyToConversation(ctx context.Context, id, clientID, reply string) (int64, error)
	InsertFAQsDraftsTx(ctx context.Context, clientID string, drafts []struct{ Question, Answer string }) error
	GetClientInvoices(ctx context.Context, clientID string) ([]models.Invoice, error)
	GetClientActivityLogs(ctx context.Context, clientID string) ([]models.ActivityLog, error)
}

// SQLPortalRepository implements PortalRepository.
type SQLPortalRepository struct {
	DB *database.DB
}

// NewSQLPortalRepository creates a SQLPortalRepository instance.
func NewSQLPortalRepository(db *database.DB) *SQLPortalRepository {
	return &SQLPortalRepository{DB: db}
}

// GetClientProfile retrieves the profile details.
func (r *SQLPortalRepository) GetClientProfile(ctx context.Context, id string) (*models.Client, error) {
	row := r.DB.QueryRowContext(ctx, r.DB.Adapt(`SELECT `+clientSelectCols+`,
		portal_token, owner_id,
		COALESCE(guardrails_enabled,0), COALESCE(guardrails_reply,''),
		COALESCE(chat_retention_days,30),
		created_at, updated_at
	FROM clients WHERE id = $1`), id)

	return scanClientRow(row, true, true)
}

// GetPortalStats compiles metrics for a client.
func (r *SQLPortalRepository) GetPortalStats(ctx context.Context, clientID string, isSQLite bool) (*PortalStatsData, error) {
	data := &PortalStatsData{}

	err := r.DB.QueryRowContext(ctx, r.DB.Adapt(`
		SELECT COUNT(*),
		       SUM(CASE WHEN status = 'Failed' THEN 1 ELSE 0 END),
		       COALESCE(AVG(CASE WHEN latency_ms > 0 THEN latency_ms END), 0)
		FROM activity_logs WHERE client_id = $1`), clientID,
	).Scan(&data.TotalConversations, &data.FailedConversations, &data.AvgLatency)
	if err != nil {
		return nil, err
	}

	if isSQLite {
		err = r.DB.QueryRowContext(ctx, r.DB.Adapt(`
			SELECT COUNT(*) FROM activity_logs
			WHERE client_id = $1
			  AND created_at >= date('now', 'start of month')`), clientID,
		).Scan(&data.ThisMonthConvs)
	} else {
		err = r.DB.QueryRowContext(ctx, r.DB.Adapt(`
			SELECT COUNT(*) FROM activity_logs
			WHERE client_id = $1
			  AND created_at >= DATE_TRUNC('month', NOW())`), clientID,
		).Scan(&data.ThisMonthConvs)
	}
	if err != nil {
		return nil, err
	}

	err = r.DB.QueryRowContext(ctx, r.DB.Adapt(`
		SELECT COALESCE(SUM(amount), 0)
		FROM invoices
		WHERE client_id = $1 AND status IN ('Pending', 'Overdue')`), clientID,
	).Scan(&data.PendingInvoice)
	if err != nil {
		return nil, err
	}

	err = r.DB.QueryRowContext(ctx, r.DB.Adapt(`
		SELECT created_at FROM clients WHERE id = $1`), clientID,
	).Scan(&data.ActiveSince)

	return data, err
}

// GetConversations retrieves recent transcripts.
func (r *SQLPortalRepository) GetConversations(ctx context.Context, clientID string, page, limit int) ([]models.ActivityLog, int, error) {
	offset := (page - 1) * limit
	var total int
	err := r.DB.QueryRowContext(ctx, r.DB.Adapt(`
		SELECT COUNT(*) FROM activity_logs WHERE client_id = $1`), clientID,
	).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.DB.QueryContext(ctx, r.DB.Adapt(`
		SELECT id, COALESCE(user_ref,'Visitor'), message,
		       COALESCE(ai_response,''), channel, status, latency_ms, created_at
		FROM activity_logs
		WHERE client_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`), clientID, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	entries := []models.ActivityLog{}
	for rows.Next() {
		var e models.ActivityLog
		var userRef, aiResponse sql.NullString
		var createdAt time.Time
		if err := rows.Scan(&e.ID, &userRef, &e.Message, &aiResponse, &e.Channel, &e.Status, &e.LatencyMs, &createdAt); err != nil {
			return nil, 0, err
		}
		if userRef.Valid {
			e.UserRef = userRef.String
		}
		if aiResponse.Valid {
			e.AIResponse = aiResponse.String
		}
		e.CreatedAt = createdAt
		entries = append(entries, e)
	}
	return entries, total, nil
}

// GetFAQs retrieves FAQs.
func (r *SQLPortalRepository) GetFAQs(ctx context.Context, clientID string) ([]models.FAQ, error) {
	rows, err := r.DB.QueryContext(ctx, r.DB.Adapt(`
		SELECT id, client_id, question, answer, is_approved, created_at
		FROM faqs
		WHERE client_id = $1
		ORDER BY created_at DESC`), clientID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	faqs := []models.FAQ{}
	for rows.Next() {
		var f models.FAQ
		var isApprovedVal interface{}
		if err := rows.Scan(&f.ID, &f.ClientID, &f.Question, &f.Answer, &isApprovedVal, &f.CreatedAt); err == nil {
			f.IsApproved = parseBoolValue(isApprovedVal)
			faqs = append(faqs, f)
		}
	}
	return faqs, nil
}

// CreateFAQ adds FAQ record.
func (r *SQLPortalRepository) CreateFAQ(ctx context.Context, id, clientID, question, answer string, approved bool) error {
	_, err := r.DB.ExecContext(ctx, r.DB.Adapt(`
		INSERT INTO faqs (id, client_id, question, answer, is_approved)
		VALUES ($1, $2, $3, $4, $5)`),
		id, clientID, question, answer, boolToSQL(r.DB, approved),
	)
	return err
}

// UpdateFAQ edits FAQ details.
func (r *SQLPortalRepository) UpdateFAQ(ctx context.Context, id, clientID, question, answer string, approved bool) (int64, error) {
	res, err := r.DB.ExecContext(ctx, r.DB.Adapt(`
		UPDATE faqs
		SET question = $1, answer = $2, is_approved = $3
		WHERE id = $4 AND client_id = $5`),
		question, answer, boolToSQL(r.DB, approved), id, clientID,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DeleteFAQ removes FAQ records.
func (r *SQLPortalRepository) DeleteFAQ(ctx context.Context, id, clientID string) (int64, error) {
	res, err := r.DB.ExecContext(ctx, r.DB.Adapt(`
		DELETE FROM faqs
		WHERE id = $1 AND client_id = $2`),
		id, clientID,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// UpdatePortalConfig updates client widget attributes.
func (r *SQLPortalRepository) UpdatePortalConfig(ctx context.Context, query string, args ...interface{}) (int64, error) {
	res, err := r.DB.ExecContext(ctx, r.DB.Adapt(query), args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ReplyToConversation logs replies to complaints.
func (r *SQLPortalRepository) ReplyToConversation(ctx context.Context, id, clientID, reply string) (int64, error) {
	res, err := r.DB.ExecContext(ctx, r.DB.Adapt(`
		UPDATE activity_logs
		SET ai_response = $1, status = 'Resolved'
		WHERE id = $2 AND client_id = $3`),
		reply, id, clientID,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// InsertFAQsDraftsTx deletes old drafts and saves new FAQ drafts.
func (r *SQLPortalRepository) InsertFAQsDraftsTx(ctx context.Context, clientID string, drafts []struct{ Question, Answer string }) error {
	tx, err := r.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, r.DB.Adapt(`DELETE FROM faqs WHERE client_id = $1 AND is_approved = 0`), clientID)
	if err != nil {
		return err
	}

	for _, d := range drafts {
		faqID := uuid.New().String()
		var insertErr error
		if r.DB.IsSQLite() {
			_, insertErr = tx.ExecContext(ctx, r.DB.Adapt(`
				INSERT INTO faqs (id, client_id, question, answer, is_approved)
				VALUES ($1, $2, $3, $4, 0)`),
				faqID, clientID, d.Question, d.Answer,
			)
		} else {
			_, insertErr = tx.ExecContext(ctx, r.DB.Adapt(`
				INSERT INTO faqs (id, client_id, question, answer, is_approved)
				VALUES ($1, $2, $3, $4, FALSE)`),
				faqID, clientID, d.Question, d.Answer,
			)
		}
		if insertErr != nil {
			return insertErr
		}
	}
	return tx.Commit()
}

// GetClientInvoices lists invoices for a client.
func (r *SQLPortalRepository) GetClientInvoices(ctx context.Context, clientID string) ([]models.Invoice, error) {
	rows, err := r.DB.QueryContext(ctx, r.DB.Adapt(`
		SELECT id, amount, status, COALESCE(due_date,''), created_at
		FROM invoices WHERE client_id = $1
		ORDER BY created_at DESC`), clientID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var invoices []models.Invoice
	for rows.Next() {
		var inv models.Invoice
		var dueDate sql.NullString
		if err := rows.Scan(&inv.ID, &inv.Amount, &inv.Status, &dueDate, &inv.CreatedAt); err != nil {
			return nil, err
		}
		if dueDate.Valid && dueDate.String != "" {
			t, err := time.Parse("2006-01-02", dueDate.String)
			if err == nil {
				inv.DueDate = &t
			}
		}
		invoices = append(invoices, inv)
	}
	return invoices, nil
}

// GetClientActivityLogs returns logs for CSV export.
func (r *SQLPortalRepository) GetClientActivityLogs(ctx context.Context, clientID string) ([]models.ActivityLog, error) {
	rows, err := r.DB.QueryContext(ctx, r.DB.Adapt(`
		SELECT id, COALESCE(user_ref,'Visitor'), channel, message,
		       COALESCE(ai_response,''), status, latency_ms, created_at
		FROM activity_logs
		WHERE client_id = $1
		ORDER BY created_at DESC
		LIMIT 5000`), clientID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.ActivityLog
	for rows.Next() {
		var e models.ActivityLog
		var userRef, aiResponse sql.NullString
		var createdAt time.Time
		if err := rows.Scan(&e.ID, &userRef, &e.Channel, &e.Message, &aiResponse, &e.Status, &e.LatencyMs, &createdAt); err != nil {
			return nil, err
		}
		if userRef.Valid {
			e.UserRef = userRef.String
		}
		if aiResponse.Valid {
			e.AIResponse = aiResponse.String
		}
		e.CreatedAt = createdAt
		logs = append(logs, e)
	}
	return logs, nil
}
