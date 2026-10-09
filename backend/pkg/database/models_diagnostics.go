package database

import (
	"github.com/google/uuid"
	"time"
)

// ClientDiagnosticEvent stores only closed technical metadata, never health or request payloads.
type ClientDiagnosticEvent struct {
	UserID         uuid.UUID `gorm:"type:uuid;primaryKey" json:"-"`
	ID             string    `gorm:"primaryKey" json:"id"`
	OccurredAt     time.Time `json:"occurred_at"`
	ReceivedAt     time.Time `gorm:"index" json:"-"`
	Operation      string    `json:"operation"`
	Category       string    `json:"category"`
	RequestID      string    `json:"request_id"`
	HTTPCode       int       `json:"http_code"`
	DurationMillis int64     `json:"duration_millis"`
	Attempt        int       `json:"attempt"`
	AppVersion     string    `json:"app_version"`
	AndroidAPI     int       `json:"android_api"`
}
