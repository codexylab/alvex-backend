package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/codexylab/alvex-backend/internal/database"
	"github.com/codexylab/alvex-backend/internal/models"
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
	DB *database.DB
}

// NewSQLBillingRepository creates a SQLBillingRepository instance.
func NewSQLBillingRepository(db *database.DB) *SQLBillingRepository {
	return &SQLBillingRepository{DB: db}
}

// GetActivePlansBreakdown counts active clients grouped by plan.
func (r *SQLBillingRepository) GetActivePlansBreakdown(ctx context.Context) (map[string]int, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT billing_plan, COUNT(*) FROM clients WHERE status = 'Active' GROUP BY billing_plan`,
	)
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
	var balance float64
	err := r.DB.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(amount), 0) FROM invoices WHERE status IN ('Pending','Overdue')`,
	).Scan(&balance)
	return balance, err
}

// ListInvoices lists all invoices ordered by date descending.
func (r *SQLBillingRepository) ListInvoices(ctx context.Context) ([]models.Invoice, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT id, client_id, client_name, amount, status, due_date, paid_at, created_at
		 FROM invoices ORDER BY created_at DESC`,
	)
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
	var name string
	err := r.DB.QueryRowContext(ctx,
		r.DB.Adapt(`SELECT name FROM clients WHERE id = $1`), clientID,
	).Scan(&name)
	return name, err
}

// GetInvoicesCount returns total invoice records count.
func (r *SQLBillingRepository) GetInvoicesCount(ctx context.Context) (int, error) {
	var count int
	err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM invoices`).Scan(&count)
	return count, err
}

// CreateInvoice inserts a pending invoice record.
func (r *SQLBillingRepository) CreateInvoice(ctx context.Context, id, clientID, clientName string, amount float64, dueDate time.Time) error {
	_, err := r.DB.ExecContext(ctx,
		r.DB.Adapt(`INSERT INTO invoices (id, client_id, client_name, amount, status, due_date)
		 VALUES ($1, $2, $3, $4, 'Pending', $5)`),
		id, clientID, clientName, amount, dueDate,
	)
	return err
}

// MarkPaid sets invoice status to Paid.
func (r *SQLBillingRepository) MarkPaid(ctx context.Context, id string, paidAt time.Time) (int64, error) {
	result, err := r.DB.ExecContext(ctx,
		r.DB.Adapt(`UPDATE invoices SET status = 'Paid', paid_at = $1 WHERE id = $2 AND status != 'Paid'`),
		paidAt, id,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// MarkOverdueInvoices updates all pending invoices past their due date to 'Overdue'.
func (r *SQLBillingRepository) MarkOverdueInvoices(ctx context.Context) (int64, error) {
	var query string
	if r.DB.IsSQLite() {
		query = `UPDATE invoices SET status = 'Overdue' WHERE status = 'Pending' AND due_date < date('now') AND due_date != ''`
	} else {
		query = `UPDATE invoices SET status = 'Overdue' WHERE status = 'Pending' AND due_date < CURRENT_DATE`
	}

	result, err := r.DB.ExecContext(ctx, r.DB.Adapt(query))
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
