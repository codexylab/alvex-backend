package services

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/codexylab/alvex-backend/internal/models"
	"github.com/codexylab/alvex-backend/internal/repository"
	"github.com/codexylab/alvex-backend/pkg/apierr"
)

// BillingStats represents aggregated metrics.
type BillingStats struct {
	TotalMRR        float64            `json:"total_mrr"`
	ActiveClients   int                `json:"active_clients"`
	PendingBalance  float64            `json:"pending_balance"`
	PlanBreakdown   map[string]int     `json:"plan_breakdown"`
}

// BillingService coordinates billing-related operations.
type BillingService struct {
	Repo repository.BillingRepository
}

// NewBillingService creates a new BillingService instance.
func NewBillingService(repo repository.BillingRepository) *BillingService {
	return &BillingService{Repo: repo}
}

// GetStats compiles MRR breakdowns and outstanding invoice balances.
func (s *BillingService) GetStats(ctx context.Context) (*BillingStats, error) {
	planBreakdown, err := s.Repo.GetActivePlansBreakdown(ctx)
	if err != nil {
		return nil, err
	}

	planCosts := map[string]float64{
		"Basic": 29.00, "Pro": 99.00, "Enterprise": 499.00,
	}

	stats := &BillingStats{
		PlanBreakdown: map[string]int{},
	}

	for plan, count := range planBreakdown {
		stats.PlanBreakdown[plan] = count
		stats.TotalMRR += planCosts[plan] * float64(count)
		stats.ActiveClients += count
	}

	pendingBalance, err := s.Repo.GetPendingInvoiceBalance(ctx)
	if err != nil {
		return nil, err
	}
	stats.PendingBalance = pendingBalance

	return stats, nil
}

// ListInvoices lists billing invoices records.
func (s *BillingService) ListInvoices(ctx context.Context) ([]models.Invoice, error) {
	return s.Repo.ListInvoices(ctx)
}

// CreateInvoice manually issues a pending invoice.
func (s *BillingService) CreateInvoice(ctx context.Context, req models.CreateInvoiceRequest) (map[string]interface{}, error) {
	if req.ClientID == "" || req.Amount <= 0 {
		return nil, fmt.Errorf("%w: client ID and valid amount are required", apierr.ErrValidation)
	}

	clientName, err := s.Repo.GetClientNameByID(ctx, req.ClientID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("%w: client not found", apierr.ErrNotFound)
		}
		return nil, err
	}

	count, err := s.Repo.GetInvoicesCount(ctx)
	if err != nil {
		return nil, err
	}

	invoiceID := fmt.Sprintf("INV-%d-%04d", time.Now().Year(), count+1)
	dueDate := time.Now().AddDate(0, 0, 30) // 30-day net terms

	err = s.Repo.CreateInvoice(ctx, invoiceID, req.ClientID, clientName, req.Amount, dueDate)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"id":          invoiceID,
		"client_id":   req.ClientID,
		"client_name": clientName,
		"amount":      req.Amount,
		"status":      "Pending",
		"due_date":    dueDate,
	}, nil
}

// MarkPaid sets an invoice as paid.
func (s *BillingService) MarkPaid(ctx context.Context, id string) error {
	now := time.Now()
	rowsAffected, err := s.Repo.MarkPaid(ctx, id, now)
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("%w: invoice not found or already paid", apierr.ErrNotFound)
	}
	return nil
}

// MarkOverdueInvoices checks and updates overdue invoices in database.
func (s *BillingService) MarkOverdueInvoices(ctx context.Context) error {
	_, err := s.Repo.MarkOverdueInvoices(ctx)
	return err
}
