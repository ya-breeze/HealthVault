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

// ErrConflict is returned by InsertRecord when a row already exists at the
// requested (user_id, timeCol) instant, per the table's unique index. Only
// reachable via an explicit, colliding time — an omitted time always
// defaults to now.
var ErrConflict = errors.New("record already exists for this time")

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

// Bucket is the aggregation granularity for QueryAggregate. Day and month
// buckets are local calendar days and months — resolved in the *time.Location
// passed to QueryAggregate/QueryAggregateBloodPressure/QueryAggregateNutrition
// — not UTC calendar days, and bucket_start labels a local calendar date
// (see LocalBucketKey). There is no "week" bucket: the UI's Week and Month
// zoom levels both use Day and differ only in requested time range (see the
// chart-zoom-aggregation spec).
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
	// {bucket_start, count, avg, min, max} for the point family. The bucket
	// resolves in loc (a nil loc is treated as time.UTC).
	QueryAggregate(
		tableName, timeCol, valueCol string, family AggFamily, bucket Bucket, loc *time.Location,
		userID uuid.UUID, tr TimeRange,
	) ([]map[string]any, error)
	// QueryAggregateBloodPressure and QueryAggregateNutrition handle the two
	// registered types with more than one value column, which QueryAggregate's
	// single-valueCol shape can't express. The bucket resolves in loc (a nil
	// loc is treated as time.UTC).
	QueryAggregateBloodPressure(bucket Bucket, loc *time.Location, userID uuid.UUID, tr TimeRange) ([]map[string]any, error)
	QueryAggregateNutrition(bucket Bucket, loc *time.Location, userID uuid.UUID, tr TimeRange) ([]map[string]any, error)
	// DeleteRecord hard-deletes a single record by ID, scoped to userID.
	// Returns ErrNotFound if no matching row exists or the row belongs to another user.
	DeleteRecord(tableName string, id uuid.UUID, userID uuid.UUID) error
	// InsertRecord inserts one manually-authored point record (used by the
	// allowlisted POST /api/data/{type} write path) and returns it in the
	// same map[string]any shape QueryRecords returns. Returns ErrConflict
	// if a row already exists at (userID, t) under the table's unique
	// (user_id, timeCol) index.
	InsertRecord(
		tableName, timeCol, valueCol string, familyID, userID uuid.UUID, t time.Time, value float64,
	) (map[string]any, error)
	// Summary data
	SummarySteps(userID uuid.UUID, tr TimeRange) (int, error)
	SummaryAvgHeartRate(userID uuid.UUID, tr TimeRange) (float64, error)
	SummarySleepSeconds(userID uuid.UUID, tr TimeRange) (int, error)
	// DB exposes raw gorm.DB for ingest fan-out
	DB() *gorm.DB
	// GetUserSettings returns the raw settings JSON for userID, or
	// gorm.ErrRecordNotFound if the user has never saved settings.
	GetUserSettings(userID uuid.UUID) (string, error)
	// UpsertUserSettings replaces the user's entire settings document in a
	// single atomic upsert (no read-modify-write).
	UpsertUserSettings(userID, familyID uuid.UUID, settingsJSON string) error
	// UpsertUserSettingsClearingFoodDayCompletions does what UpsertUserSettings
	// does, plus hard-deletes (Unscoped) every FoodDayCompletion row for
	// userID, in a single transaction. For use when the caller's timezone
	// setting is changing: an existing confirmation's LocalDate was computed
	// under the old zone and can end up matched against a different set of
	// meals once the zone changes, so every confirmation is dropped rather
	// than risking a stale one silently misattaching itself — see design.md
	// §4 "Storage" under openspec/changes/food-day-completeness.
	UpsertUserSettingsClearingFoodDayCompletions(userID, familyID uuid.UUID, settingsJSON string) error
}
