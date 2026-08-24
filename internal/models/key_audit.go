package models

import "time"

// KeyAuditLog records audit trails whenever sensitive API keys or portal tokens are rotated.
type KeyAuditLog struct {
	ID        string    `json:"id"`
	ClientID  string    `json:"client_id"`
	KeyType   string    `json:"key_type"`   // "portal_token", "api_key", "gemini_key", "groq_key"
	RotatedBy string    `json:"rotated_by"` // user ID or "admin"
	CreatedAt time.Time `json:"created_at"`
}
