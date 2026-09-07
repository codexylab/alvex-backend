package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/codexylab/alvex-backend/pkg/database"
	"github.com/codexylab/alvex-backend/pkg/models"
	"github.com/codexylab/alvex-backend/pkg/tenant"
)

// BillingRepository defines database operations for billing and invoices.
type BillingRepository interface {
	GetActivePlansBreakdown(ctx context.Context) (map[string]int, error)
	GetPendingInvoiceBalance(ctx context.Context) (float64, error)
	ListInvoices(ctx context.Context) ([]models.Invoice, error)
	GetClientNameByID(ctx context.Context, clientID string) (string, error)
	GetInvoicesCount(ctx context.Context) (int, error)
	CreateInvoice(ctx context.Context, id, clientID, clientName string, amount float64, dueDate time.Time) error
	MarkPaid(ctx context.Context, id string, paidAt time.Time) (int64, error)
	MarkOverdueInvoices(ctx context.Context) (int64, error)
}

// SQLBillingRepository implements BillingRepository.
type SQLBillingRepository struct {
	DB                  *database.DB
	requireOrganization bool
}

// NewSQLBillingRepository creates a SQLBillingRepository instance.
func NewSQLBillingRepository(db *database.DB) *SQLBillingRepository {
	return &SQLBillingRepository{DB: db}
}

// NewTenantSQLBillingRepository creates a fail-closed billing repository.
func NewTenantSQLBillingRepository(db *database.DB) *SQLBillingRepository {
	return &SQLBillingRepository{DB: db, requireOrganization: true}
}

func (r *SQLBillingRepository) organizationID(ctx context.Context) (string, error) {
	if !r.requireOrganization {
		return "", nil
	}
	return tenant.RequireOrganizationID(ctx)
}

// GetActivePlansBreakdown counts active clients grouped by plan.
func (r *SQLBillingRepository) GetActivePlansBreakdown(ctx context.Context) (map[string]int, error) {
	organizationID, err := r.organizationID(ctx)
	if err != nil {
		return nil, err
	}
	query := `SELECT billing_plan, COUNT(*) FROM clients WHERE status = 'Active'`
	var args []interface{}
	if r.requireOrganization {
		query += ` AND organization_id = $1`
		args = append(args, organizationID)
	}
	query += ` GROUP BY billing_plan`
	rows, err := r.DB.QueryContext(ctx, r.DB.Adapt(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	breakdown := map[string]int{}
	for rows.Next() {
		var plan string
		var count int
		if err := rows.Scan(&plan, &count); err != nil {
			return nil, err
		}
		breakdown[plan] = count
	}
	return breakdown, nil
}

// GetPendingInvoiceBalance sums pending and overdue invoices.
func (r *SQLBillingRepository) GetPendingInvoiceBalance(ctx context.Context) (float64, error) {
	organizationID, err := r.organizationID(ctx)
	if err != nil {
		return 0, err
	}
	query := `SELECT COALESCE(SUM(i.amount), 0) FROM invoices i WHERE i.status IN ('Pending','Overdue')`
	var args []interface{}
	if r.requireOrganization {
		query += ` AND EXISTS (SELECT 1 FROM clients c WHERE c.id = i.client_id AND c.organization_id = $1)`
		args = append(args, organizationID)
	}
	var balance float64
	err = r.DB.QueryRowContext(ctx, r.DB.Adapt(query), args...).Scan(&balance)
	return balance, err
}

// ListInvoices lists all invoices ordered by date descending.
func (r *SQLBillingRepository) ListInvoices(ctx context.Context) ([]models.Invoice, error) {
	organizationID, err := r.organizationID(ctx)
	if err != nil {
		return nil, err
	}
	query := `SELECT i.id, i.client_id, i.client_name, i.amount, i.status, i.due_date, i.paid_at, i.created_at
		 FROM invoices i`
	var args []interface{}
	if r.requireOrganization {
		query += ` JOIN clients c ON c.id = i.client_id WHERE c.organization_id = $1`
		args = append(args, organizationID)
	}
	query += ` ORDER BY i.created_at DESC`
	rows, err := r.DB.QueryContext(ctx, r.DB.Adapt(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	invoices := []models.Invoice{}
	for rows.Next() {
		inv := models.Invoice{}
		var clientID sql.NullString
		var dueDate, paidAt sql.NullTime

		if err := rows.Scan(
			&inv.ID, &clientID, &inv.ClientName,
			&inv.Amount, &inv.Status, &dueDate, &paidAt, &inv.CreatedAt,
		); err != nil {
			return nil, err
		}

		if clientID.Valid {
			inv.ClientID = &clientID.String
		}
		if dueDate.Valid {
			inv.DueDate = &dueDate.Time
		}
		if paidAt.Valid {
			inv.PaidAt = &paidAt.Time
		}
		invoices = append(invoices, inv)
	}
	return invoices, nil
}

// GetClientNameByID resolves client name.
func (r *SQLBillingRepository) GetClientNameByID(ctx context.Context, clientID string) (string, error) {
	organizationID, err := r.organizationID(ctx)
	if err != nil {
		return "", err
	}
	query := `SELECT name FROM clients WHERE id = $1`
	args := []interface{}{clientID}
	if r.requireOrganization {
		query += ` AND organization_id = $2`
		args = append(args, organizationID)
	}
	var name string
	err = r.DB.QueryRowContext(ctx, r.DB.Adapt(query), args...).Scan(&name)
	return name, err
}

// GetInvoicesCount returns total invoice records count.
func (r *SQLBillingRepository) GetInvoicesCount(ctx context.Context) (int, error) {
	organizationID, err := r.organizationID(ctx)
	if err != nil {
		return 0, err
	}
	query := `SELECT COUNT(*) FROM invoices i`
	var args []interface{}
	if r.requireOrganization {
		query += ` JOIN clients c ON c.id = i.client_id WHERE c.organization_id = $1`
		args = append(args, organizationID)
	}
	var count int
	err = r.DB.QueryRowContext(ctx, r.DB.Adapt(query), args...).Scan(&count)
	return count, err
}

// CreateInvoice inserts a pending invoice record.
func (r *SQLBillingRepository) CreateInvoice(ctx context.Context, id, clientID, clientName string, amount float64, dueDate time.Time) error {
	organizationID, err := r.organizationID(ctx)
	if err != nil {
		return err
	}
	query := `INSERT INTO invoices (id, client_id, client_name, amount, status, due_date)
		 VALUES ($1, $2, $3, $4, 'Pending', $5)`
	args := []interface{}{id, clientID, clientName, amount, dueDate}
	if r.requireOrganization {
		query = `INSERT INTO invoices (id, client_id, client_name, amount, status, due_date)
		 SELECT $1, $2, $3, $4, 'Pending', $5
		 WHERE EXISTS (SELECT 1 FROM clients WHERE id = $2 AND organization_id = $6)`
		args = append(args, organizationID)
	}
	result, err := r.DB.ExecContext(ctx, r.DB.Adapt(query), args...)
	if err == nil && r.requireOrganization {
		if affected, rowsErr := result.RowsAffected(); rowsErr != nil || affected == 0 {
			return sql.ErrNoRows
		}
	}
	return err
}

// MarkPaid sets invoice status to Paid.
func (r *SQLBillingRepository) MarkPaid(ctx context.Context, id string, paidAt time.Time) (int64, error) {
	organizationID, err := r.organizationID(ctx)
	if err != nil {
		return 0, err
	}
	query := `UPDATE invoices SET status = 'Paid', paid_at = $1 WHERE id = $2 AND status != 'Paid'`
	args := []interface{}{paidAt, id}
	if r.requireOrganization {
		query += ` AND EXISTS (SELECT 1 FROM clients c WHERE c.id = invoices.client_id AND c.organization_id = $3)`
		args = append(args, organizationID)
	}
	result, err := r.DB.ExecContext(ctx,
		r.DB.Adapt(query), args...,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// MarkOverdueInvoices updates all pending invoices past their due date to 'Overdue'.
func (r *SQLBillingRepository) MarkOverdueInvoices(ctx context.Context) (int64, error) {
	organizationID, err := r.organizationID(ctx)
	if err != nil {
		return 0, err
	}
	var query string
	if r.DB.IsSQLite() {
		query = `UPDATE invoices SET status = 'Overdue' WHERE status = 'Pending' AND due_date < date('now') AND due_date != ''`
	} else {
		query = `UPDATE invoices SET status = 'Overdue' WHERE status = 'Pending' AND due_date < CURRENT_DATE`
	}
	var args []interface{}
	if r.requireOrganization {
		query += ` AND EXISTS (SELECT 1 FROM clients c WHERE c.id = invoices.client_id AND c.organization_id = $1)`
		args = append(args, organizationID)
	}

	result, err := r.DB.ExecContext(ctx, r.DB.Adapt(query), args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
