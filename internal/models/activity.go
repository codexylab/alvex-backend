package models

import (
	"time"
)

// ActivityChannel represents the communication channel source.
type ActivityChannel string

const (
	ChannelWeb       ActivityChannel = "web"
	ChannelWhatsApp  ActivityChannel = "whatsapp"
)

// ActivityStatus represents the resolution state of a conversation entry.
type ActivityStatus string

const (
	ActivityHandling   ActivityStatus = "Handling..."
	ActivityResolved   ActivityStatus = "Resolved"
	ActivityArchived   ActivityStatus = "Archived"
	ActivityFailed     ActivityStatus = "Failed"
	ActivityNeedsHuman ActivityStatus = "Needs Human"
)

// ActivityLog represents a single conversation event in the live feed.
type ActivityLog struct {
	ID            string          `json:"id"`
	ClientID      *string         `json:"client_id,omitempty"`
	ClientName    string          `json:"client"`
	Channel       ActivityChannel `json:"type"`
	UserRef       string          `json:"user"`
	SessionID     string          `json:"session_id,omitempty"`
	Message       string          `json:"text"`
	AIResponse    string          `json:"ai_response,omitempty"`
	Status        ActivityStatus  `json:"status"`
	LatencyMs     int64           `json:"latency_ms,omitempty"`
	NeedsHuman    bool            `json:"needs_human"`
	HumanReply    string          `json:"human_reply,omitempty"`
	RepliedAt     *time.Time      `json:"replied_at,omitempty"`
	HandoffReason string          `json:"handoff_reason,omitempty"`
	CreatedAt     time.Time       `json:"time"`
}
