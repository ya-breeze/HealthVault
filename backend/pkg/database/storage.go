package database

import (
	"errors"
	"time"

	"github.com/google/uuid"
	kinmodels "github.com/ya-breeze/kin-core/models"
	"gorm.io/gorm"
)

// ErrNotFound is returned by DeleteRecord when no row matches (wrong user or non-existent ID).
var ErrNotFound = errors.New("record not found")

type TimeRange struct {
	From time.Time
	To   time.Time
}

// AggFamily selects how QueryAggregate reduces a bucket's rows: summed
// ("cumulative" — steps, distance, calories, ...) or averaged with a
// min/max band ("point" — heart rate, weight, ...). See typeRegistry in
// pkg/server/api.go for the per-type assignment.
type AggFamily string

const (
	AggFamilyCumulative AggFamily = "cumulative"
	AggFamilyPoint      AggFamily = "point"
)

// Bucket is the aggregation granularity for QueryAggregate. There is no
// "week" bucket: the UI's Week and Month zoom levels both use Day and
// differ only in requested time range (see the chart-zoom-aggregation spec).
type Bucket string

const (
	BucketDay   Bucket = "day"
	BucketMonth Bucket = "month"
)

type Storage interface {
	FindUserByName(username string) (*kinmodels.User, error)
	FindUserByID(id uuid.UUID) (*kinmodels.User, error)
	FindUsersByFamilyID(familyID uuid.UUID) ([]kinmodels.User, error)
	AllUsers() ([]kinmodels.User, error)
	SaveWebhookPayload(p *WebhookPayload) error
	// Generic health record queries — returns []map[string]any for JSON serialization.
	// timeCol is the column to filter on ("time", "start_time", etc.).
	QueryRecords(tableName string, timeCol string, userID uuid.UUID, tr TimeRange) ([]map[string]any, error)
	// QueryAggregate returns one row per time bucket for a single value
	// column: {bucket_start, count, sum} for the cumulative family or
	// {bucket_start, count, avg, min, max} for the point family.
	QueryAggregate(tableName, timeCol, valueCol string, family AggFamily, bucket Bucket, userID uuid.UUID, tr TimeRange) ([]map[string]any, error)
	// QueryAggregateBloodPressure and QueryAggregateNutrition handle the two
	// registered types with more than one value column, which QueryAggregate's
	// single-valueCol shape can't express.
	QueryAggregateBloodPressure(bucket Bucket, userID uuid.UUID, tr TimeRange) ([]map[string]any, error)
	QueryAggregateNutrition(bucket Bucket, userID uuid.UUID, tr TimeRange) ([]map[string]any, error)
	// DeleteRecord hard-deletes a single record by ID, scoped to userID.
	// Returns ErrNotFound if no matching row exists or the row belongs to another user.
	DeleteRecord(tableName string, id uuid.UUID, userID uuid.UUID) error
	// Summary data
	SummarySteps(userID uuid.UUID, tr TimeRange) (int, error)
	SummaryAvgHeartRate(userID uuid.UUID, tr TimeRange) (float64, error)
	SummarySleepSeconds(userID uuid.UUID, tr TimeRange) (int, error)
	// DB exposes raw gorm.DB for ingest fan-out
	DB() *gorm.DB
}
