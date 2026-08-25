package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/codexylab/alvex-backend/pkg/database"
	"github.com/codexylab/alvex-backend/pkg/models"
)

// clientSelectCols is the shared SELECT column list common to both List and GetByID queries.
// Must stay in sync with the scan order in both functions.
// GetByID extends this with: portal_token, owner_id, guardrails_enabled, guardrails_reply, created_at, updated_at.
const clientSelectCols = `
	id, name, domain, status, provider, model, api_key,
	system_persona, webhook_url, temperature, strict_adherence,
	billing_plan, custom_rate,
	COALESCE(gemini_api_key,''), COALESCE(groq_api_key,''),
	COALESCE(groq_fallback_enabled,0),
	scraped_content, scrape_synced_at, scrape_enabled, scrape_interval_hours,
	COALESCE(widget_chat_enabled,1), COALESCE(widget_ticketing_enabled,1),
	COALESCE(widget_admin_msg_enabled,1), COALESCE(widget_image_search_enabled,1),
	COALESCE(widget_ticketing_allowed,1), COALESCE(widget_admin_msg_allowed,1), COALESCE(widget_image_search_allowed,1),
	COALESCE(widget_brand_name,''), COALESCE(widget_logo_url,''),
	COALESCE(widget_primary_color,''), COALESCE(widget_secondary_color,''),
	COALESCE(widget_remove_branding,0), COALESCE(widget_branding_allowed,1)`

// ClientRepository defines the interface for client database operations.
type ClientRepository interface {
	List(ctx context.Context, search, status string, page, limit int) ([]models.Client, int, error)
	GetByID(ctx context.Context, id string) (*models.Client, error)
	ExistsByID(ctx context.Context, id string) (bool, error)
	Create(ctx context.Context, client *models.Client) error
	Update(ctx context.Context, query string, args ...interface{}) (int64, error)
	UpdateFields(ctx context.Context, id string, fields map[string]interface{}) (int64, error)
	Delete(ctx context.Context, id string) (int64, error)
}

// SQLClientRepository implements ClientRepository for SQL databases.
type SQLClientRepository struct {
	DB *database.DB
}

// NewSQLClientRepository creates a new SQLClientRepository instance.
func NewSQLClientRepository(db *database.DB) *SQLClientRepository {
	return &SQLClientRepository{DB: db}
}

// List retrieves a paginated, searchable, filterable list of clients.
func (r *SQLClientRepository) List(ctx context.Context, search, status string, page, limit int) ([]models.Client, int, error) {
	offset := (page - 1) * limit

	conditions := []string{"1=1"}
	args := []interface{}{}
	argIdx := 1

	if search != "" {
		like := "%" + search + "%"
		if r.DB.IsSQLite() {
			conditions = append(conditions, "(LOWER(name) LIKE ? OR LOWER(domain) LIKE ? OR LOWER(model) LIKE ?)")
			args = append(args, like, like, like)
			argIdx += 3
		} else {
			conditions = append(conditions, fmt.Sprintf(
				"(LOWER(name) LIKE $%d OR LOWER(domain) LIKE $%d OR LOWER(model) LIKE $%d)",
				argIdx, argIdx, argIdx,
			))
			args = append(args, like)
			argIdx++
		}
	}
	if status != "" && status != "All" {
		if r.DB.IsSQLite() {
			conditions = append(conditions, "status = ?")
		} else {
			conditions = append(conditions, fmt.Sprintf("status = $%d", argIdx))
		}
		args = append(args, status)
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")

	var total int
	countQuery := r.DB.Adapt(fmt.Sprintf(`SELECT COUNT(*) FROM clients WHERE %s`, whereClause))
	if err := r.DB.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	dataArgs := append(args, limit, offset)
	var dataQuery string
	if r.DB.IsSQLite() {
		dataQuery = fmt.Sprintf(`SELECT`+clientSelectCols+`,
			created_at, updated_at
		FROM clients
		WHERE %s
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?`, whereClause)
	} else {
		dataQuery = fmt.Sprintf(`SELECT`+clientSelectCols+`,
			created_at, updated_at
		FROM clients
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, whereClause, argIdx, argIdx+1)
	}

	rows, err := r.DB.QueryContext(ctx, r.DB.Adapt(dataQuery), dataArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	clients := []models.Client{}
	for rows.Next() {
		c, err := scanClientRow(rows, false, false)
		if err != nil {
			return nil, 0, err
		}
		// Mask API key in list view for security
		c.APIKey = c.MaskedAPIKey()
		c.GeminiAPIKey = ""
		c.GroqAPIKey = ""
		clients = append(clients, *c)
	}

	return clients, total, nil
}

// GetByID retrieves a single client by ID.
func (r *SQLClientRepository) GetByID(ctx context.Context, id string) (*models.Client, error) {
	row := r.DB.QueryRowContext(ctx, r.DB.Adapt(`SELECT`+clientSelectCols+`,
		portal_token, owner_id,
		COALESCE(guardrails_enabled,0), COALESCE(guardrails_reply,''),
		created_at, updated_at
	FROM clients WHERE id = $1`), id)

	return scanClientRow(row, true, false)
}

// ExistsByID checks if a client exists.
func (r *SQLClientRepository) ExistsByID(ctx context.Context, id string) (bool, error) {
	var exists bool
	err := r.DB.QueryRowContext(ctx, r.DB.Adapt(`SELECT EXISTS(SELECT 1 FROM clients WHERE id = $1)`), id).Scan(&exists)
	return exists, err
}

// Create inserts a new client into the database.
func (r *SQLClientRepository) Create(ctx context.Context, c *models.Client) error {
	_, err := r.DB.ExecContext(ctx, r.DB.Adapt(`
		INSERT INTO clients
		  (id, name, domain, status, provider, model, api_key, system_persona,
		   webhook_url, temperature, strict_adherence, billing_plan, portal_token, owner_id,
		   widget_chat_enabled, widget_ticketing_enabled, widget_admin_msg_enabled, widget_image_search_enabled,
		   widget_ticketing_allowed, widget_admin_msg_allowed, widget_image_search_allowed)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)`),
		c.ID, c.Name, c.Domain, c.Status,
		c.Provider, c.Model, c.APIKey, c.SystemPersona,
		c.WebhookURL, c.Temperature, c.StrictAdherence, c.BillingPlan, c.PortalToken, c.OwnerID,
		c.WidgetChatEnabled, c.WidgetTicketingEnabled, c.WidgetAdminMsgEnabled, c.WidgetImageSearchEnabled,
		c.WidgetTicketingAllowed, c.WidgetAdminMsgAllowed, c.WidgetImageSearchAllowed,
	)
	return err
}

// Update executes a custom update query on the database.
func (r *SQLClientRepository) Update(ctx context.Context, query string, args ...interface{}) (int64, error) {
	result, err := r.DB.ExecContext(ctx, r.DB.Adapt(query), args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// UpdateFields dynamically updates specified fields for a client by ID.
func (r *SQLClientRepository) UpdateFields(ctx context.Context, id string, fields map[string]interface{}) (int64, error) {
	if len(fields) == 0 {
		return 0, nil
	}

	sets := []string{}
	args := []interface{}{}
	argIdx := 1

	for k, v := range fields {
		if r.DB.IsSQLite() {
			sets = append(sets, fmt.Sprintf("%s = ?", k))
		} else {
			sets = append(sets, fmt.Sprintf("%s = $%d", k, argIdx))
		}
		args = append(args, v)
		argIdx++
	}

	args = append(args, id)
	var query string
	if r.DB.IsSQLite() {
		query = fmt.Sprintf("UPDATE clients SET %s WHERE id = ?", strings.Join(sets, ", "))
	} else {
		query = fmt.Sprintf("UPDATE clients SET %s WHERE id = $%d", strings.Join(sets, ", "), argIdx)
	}

	result, err := r.DB.ExecContext(ctx, r.DB.Adapt(query), args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// Delete deletes a client from the database.
func (r *SQLClientRepository) Delete(ctx context.Context, id string) (int64, error) {
	result, err := r.DB.ExecContext(ctx, r.DB.Adapt(`DELETE FROM clients WHERE id = $1`), id)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
