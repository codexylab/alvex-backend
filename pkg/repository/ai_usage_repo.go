package repository

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/codexylab/alvex-backend/pkg/database"
)

// AIUsageEvent is an immutable record of one successful provider generation.
// The token ledger can be priced later from a versioned catalog instead of
// guessing at request time from rates that change independently of ALVEX.
type AIUsageEvent struct {
	OrganizationID   string
	ClientID         string
	Provider         string
	Model            string
	Operation        string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	CreatedAt        time.Time
}

type AIUsageRepository interface {
	Record(ctx context.Context, event AIUsageEvent) error
}

type AIUsageSummary struct {
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	Operation        string `json:"operation"`
	Requests         int64  `json:"requests"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
}

type AIUsageOperationsRepository interface {
	Summarize(ctx context.Context, since time.Time) ([]AIUsageSummary, error)
}

type SQLAIUsageRepository struct {
	DB *database.DB
}

func NewSQLAIUsageRepository(db *database.DB) *SQLAIUsageRepository {
	return &SQLAIUsageRepository{DB: db}
}

func (r *SQLAIUsageRepository) Record(ctx context.Context, event AIUsageEvent) error {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	if event.TotalTokens == 0 {
		event.TotalTokens = event.PromptTokens + event.CompletionTokens
	}
	_, err := r.DB.ExecContext(ctx, r.DB.Adapt(`
		INSERT INTO ai_usage_events
		  (id, organization_id, client_id, provider, model, operation,
		   prompt_tokens, completion_tokens, total_tokens, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`),
		uuid.NewString(), event.OrganizationID, event.ClientID, event.Provider,
		event.Model, event.Operation, event.PromptTokens, event.CompletionTokens,
		event.TotalTokens, event.CreatedAt,
	)
	return err
}

func (r *SQLAIUsageRepository) Summarize(ctx context.Context, since time.Time) ([]AIUsageSummary, error) {
	rows, err := r.DB.QueryContext(ctx, r.DB.Adapt(`
		SELECT provider, model, operation, COUNT(*),
		       COALESCE(SUM(prompt_tokens), 0),
		       COALESCE(SUM(completion_tokens), 0),
		       COALESCE(SUM(total_tokens), 0)
		FROM ai_usage_events
		WHERE created_at >= $1
		GROUP BY provider, model, operation
		ORDER BY SUM(total_tokens) DESC, provider, model, operation`), since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	summaries := make([]AIUsageSummary, 0)
	for rows.Next() {
		var summary AIUsageSummary
		if err := rows.Scan(
			&summary.Provider,
			&summary.Model,
			&summary.Operation,
			&summary.Requests,
			&summary.PromptTokens,
			&summary.CompletionTokens,
			&summary.TotalTokens,
		); err != nil {
			return nil, err
		}
		summaries = append(summaries, summary)
	}
	return summaries, rows.Err()
}
