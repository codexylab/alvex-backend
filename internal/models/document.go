package models

import "time"

// Document represents an uploaded file (PDF, DOCX, CSV, TXT) processed for a client's knowledge base.
type Document struct {
	ID        string    `json:"id"`
	ClientID  string    `json:"client_id"`
	Filename  string    `json:"filename"`
	FileType  string    `json:"file_type"`
	FileSize  int64     `json:"file_size"`
	Status    string    `json:"status"` // "processed", "processing", "failed"
	CreatedAt time.Time `json:"created_at"`
}
