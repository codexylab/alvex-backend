package models

import "time"

// Lead represents a potential customer captured through the chat widget lead generation form.
type Lead struct {
	ID        string    `json:"id"`
	ClientID  string    `json:"client_id"`
	Name      string    `json:"name"`
	Email     string    `json:"email,omitempty"`
	Phone     string    `json:"phone,omitempty"`
	SessionID string    `json:"session_id,omitempty"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateLeadRequest represents the incoming payload from the chat widget.
type CreateLeadRequest struct {
	Name      string `json:"name"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	SessionID string `json:"session_id"`
}
