package database

import (
	"time"

	"github.com/google/uuid"
	kinmodels "github.com/ya-breeze/kin-core/models"
	"gorm.io/gorm"
)

type TimeRange struct {
	From time.Time
	To   time.Time
}

type Storage interface {
	FindUserByName(username string) (*kinmodels.User, error)
	FindUserByID(id uuid.UUID) (*kinmodels.User, error)
	FindUsersByFamilyID(familyID uuid.UUID) ([]kinmodels.User, error)
	AllUsers() ([]kinmodels.User, error)
	SaveWebhookPayload(p *WebhookPayload) error
	// Generic health record queries — returns []map[string]any for JSON serialization.
	// timeCol is the column to filter on ("time", "start_time", etc.).
	QueryRecords(tableName string, timeCol string, userID uuid.UUID, tr TimeRange) ([]map[string]any, error)
	// Summary data
	SummarySteps(userID uuid.UUID, tr TimeRange) (int, error)
	SummaryAvgHeartRate(userID uuid.UUID, tr TimeRange) (float64, error)
	SummarySleepSeconds(userID uuid.UUID, tr TimeRange) (int, error)
	// DB exposes raw gorm.DB for ingest fan-out
	DB() *gorm.DB
}
