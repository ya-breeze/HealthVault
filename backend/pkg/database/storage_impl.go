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

// bucketExpr returns a SQLite expression that truncates timeCol to the start
// of its day or month bucket, formatted as RFC3339 UTC. strftime only
// substitutes its %-directives; the literal "T00:00:00Z" (and "-01" for
// month) is copied through unchanged, so this doubles as both the GROUP BY
// key and the bucket_start value.
func bucketExpr(bucket Bucket, timeCol string) (string, error) {
	switch bucket {
	case BucketDay:
		return fmt.Sprintf("strftime('%%Y-%%m-%%dT00:00:00Z', %s)", timeCol), nil
	case BucketMonth:
		return fmt.Sprintf("strftime('%%Y-%%m-01T00:00:00Z', %s)", timeCol), nil
	default:
		return "", fmt.Errorf("unknown bucket %q", bucket)
	}
}

func (s *storageImpl) QueryAggregate(
	tableName, timeCol, valueCol string, family AggFamily, bucket Bucket, userID uuid.UUID, tr TimeRange,
) ([]map[string]any, error) {
	be, err := bucketExpr(bucket, timeCol)
	if err != nil {
		return nil, err
	}
	var selectExpr string
	switch family {
	case AggFamilyCumulative:
		selectExpr = fmt.Sprintf("%s AS bucket_start, COUNT(*) AS count, SUM(%s) AS sum", be, valueCol)
	case AggFamilyPoint:
		selectExpr = fmt.Sprintf(
			"%s AS bucket_start, COUNT(*) AS count, AVG(%s) AS avg, MIN(%s) AS min, MAX(%s) AS max",
			be, valueCol, valueCol, valueCol,
		)
	default:
		return nil, fmt.Errorf("unknown aggregation family %q", family)
	}
	var results []map[string]any
	whereClause := fmt.Sprintf("user_id = ? AND %s >= ? AND %s <= ?", timeCol, timeCol)
	err = s.db.Table(tableName).
		Select(selectExpr).
		Where(whereClause, userID, tr.From, tr.To).
		Group(be).
		Order(be).
		Find(&results).Error
	return results, err
}

func (s *storageImpl) QueryAggregateBloodPressure(bucket Bucket, userID uuid.UUID, tr TimeRange) ([]map[string]any, error) {
	be, err := bucketExpr(bucket, "time")
	if err != nil {
		return nil, err
	}
	selectExpr := fmt.Sprintf(
		`%s AS bucket_start, COUNT(*) AS count,
		AVG(systolic) AS systolic_avg, MIN(systolic) AS systolic_min, MAX(systolic) AS systolic_max,
		AVG(diastolic) AS diastolic_avg, MIN(diastolic) AS diastolic_min, MAX(diastolic) AS diastolic_max`,
		be,
	)
	var results []map[string]any
	err = s.db.Table("blood_pressures").
		Select(selectExpr).
		Where("user_id = ? AND time >= ? AND time <= ?", userID, tr.From, tr.To).
		Group(be).
		Order(be).
		Find(&results).Error
	return results, err
}

func (s *storageImpl) QueryAggregateNutrition(bucket Bucket, userID uuid.UUID, tr TimeRange) ([]map[string]any, error) {
	be, err := bucketExpr(bucket, "start_time")
	if err != nil {
		return nil, err
	}
	selectExpr := fmt.Sprintf(
		`%s AS bucket_start, COUNT(*) AS count,
		SUM(calories) AS sum_calories, SUM(protein_grams) AS sum_protein_grams, SUM(carbs_grams) AS sum_carbs_grams,
		SUM(fat_grams) AS sum_fat_grams, SUM(sugar_grams) AS sum_sugar_grams, SUM(sodium_grams) AS sum_sodium_grams,
		SUM(dietary_fiber_grams) AS sum_dietary_fiber_grams`,
		be,
	)
	var results []map[string]any
	err = s.db.Table("nutritions").
		Select(selectExpr).
		Where("user_id = ? AND start_time >= ? AND start_time <= ?", userID, tr.From, tr.To).
		Group(be).
		Order(be).
		Find(&results).Error
	return results, err
}

// stepsBucketKey computes the same bucket_start string bucketExpr's SQL
// expression would produce for t, in Go — needed because QueryAggregateSteps
// and SummarySteps assign a kept record to its bucket after streaming it
// off a *sql.Rows cursor, not via a SQL GROUP BY.
func stepsBucketKey(bucket Bucket, t time.Time) (string, error) {
	t = t.UTC()
	switch bucket {
	case BucketDay:
		return fmt.Sprintf("%04d-%02d-%02dT00:00:00Z", t.Year(), t.Month(), t.Day()), nil
	case BucketMonth:
		return fmt.Sprintf("%04d-%02d-01T00:00:00Z", t.Year(), t.Month()), nil
	default:
		return "", fmt.Errorf("unknown bucket %q", bucket)
	}
}

func (s *storageImpl) QueryAggregateSteps(bucket Bucket, userID uuid.UUID, tr TimeRange) ([]map[string]any, error) {
	if bucket != BucketDay && bucket != BucketMonth {
		return nil, fmt.Errorf("unknown bucket %q", bucket)
	}
	rows, err := s.db.Model(&Steps{}).
		Select("start_time, end_time, count").
		Where("user_id = ? AND start_time >= ? AND start_time <= ?", userID, tr.From, tr.To).
		Order("start_time, end_time").
		Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	type bucketAccum struct {
		bucketStart string
		count       int64
		sum         int64
	}
	var buckets []bucketAccum
	var wm stepWatermark
	for rows.Next() {
		var start, end time.Time
		var count int
		if err := rows.Scan(&start, &end, &count); err != nil {
			return nil, err
		}
		if !wm.admit(end) {
			continue
		}
		key, err := stepsBucketKey(bucket, start)
		if err != nil {
			return nil, err
		}
		if n := len(buckets); n > 0 && buckets[n-1].bucketStart == key {
			buckets[n-1].count++
			buckets[n-1].sum += int64(count)
		} else {
			buckets = append(buckets, bucketAccum{bucketStart: key, count: 1, sum: int64(count)})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	results := make([]map[string]any, len(buckets))
	for i, b := range buckets {
		results[i] = map[string]any{"bucket_start": b.bucketStart, "count": b.count, "sum": b.sum}
	}
	return results, nil
}

func (s *storageImpl) SummarySteps(userID uuid.UUID, tr TimeRange) (int, error) {
	rows, err := s.db.Model(&Steps{}).
		Select("start_time, end_time, count").
		Where("user_id = ? AND start_time >= ? AND start_time <= ?", userID, tr.From, tr.To).
		Order("start_time, end_time").
		Rows()
	if err != nil {
		return 0, err
	}
	defer rows.Close() //nolint:errcheck

	var total int
	var wm stepWatermark
	for rows.Next() {
		var start, end time.Time
		var count int
		if err := rows.Scan(&start, &end, &count); err != nil {
			return 0, err
		}
		if !wm.admit(end) {
			continue
		}
		total += count
	}
	return total, rows.Err()
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
