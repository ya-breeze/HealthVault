package database

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/mattn/go-sqlite3"
	kinmodels "github.com/ya-breeze/kin-core/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type storageImpl struct {
	db *gorm.DB
}

func NewStorage(db *gorm.DB) Storage {
	return &storageImpl{db: db}
}

func (s *storageImpl) DB() *gorm.DB { return s.db }

func (s *storageImpl) FindUserByName(username string) (*kinmodels.User, error) {
	var u kinmodels.User
	if err := s.db.Where("username = ?", username).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *storageImpl) FindUserByID(id uuid.UUID) (*kinmodels.User, error) {
	var u kinmodels.User
	if err := s.db.Where("id = ?", id).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *storageImpl) FindUsersByFamilyID(familyID uuid.UUID) ([]kinmodels.User, error) {
	var users []kinmodels.User
	return users, s.db.Where("family_id = ?", familyID).Find(&users).Error
}

func (s *storageImpl) AllUsers() ([]kinmodels.User, error) {
	var users []kinmodels.User
	return users, s.db.Find(&users).Error
}

func (s *storageImpl) SaveWebhookPayload(p *WebhookPayload) error {
	return s.db.Create(p).Error
}

// columnAllowlist restricts QueryRecords to a safe column set for tables
// whose schema carries fields that must never reach a generic API response.
// food_meals carries photo_path (a server filesystem path) and raw_response
// (the full raw vision model JSON); an unprojected Find would render both
// into the frontend's generic data table and expose them to any consumer
// of GET /api/data/food_meal, including family members. Tables absent from
// this map are queried unrestricted (select *).
var columnAllowlist = map[string][]string{
	"food_meals": {
		"id", "logged_at", "name", "status",
		"calories", "protein_grams", "carbs_grams", "fat_grams",
		"sugar_grams", "sodium_grams", "dietary_fiber_grams",
	},
}

func (s *storageImpl) DeleteRecord(tableName string, id uuid.UUID, userID uuid.UUID) error {
	result := s.db.Exec(
		"DELETE FROM "+tableName+" WHERE id = ? AND user_id = ?",
		id, userID,
	)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	// Sleep rows own SleepStage children; SQLite FK enforcement is off by default,
	// so we cascade manually to avoid orphaned stage rows. FoodMeal rows own
	// FoodItem children the same way.
	switch tableName {
	case "sleeps":
		if err := s.db.Exec("DELETE FROM sleep_stages WHERE sleep_id = ?", id).Error; err != nil {
			return err
		}
	case "food_meals":
		if err := s.db.Exec("DELETE FROM food_items WHERE meal_id = ?", id).Error; err != nil {
			return err
		}
	}
	return nil
}

// InsertRecord inserts a single manually-authored point record via a raw
// parameterized INSERT (there's no Go struct shared across weight/height/
// weight_goal to hand GORM's struct-based Create, and GORM's map-based
// Create needs a registered schema to build its RETURNING clause, which a
// bare Table() call doesn't have). tableName/timeCol/valueCol are always
// sourced from typeRegistry, never user input, so building the query string
// with them is safe — same trust boundary DeleteRecord and QueryRecords
// already rely on. The row is re-read by ID afterward so the response
// matches QueryRecords' map[string]any shape exactly, including
// DB-computed defaults.
func (s *storageImpl) InsertRecord(
	tableName, timeCol, valueCol string, familyID, userID uuid.UUID, t time.Time, value float64,
) (map[string]any, error) {
	id := uuid.New()
	now := time.Now().UTC()
	query := fmt.Sprintf(
		"INSERT INTO %s (id, family_id, user_id, created_at, updated_at, %s, %s) VALUES (?, ?, ?, ?, ?, ?, ?)",
		tableName, timeCol, valueCol,
	)
	if err := s.db.Exec(query, id, familyID, userID, now, now, t, value).Error; err != nil {
		var sqliteErr sqlite3.Error
		if errors.As(err, &sqliteErr) && sqliteErr.ExtendedCode == sqlite3.ErrConstraintUnique {
			return nil, ErrConflict
		}
		return nil, err
	}
	// Take, not First: First appends "ORDER BY <primary key>" by default,
	// which needs a registered schema to resolve the primary key column —
	// unavailable on a bare Table() call with no Model. Take has no default
	// order, which is fine since Where("id = ?") already narrows to at most
	// one row.
	var result map[string]any
	if err := s.db.Table(tableName).Where("id = ?", id).Take(&result).Error; err != nil {
		return nil, err
	}
	return result, nil
}

func (s *storageImpl) QueryRecords(tableName string, timeCol string, userID uuid.UUID, tr TimeRange) ([]map[string]any, error) {
	var results []map[string]any
	q := s.db.Table(tableName)
	if cols, ok := columnAllowlist[tableName]; ok {
		q = q.Select(cols)
	}
	query := fmt.Sprintf("user_id = ? AND %s >= ? AND %s <= ?", timeCol, timeCol)
	err := q.Where(query, userID, tr.From, tr.To).
		Find(&results).Error
	return results, err
}

// slotSeconds is the width of the SQL-side pre-aggregation slot in
// QueryAggregate/QueryAggregateBloodPressure/QueryAggregateNutrition: the
// coarsest width that never straddles a local-day or local-month boundary.
// Every offset in the modern IANA database is a whole number of 15-minute
// steps — Asia/Kathmandu at +05:45, Australia/Eucla at +08:45 and
// Pacific/Chatham at +12:45 are the finest — and every DST transition lands
// on a slot edge, so folding slots into local buckets (bucket_regroup.go's
// foldSlotsToBuckets) is exact rather than an approximation that merely
// gets closer on most days. Integer division of a negative epoch second
// truncates toward zero rather than flooring, so this mis-slots any row
// before 1970-01-01T00:00:00Z; health data is post-1970, so that limit is
// accepted rather than handled.
const slotSeconds = 900

// slotExpr returns the SQLite expression that groups timeCol into
// slotSeconds-wide slots since the Unix epoch — the 15-minute slot index,
// selected as the integer `slot` column bucket_regroup.go's fold reads to
// derive each row's local bucket.
func slotExpr(timeCol string) string {
	return fmt.Sprintf("CAST(strftime('%%s', %s) AS INTEGER) / %d", timeCol, slotSeconds)
}

// validateBucket rejects any Bucket value other than the two SQL and the Go
// fold both understand, matching bucketExpr's old validation now that no
// single SQL expression stands in for "the bucket" to fail unknown values.
func validateBucket(bucket Bucket) error {
	switch bucket {
	case BucketDay, BucketMonth:
		return nil
	default:
		return fmt.Errorf("unknown bucket %q", bucket)
	}
}

func (s *storageImpl) QueryAggregate(
	tableName, timeCol, valueCol string, family AggFamily, bucket Bucket, loc *time.Location,
	userID uuid.UUID, tr TimeRange,
) ([]map[string]any, error) {
	if err := validateBucket(bucket); err != nil {
		return nil, err
	}
	se := slotExpr(timeCol)
	var selectExpr string
	var cols []aggColumn
	switch family {
	case AggFamilyCumulative:
		selectExpr = fmt.Sprintf("%s AS slot, COUNT(*) AS count, SUM(%s) AS sum", se, valueCol)
		cols = []aggColumn{
			{name: "count", kind: colSum, slotCol: "count"},
			{name: "sum", kind: colSum, slotCol: "sum"},
		}
	case AggFamilyPoint:
		selectExpr = fmt.Sprintf(
			"%s AS slot, COUNT(*) AS count, COUNT(%s) AS value_count, SUM(%s) AS sum, MIN(%s) AS min, MAX(%s) AS max",
			se, valueCol, valueCol, valueCol, valueCol,
		)
		cols = []aggColumn{
			{name: "count", kind: colSum, slotCol: "count"},
			{name: "avg", kind: colAvg, sumCol: "sum", countCol: "value_count"},
			{name: "min", kind: colMin, slotCol: "min"},
			{name: "max", kind: colMax, slotCol: "max"},
		}
	default:
		return nil, fmt.Errorf("unknown aggregation family %q", family)
	}
	var rows []map[string]any
	whereClause := fmt.Sprintf("user_id = ? AND %s >= ? AND %s <= ?", timeCol, timeCol)
	err := s.db.Table(tableName).
		Select(selectExpr).
		Where(whereClause, userID, tr.From, tr.To).
		Group(se).
		Order(se).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return foldSlotsToBuckets(rows, bucket, loc, cols), nil
}

func (s *storageImpl) QueryAggregateBloodPressure(
	bucket Bucket, loc *time.Location, userID uuid.UUID, tr TimeRange,
) ([]map[string]any, error) {
	if err := validateBucket(bucket); err != nil {
		return nil, err
	}
	se := slotExpr("time")
	selectExpr := fmt.Sprintf(
		`%s AS slot, COUNT(*) AS count,
		COUNT(systolic) AS systolic_count, SUM(systolic) AS systolic_sum,
		MIN(systolic) AS systolic_min, MAX(systolic) AS systolic_max,
		COUNT(diastolic) AS diastolic_count, SUM(diastolic) AS diastolic_sum,
		MIN(diastolic) AS diastolic_min, MAX(diastolic) AS diastolic_max`,
		se,
	)
	var rows []map[string]any
	err := s.db.Table("blood_pressures").
		Select(selectExpr).
		Where("user_id = ? AND time >= ? AND time <= ?", userID, tr.From, tr.To).
		Group(se).
		Order(se).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	cols := []aggColumn{
		{name: "count", kind: colSum, slotCol: "count"},
		{name: "systolic_avg", kind: colAvg, sumCol: "systolic_sum", countCol: "systolic_count"},
		{name: "systolic_min", kind: colMin, slotCol: "systolic_min"},
		{name: "systolic_max", kind: colMax, slotCol: "systolic_max"},
		{name: "diastolic_avg", kind: colAvg, sumCol: "diastolic_sum", countCol: "diastolic_count"},
		{name: "diastolic_min", kind: colMin, slotCol: "diastolic_min"},
		{name: "diastolic_max", kind: colMax, slotCol: "diastolic_max"},
	}
	return foldSlotsToBuckets(rows, bucket, loc, cols), nil
}

func (s *storageImpl) QueryAggregateNutrition(
	bucket Bucket, loc *time.Location, userID uuid.UUID, tr TimeRange,
) ([]map[string]any, error) {
	if err := validateBucket(bucket); err != nil {
		return nil, err
	}
	se := slotExpr("start_time")
	selectExpr := fmt.Sprintf(
		`%s AS slot, COUNT(*) AS count,
		SUM(calories) AS sum_calories, SUM(protein_grams) AS sum_protein_grams, SUM(carbs_grams) AS sum_carbs_grams,
		SUM(fat_grams) AS sum_fat_grams, SUM(sugar_grams) AS sum_sugar_grams, SUM(sodium_grams) AS sum_sodium_grams,
		SUM(dietary_fiber_grams) AS sum_dietary_fiber_grams`,
		se,
	)
	var rows []map[string]any
	err := s.db.Table("nutritions").
		Select(selectExpr).
		Where("user_id = ? AND start_time >= ? AND start_time <= ?", userID, tr.From, tr.To).
		Group(se).
		Order(se).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	cols := []aggColumn{
		{name: "count", kind: colSum, slotCol: "count"},
		{name: "sum_calories", kind: colSum, slotCol: "sum_calories"},
		{name: "sum_protein_grams", kind: colSum, slotCol: "sum_protein_grams"},
		{name: "sum_carbs_grams", kind: colSum, slotCol: "sum_carbs_grams"},
		{name: "sum_fat_grams", kind: colSum, slotCol: "sum_fat_grams"},
		{name: "sum_sugar_grams", kind: colSum, slotCol: "sum_sugar_grams"},
		{name: "sum_sodium_grams", kind: colSum, slotCol: "sum_sodium_grams"},
		{name: "sum_dietary_fiber_grams", kind: colSum, slotCol: "sum_dietary_fiber_grams"},
	}
	return foldSlotsToBuckets(rows, bucket, loc, cols), nil
}

func (s *storageImpl) SummarySteps(userID uuid.UUID, tr TimeRange) (int, error) {
	var total int
	err := s.db.Model(&Steps{}).
		Where("user_id = ? AND start_time >= ? AND start_time <= ?", userID, tr.From, tr.To).
		Select("COALESCE(SUM(count), 0)").Scan(&total).Error
	return total, err
}

func (s *storageImpl) SummaryAvgHeartRate(userID uuid.UUID, tr TimeRange) (float64, error) {
	var avg float64
	err := s.db.Model(&HeartRate{}).
		Where("user_id = ? AND time >= ? AND time <= ?", userID, tr.From, tr.To).
		Select("COALESCE(AVG(bpm), 0)").Scan(&avg).Error
	return avg, err
}

func (s *storageImpl) SummarySleepSeconds(userID uuid.UUID, tr TimeRange) (int, error) {
	var total int
	err := s.db.Model(&Sleep{}).
		Where("user_id = ? AND start_time >= ? AND start_time <= ?", userID, tr.From, tr.To).
		Select("COALESCE(SUM(duration_seconds), 0)").Scan(&total).Error
	return total, err
}

func (s *storageImpl) GetUserSettings(userID uuid.UUID) (string, error) {
	var row UserSettings
	if err := s.db.Where("user_id = ?", userID).First(&row).Error; err != nil {
		return "", err
	}
	return row.SettingsJSON, nil
}

func (s *storageImpl) UpsertUserSettings(userID, familyID uuid.UUID, settingsJSON string) error {
	return upsertUserSettings(s.db, userID, familyID, settingsJSON)
}

func (s *storageImpl) UpsertUserSettingsClearingFoodDayCompletions(
	userID, familyID uuid.UUID, settingsJSON string,
) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := upsertUserSettings(tx, userID, familyID, settingsJSON); err != nil {
			return err
		}
		// Unscoped: FoodDayCompletion embeds TenantModel, so a plain Delete
		// soft-deletes rather than removing the rows. The
		// (user_id, local_date) unique index has no deleted_at clause, so a
		// soft-deleted row would permanently block re-confirming that date —
		// the same CustomFood/DeleteCustomFood trap this mirrors.
		return tx.Unscoped().Where("user_id = ?", userID).Delete(&FoodDayCompletion{}).Error
	})
}

func upsertUserSettings(db *gorm.DB, userID, familyID uuid.UUID, settingsJSON string) error {
	row := UserSettings{UserID: userID, SettingsJSON: settingsJSON}
	row.ID = uuid.New()
	row.FamilyID = familyID
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"settings_json", "updated_at"}),
	}).Create(&row).Error
}

var _ Storage = (*storageImpl)(nil) // compile-time interface check
