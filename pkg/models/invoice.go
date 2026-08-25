package models

import (
	"time"
)

// InvoiceStatus represents the payment state of an invoice.
type InvoiceStatus string

const (
	InvoiceStatusPaid    InvoiceStatus = "Paid"
	InvoiceStatusPending InvoiceStatus = "Pending"
	InvoiceStatusOverdue InvoiceStatus = "Overdue"
)

// Invoice represents a billing record issued to a client.
type Invoice struct {
	ID         string        `json:"id"`
	ClientID   *string       `json:"client_id,omitempty"`
	ClientName string        `json:"client"`
	Amount     float64       `json:"amount"`
	Status     InvoiceStatus `json:"status"`
	DueDate    *time.Time    `json:"due_date,omitempty"`
	PaidAt     *time.Time    `json:"paid_at,omitempty"`
	CreatedAt  time.Time     `json:"date"`
}

// CreateInvoiceRequest is the payload for POST /api/v1/billing/invoices.
type CreateInvoiceRequest struct {
	ClientID string  `json:"client_id"  validate:"required"`
	Amount   float64 `json:"amount"     validate:"required,gt=0"`
}
