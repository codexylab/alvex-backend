package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/codexylab/alvex-backend/internal/database"
	"github.com/codexylab/alvex-backend/internal/models"
)

// HistoryItem is a lightweight chat history record.
type HistoryItem struct {
	ID         string    `json:"id"`
	Message    string    `json:"message"`
	AIResponse string    `json:"ai_response"`
	Status     string    `json:"status"`
	IsTicket   bool      `json:"is_ticket"`
	CreatedAt  time.Time `json:"created_at"`
	Reaction   string    `json:"reaction"`
	ImageData  string    `json:"image_data"`
}

// ActivityRepository manages activity logs and chat history.
type ActivityRepository interface {
	InsertActivityLog(ctx context.Context, id, clientID, clientName, channel, userRef, sessionID, message, aiResponse, status string, latency int64, isTicket int, reaction string, imageData string) error
	FetchSessionHistory(ctx context.Context, clientID, sessionID string, limit int) ([]models.ActivityLog, error)
	GetWebChatHistory(ctx context.Context, clientID, sessionID string) ([]HistoryItem, error)
	GetWebTicketHistory(ctx context.Context, clientID, ticketRef string) ([]HistoryItem, error)
	GetApprovedFAQs(ctx context.Context, clientID string) ([]models.FAQ, error)
	CleanupOldChats(ctx context.Context, clientID string, retentionDays int) (int64, error)
	UpdateReaction(ctx context.Context, id, reaction string) error
}

// SQLActivityRepository implements ActivityRepository.
type SQLActivityRepository struct {
	DB *database.DB
}

// NewSQLActivityRepository creates a SQLActivityRepository instance.
func NewSQLActivityRepository(db *database.DB) *SQLActivityRepository {
	return &SQLActivityRepository{DB: db}
}

// InsertActivityLog saves an activity log to the database.
func (r *SQLActivityRepository) InsertActivityLog(ctx context.Context, id, clientID, clientName, channel, userRef, sessionID, message, aiResponse, status string, latency int64, isTicket int, reaction string, imageData string) error {
	_, err := r.DB.ExecContext(ctx, r.DB.Adapt(`
		INSERT INTO activity_logs
		  (id, client_id, client_name, channel, user_ref, session_id, message, ai_response, status, latency_ms, is_ticket, reaction, image_data)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`),
		id, clientID, clientName, channel, userRef, sessionID, message, aiResponse, status, latency, isTicket, reaction, imageData,
	)
	return err
}

// FetchSessionHistory retrieves the last N turns.
func (r *SQLActivityRepository) FetchSessionHistory(ctx context.Context, clientID, sessionID string, limit int) ([]models.ActivityLog, error) {
	rows, err := r.DB.QueryContext(ctx, r.DB.Adapt(`
		SELECT message, ai_response
		FROM   activity_logs
		WHERE  client_id  = $1
		  AND  session_id = $2
		  AND  status     = 'Resolved'
		ORDER BY created_at DESC
		LIMIT $3`),
		clientID, sessionID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []models.ActivityLog
	for rows.Next() {
		var h models.ActivityLog
		var aiResponse sql.NullString
		if err := rows.Scan(&h.Message, &aiResponse); err != nil {
			return nil, err
		}
		if aiResponse.Valid {
			h.AIResponse = aiResponse.String
		}
		history = append(history, h)
	}
	return history, nil
}

// GetWebChatHistory fetches chat history.
func (r *SQLActivityRepository) GetWebChatHistory(ctx context.Context, clientID, sessionID string) ([]HistoryItem, error) {
	rows, err := r.DB.QueryContext(ctx, r.DB.Adapt(`
		SELECT id, message, COALESCE(ai_response,''), status, COALESCE(is_ticket,0), created_at, COALESCE(reaction,''), COALESCE(image_data,'')
		FROM activity_logs
		WHERE client_id = $1 AND session_id = $2 AND is_ticket = 0
		ORDER BY created_at ASC`), clientID, sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []HistoryItem
	for rows.Next() {
		var item HistoryItem
		var isTicketVal interface{}
		if err := rows.Scan(&item.ID, &item.Message, &item.AIResponse, &item.Status, &isTicketVal, &item.CreatedAt, &item.Reaction, &item.ImageData); err != nil {
			return nil, err
		}
		switch v := isTicketVal.(type) {
		case int64:
			item.IsTicket = v == 1
		case bool:
			item.IsTicket = v
		}
		history = append(history, item)
	}
	return history, nil
}

// GetWebTicketHistory fetches ticket history.
func (r *SQLActivityRepository) GetWebTicketHistory(ctx context.Context, clientID, ticketRef string) ([]HistoryItem, error) {
	rows, err := r.DB.QueryContext(ctx, r.DB.Adapt(`
		SELECT id, message, COALESCE(ai_response,''), status, COALESCE(is_ticket,0), created_at, COALESCE(reaction,''), COALESCE(image_data,'')
		FROM activity_logs
		WHERE client_id = $1 AND user_ref = $2 AND is_ticket = 1
		ORDER BY created_at ASC`), clientID, ticketRef,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []HistoryItem
	for rows.Next() {
		var item HistoryItem
		var isTicketVal interface{}
		if err := rows.Scan(&item.ID, &item.Message, &item.AIResponse, &item.Status, &isTicketVal, &item.CreatedAt, &item.Reaction, &item.ImageData); err != nil {
			return nil, err
		}
		switch v := isTicketVal.(type) {
		case int64:
			item.IsTicket = v == 1
		case bool:
			item.IsTicket = v
		}
		history = append(history, item)
	}
	return history, nil
}

// GetApprovedFAQs returns approved FAQs.
func (r *SQLActivityRepository) GetApprovedFAQs(ctx context.Context, clientID string) ([]models.FAQ, error) {
	rows, err := r.DB.QueryContext(ctx, r.DB.Adapt(`
		SELECT question, answer FROM faqs
		WHERE client_id = $1 AND is_approved = 1
		ORDER BY created_at ASC`), clientID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var faqs []models.FAQ
	for rows.Next() {
		var f models.FAQ
		if err := rows.Scan(&f.Question, &f.Answer); err != nil {
			return nil, err
		}
		faqs = append(faqs, f)
	}
	return faqs, nil
}

// CleanupOldChats deletes old AI chat messages (not tickets) from activity_logs
// based on client's individual chat_retention_days preference.
func (r *SQLActivityRepository) CleanupOldChats(ctx context.Context, clientID string, retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		return 0, nil
	}

	var query string
	var args []interface{}
	if r.DB.IsSQLite() {
		query = `DELETE FROM activity_logs WHERE client_id = ? AND is_ticket = 0 AND created_at < datetime('now', ? || ' days')`
		args = []interface{}{clientID, fmt.Sprintf("-%d", retentionDays)}
	} else {
		query = `DELETE FROM activity_logs WHERE client_id = $1 AND is_ticket = FALSE AND created_at < NOW() - INTERVAL '1 day' * $2`
		args = []interface{}{clientID, retentionDays}
	}

	result, err := r.DB.ExecContext(ctx, r.DB.Adapt(query), args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// UpdateReaction updates the reaction of a message.
func (r *SQLActivityRepository) UpdateReaction(ctx context.Context, id, reaction string) error {
	_, err := r.DB.ExecContext(ctx, r.DB.Adapt(`
		UPDATE activity_logs
		SET reaction = $1
		WHERE id = $2`),
		reaction, id,
	)
	return err
}
