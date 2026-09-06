package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/codexylab/alvex-backend/pkg/database"
	"github.com/codexylab/alvex-backend/pkg/models"
	"github.com/codexylab/alvex-backend/pkg/tenant"
)

// TrendDataPoint represents a single data point in the trends chart.
type TrendDataPoint struct {
	Label   string `json:"label"`
	WebChat int    `json:"web_chat"`
	WA      int    `json:"whatsapp"`
}

// TopQuestion represents a commonly asked question and its frequency.
type TopQuestion struct {
	Question string `json:"question"`
	Count    int    `json:"count"`
}

// SatisfactionStats represents positive vs negative customer feedback.
type SatisfactionStats struct {
	TotalFeedback   int     `json:"total_feedback"`
	PositiveCount   int     `json:"positive_count"`
	NegativeCount   int     `json:"negative_count"`
	SatisfactionPct float64 `json:"satisfaction_pct"`
}

// AnalyticsRepository defines database operations for dashboard metrics and logs.
type AnalyticsRepository interface {
	GetOverviewCounts(ctx context.Context) (total, active int, err error)
	GetBillingPlans(ctx context.Context) ([]string, error)
	GetActivityLogsSummary(ctx context.Context) (totalLogs, failedLogs int, err error)
	GetAvgLatency(ctx context.Context) (float64, error)
	GetTrends(ctx context.Context, period, clientID string) ([]TrendDataPoint, error)
	GetRecentActivity(ctx context.Context, limit int) ([]models.ActivityLog, error)
	GetTopQuestions(ctx context.Context, clientID string, limit int) ([]TopQuestion, error)
	GetFailedQueries(ctx context.Context, clientID string, limit int) ([]models.ActivityLog, error)
	GetSatisfactionStats(ctx context.Context, clientID string) (*SatisfactionStats, error)
	QueryClientsForExport(ctx context.Context) (*sql.Rows, error)
	QueryInvoicesForExport(ctx context.Context) (*sql.Rows, error)
	QueryActivityLogsForExport(ctx context.Context) (*sql.Rows, error)
}

// SQLAnalyticsRepository implements AnalyticsRepository.
type SQLAnalyticsRepository struct {
	DB *database.DB
}

// NewSQLAnalyticsRepository creates a SQLAnalyticsRepository instance.
func NewSQLAnalyticsRepository(db *database.DB) *SQLAnalyticsRepository {
	return &SQLAnalyticsRepository{DB: db}
}

func (r *SQLAnalyticsRepository) GetOverviewCounts(ctx context.Context) (total, active int, err error) {
	organizationID, err := tenant.RequireOrganizationID(ctx)
	if err != nil {
		return 0, 0, err
	}
	err = r.DB.QueryRowContext(ctx, r.DB.Adapt(`SELECT COUNT(*) FROM clients WHERE organization_id = $1`), organizationID).Scan(&total)
	if err != nil {
		return 0, 0, err
	}
	err = r.DB.QueryRowContext(ctx, r.DB.Adapt(`SELECT COUNT(*) FROM clients WHERE organization_id = $1 AND status = 'Active'`), organizationID).Scan(&active)
	return total, active, err
}

func (r *SQLAnalyticsRepository) GetBillingPlans(ctx context.Context) ([]string, error) {
	organizationID, err := tenant.RequireOrganizationID(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := r.DB.QueryContext(ctx, r.DB.Adapt(`SELECT billing_plan FROM clients WHERE organization_id = $1 AND status = 'Active'`), organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var plans []string
	for rows.Next() {
		var plan string
		if err := rows.Scan(&plan); err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	return plans, nil
}

func (r *SQLAnalyticsRepository) GetActivityLogsSummary(ctx context.Context) (totalLogs, failedLogs int, err error) {
	organizationID, err := tenant.RequireOrganizationID(ctx)
	if err != nil {
		return 0, 0, err
	}
	scopeFilter := `EXISTS (SELECT 1 FROM clients c WHERE c.id = activity_logs.client_id AND c.organization_id = $1)`
	err = r.DB.QueryRowContext(ctx, r.DB.Adapt(`SELECT COUNT(*) FROM activity_logs WHERE `+scopeFilter), organizationID).Scan(&totalLogs)
	if err != nil {
		return 0, 0, err
	}
	err = r.DB.QueryRowContext(ctx, r.DB.Adapt(`SELECT COUNT(*) FROM activity_logs WHERE status = 'Failed' AND `+scopeFilter), organizationID).Scan(&failedLogs)
	return totalLogs, failedLogs, err
}

func (r *SQLAnalyticsRepository) GetAvgLatency(ctx context.Context) (float64, error) {
	organizationID, err := tenant.RequireOrganizationID(ctx)
	if err != nil {
		return 0, err
	}
	var avg float64
	err = r.DB.QueryRowContext(ctx, r.DB.Adapt(`
		SELECT COALESCE(AVG(latency_ms), 0) FROM activity_logs
		WHERE latency_ms > 0
		  AND EXISTS (SELECT 1 FROM clients c WHERE c.id = activity_logs.client_id AND c.organization_id = $1)`),
		organizationID,
	).Scan(&avg)
	return avg, err
}

func (r *SQLAnalyticsRepository) GetTrends(ctx context.Context, period, clientID string) ([]TrendDataPoint, error) {
	organizationID, err := tenant.RequireOrganizationID(ctx)
	if err != nil {
		return nil, err
	}
	args := []interface{}{organizationID}
	filter := `
		  AND EXISTS (SELECT 1 FROM clients c WHERE c.id = activity_logs.client_id AND c.organization_id = $1)`
	if clientID != "" {
		filter += ` AND client_id = $2`
		args = append(args, clientID)
	}

	var query string
	if r.DB.IsSQLite() {
		if period == "30d" {
			query = `
				SELECT
					'Week ' || CAST(CEIL(CAST(strftime('%j', created_at) AS REAL) / 7) AS INTEGER) AS label,
					SUM(CASE WHEN channel = 'web' THEN 1 ELSE 0 END) AS web_chat,
					SUM(CASE WHEN channel = 'whatsapp' THEN 1 ELSE 0 END) AS wa
				FROM activity_logs
				WHERE created_at >= datetime('now', '-30 days')` + filter + `
				GROUP BY 1 ORDER BY MIN(created_at)`
		} else {
			query = `
				SELECT
					CASE strftime('%w', created_at)
						WHEN '0' THEN 'Sun' WHEN '1' THEN 'Mon' WHEN '2' THEN 'Tue'
						WHEN '3' THEN 'Wed' WHEN '4' THEN 'Thu' WHEN '5' THEN 'Fri'
						ELSE 'Sat' END AS label,
					SUM(CASE WHEN channel = 'web' THEN 1 ELSE 0 END) AS web_chat,
					SUM(CASE WHEN channel = 'whatsapp' THEN 1 ELSE 0 END) AS wa
				FROM activity_logs
				WHERE created_at >= datetime('now', '-7 days')` + filter + `
				GROUP BY strftime('%Y-%m-%d', created_at) ORDER BY MIN(created_at)`
		}
	} else if period == "30d" {
		query = `
			SELECT
				'Week ' || CEIL(EXTRACT(DOY FROM created_at)::float / 7)::int AS label,
				SUM(CASE WHEN channel = 'web' THEN 1 ELSE 0 END) AS web_chat,
				SUM(CASE WHEN channel = 'whatsapp' THEN 1 ELSE 0 END) AS wa
			FROM activity_logs
			WHERE created_at >= NOW() - INTERVAL '30 days'` + filter + `
			GROUP BY 1 ORDER BY MIN(created_at)`
	} else {
		query = `
			SELECT TO_CHAR(created_at, 'Dy') AS label,
				SUM(CASE WHEN channel = 'web' THEN 1 ELSE 0 END) AS web_chat,
				SUM(CASE WHEN channel = 'whatsapp' THEN 1 ELSE 0 END) AS wa
			FROM activity_logs
			WHERE created_at >= NOW() - INTERVAL '7 days'` + filter + `
			GROUP BY 1, DATE_TRUNC('day', created_at) ORDER BY DATE_TRUNC('day', created_at)`
	}

	rows, err := r.DB.QueryContext(ctx, r.DB.Adapt(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []TrendDataPoint
	for rows.Next() {
		dp := TrendDataPoint{}
		if err := rows.Scan(&dp.Label, &dp.WebChat, &dp.WA); err != nil {
			return nil, err
		}
		points = append(points, dp)
	}
	return points, nil
}

func (r *SQLAnalyticsRepository) GetRecentActivity(ctx context.Context, limit int) ([]models.ActivityLog, error) {
	organizationID, err := tenant.RequireOrganizationID(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := r.DB.QueryContext(ctx, r.DB.Adapt(`
		SELECT id, client_id, client_name, channel, user_ref, message, status, latency_ms, created_at
		FROM activity_logs
		WHERE EXISTS (SELECT 1 FROM clients c WHERE c.id = activity_logs.client_id AND c.organization_id = $1)
		ORDER BY created_at DESC
		LIMIT $2`), organizationID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logs := []models.ActivityLog{}
	for rows.Next() {
		log := models.ActivityLog{}
		var clientID sql.NullString
		var createdAt time.Time

		if err := rows.Scan(
			&log.ID, &clientID, &log.ClientName,
			&log.Channel, &log.UserRef, &log.Message,
			&log.Status, &log.LatencyMs, &createdAt,
		); err != nil {
			return nil, err
		}
		if clientID.Valid {
			log.ClientID = &clientID.String
		}
		log.CreatedAt = createdAt
		logs = append(logs, log)
	}
	return logs, nil
}

func (r *SQLAnalyticsRepository) QueryClientsForExport(ctx context.Context) (*sql.Rows, error) {
	organizationID, err := tenant.RequireOrganizationID(ctx)
	if err != nil {
		return nil, err
	}
	return r.DB.QueryContext(ctx, r.DB.Adapt(`
		SELECT id, name, domain, status, provider, model, billing_plan, created_at
		FROM clients WHERE organization_id = $1 ORDER BY created_at DESC`), organizationID)
}

func (r *SQLAnalyticsRepository) QueryInvoicesForExport(ctx context.Context) (*sql.Rows, error) {
	organizationID, err := tenant.RequireOrganizationID(ctx)
	if err != nil {
		return nil, err
	}
	return r.DB.QueryContext(ctx, r.DB.Adapt(`
		SELECT i.id, i.client_name, i.amount, i.status, COALESCE(i.due_date,''), i.created_at
		FROM invoices i JOIN clients c ON c.id = i.client_id
		WHERE c.organization_id = $1 ORDER BY i.created_at DESC`), organizationID)
}

func (r *SQLAnalyticsRepository) QueryActivityLogsForExport(ctx context.Context) (*sql.Rows, error) {
	organizationID, err := tenant.RequireOrganizationID(ctx)
	if err != nil {
		return nil, err
	}
	return r.DB.QueryContext(ctx, r.DB.Adapt(`
		SELECT a.id, a.client_name, a.channel, a.user_ref, a.message, a.status, a.latency_ms, a.created_at
		FROM activity_logs a JOIN clients c ON c.id = a.client_id
		WHERE c.organization_id = $1 ORDER BY a.created_at DESC LIMIT 5000`), organizationID)
}

// GetTopQuestions returns the most frequent user queries.
func (r *SQLAnalyticsRepository) GetTopQuestions(ctx context.Context, clientID string, limit int) ([]TopQuestion, error) {
	if limit <= 0 {
		limit = 10
	}

	organizationID, err := tenant.RequireOrganizationID(ctx)
	if err != nil {
		return nil, err
	}
	var query string
	var args []interface{}
	if clientID != "" {
		query = `
			SELECT message, COUNT(*) as cnt
			FROM activity_logs
			WHERE EXISTS (SELECT 1 FROM clients c WHERE c.id = activity_logs.client_id AND c.organization_id = $1)
			  AND client_id = $2 AND message != '' AND is_ticket = 0
			GROUP BY message
			ORDER BY cnt DESC
			LIMIT $3
		`
		args = []interface{}{organizationID, clientID, limit}
	} else {
		query = `
			SELECT message, COUNT(*) as cnt
			FROM activity_logs
			WHERE EXISTS (SELECT 1 FROM clients c WHERE c.id = activity_logs.client_id AND c.organization_id = $1)
			  AND message != '' AND is_ticket = 0
			GROUP BY message
			ORDER BY cnt DESC
			LIMIT $2
		`
		args = []interface{}{organizationID, limit}
	}

	rows, err := r.DB.QueryContext(ctx, r.DB.Adapt(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var questions []TopQuestion
	for rows.Next() {
		var q TopQuestion
		if err := rows.Scan(&q.Question, &q.Count); err != nil {
			return nil, err
		}
		questions = append(questions, q)
	}
	return questions, nil
}

// GetFailedQueries returns conversations where AI failed or status is Failed.
func (r *SQLAnalyticsRepository) GetFailedQueries(ctx context.Context, clientID string, limit int) ([]models.ActivityLog, error) {
	if limit <= 0 {
		limit = 20
	}

	organizationID, err := tenant.RequireOrganizationID(ctx)
	if err != nil {
		return nil, err
	}
	var query string
	var args []interface{}
	if clientID != "" {
		query = `
			SELECT id, client_id, client_name, channel, user_ref, session_id, message,
			       COALESCE(ai_response,''), status, latency_ms, created_at
			FROM activity_logs
			WHERE EXISTS (SELECT 1 FROM clients c WHERE c.id = activity_logs.client_id AND c.organization_id = $1)
			  AND client_id = $2 AND status = 'Failed'
			ORDER BY created_at DESC
			LIMIT $3
		`
		args = []interface{}{organizationID, clientID, limit}
	} else {
		query = `
			SELECT id, client_id, client_name, channel, user_ref, session_id, message,
			       COALESCE(ai_response,''), status, latency_ms, created_at
			FROM activity_logs
			WHERE status = 'Failed'
			  AND EXISTS (SELECT 1 FROM clients c WHERE c.id = activity_logs.client_id AND c.organization_id = $1)
			ORDER BY created_at DESC
			LIMIT $2
		`
		args = []interface{}{organizationID, limit}
	}

	rows, err := r.DB.QueryContext(ctx, r.DB.Adapt(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.ActivityLog
	for rows.Next() {
		var l models.ActivityLog
		var cid sql.NullString
		if err := rows.Scan(&l.ID, &cid, &l.ClientName, &l.Channel, &l.UserRef, &l.SessionID, &l.Message, &l.AIResponse, &l.Status, &l.LatencyMs, &l.CreatedAt); err != nil {
			return nil, err
		}
		if cid.Valid {
			l.ClientID = &cid.String
		}
		logs = append(logs, l)
	}
	return logs, nil
}

// GetSatisfactionStats computes feedback statistics from reactions (thumbs_up / thumbs_down).
func (r *SQLAnalyticsRepository) GetSatisfactionStats(ctx context.Context, clientID string) (*SatisfactionStats, error) {
	organizationID, err := tenant.RequireOrganizationID(ctx)
	if err != nil {
		return nil, err
	}
	var query string
	var args []interface{}
	if clientID != "" {
		query = `
			SELECT
				COUNT(*) as total,
				SUM(CASE WHEN reaction = '👍' OR reaction = 'positive' THEN 1 ELSE 0 END) as pos,
				SUM(CASE WHEN reaction = '👎' OR reaction = 'negative' THEN 1 ELSE 0 END) as neg
			FROM activity_logs
			WHERE EXISTS (SELECT 1 FROM clients c WHERE c.id = activity_logs.client_id AND c.organization_id = $1)
			  AND client_id = $2 AND reaction IS NOT NULL AND reaction != ''
		`
		args = []interface{}{organizationID, clientID}
	} else {
		query = `
			SELECT
				COUNT(*) as total,
				SUM(CASE WHEN reaction = '👍' OR reaction = 'positive' THEN 1 ELSE 0 END) as pos,
				SUM(CASE WHEN reaction = '👎' OR reaction = 'negative' THEN 1 ELSE 0 END) as neg
			FROM activity_logs
			WHERE reaction IS NOT NULL AND reaction != ''
			  AND EXISTS (SELECT 1 FROM clients c WHERE c.id = activity_logs.client_id AND c.organization_id = $1)
		`
		args = []interface{}{organizationID}
	}

	var total, pos, neg sql.NullInt64
	err = r.DB.QueryRowContext(ctx, r.DB.Adapt(query), args...).Scan(&total, &pos, &neg)
	if err != nil {
		return &SatisfactionStats{SatisfactionPct: 100.0}, nil
	}

	totVal := int(total.Int64)
	posVal := int(pos.Int64)
	negVal := int(neg.Int64)

	pct := 100.0
	if totVal > 0 {
		pct = (float64(posVal) / float64(totVal)) * 100.0
	}

	return &SatisfactionStats{
		TotalFeedback:   totVal,
		PositiveCount:   posVal,
		NegativeCount:   negVal,
		SatisfactionPct: pct,
	}, nil
}
