package repository

import (
	"context"
	"errors"
	"time"

	"github.com/codexylab/alvex-backend/pkg/database"
)

var ErrWhatsAppDeliveryClientMismatch = errors.New("WhatsApp delivery belongs to another client")

type WhatsAppDelivery struct {
	ProviderMessageID string
	ClientID          string
	Recipient         string
	Direction         string
	Status            string
	ProviderTimestamp *time.Time
	ErrorCode         string
	ErrorMessage      string
}

type WhatsAppDeliveryRepository interface {
	Upsert(ctx context.Context, delivery WhatsAppDelivery) error
}

type WhatsAppClientVerifier interface {
	ClientOwnsPhoneNumber(ctx context.Context, clientID, phoneNumberID string) (bool, error)
}

type SQLWhatsAppDeliveryRepository struct {
	DB *database.DB
}

func NewSQLWhatsAppDeliveryRepository(db *database.DB) *SQLWhatsAppDeliveryRepository {
	return &SQLWhatsAppDeliveryRepository{DB: db}
}

func (r *SQLWhatsAppDeliveryRepository) ClientOwnsPhoneNumber(
	ctx context.Context,
	clientID string,
	phoneNumberID string,
) (bool, error) {
	var owns bool
	err := r.DB.QueryRowContext(ctx, r.DB.Adapt(`
		SELECT EXISTS(
			SELECT 1 FROM clients
			WHERE id = $1 AND whatsapp_phone_number_id = $2 AND status = 'Active'
		)`), clientID, phoneNumberID).Scan(&owns)
	return owns, err
}

func (r *SQLWhatsAppDeliveryRepository) Upsert(ctx context.Context, delivery WhatsAppDelivery) error {
	result, err := r.DB.ExecContext(ctx, r.DB.Adapt(`
		INSERT INTO whatsapp_deliveries
		  (provider_message_id, client_id, recipient, direction, status,
		   provider_timestamp, error_code, error_message, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (provider_message_id) DO UPDATE SET
		  status = excluded.status,
		  provider_timestamp = COALESCE(excluded.provider_timestamp, whatsapp_deliveries.provider_timestamp),
		  error_code = excluded.error_code,
		  error_message = excluded.error_message,
		  updated_at = excluded.updated_at
		WHERE whatsapp_deliveries.client_id = excluded.client_id`),
		delivery.ProviderMessageID,
		delivery.ClientID,
		delivery.Recipient,
		delivery.Direction,
		delivery.Status,
		delivery.ProviderTimestamp,
		delivery.ErrorCode,
		truncateJobError(delivery.ErrorMessage),
		time.Now().UTC(),
		time.Now().UTC(),
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrWhatsAppDeliveryClientMismatch
	}
	return nil
}
