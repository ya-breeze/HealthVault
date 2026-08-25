package database

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ya-breeze/kin-core/authdb"
	kinmodels "github.com/ya-breeze/kin-core/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

type slogGormLogger struct {
	l *slog.Logger
}

func (g *slogGormLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface { return g }
func (g *slogGormLogger) Info(ctx context.Context, msg string, args ...interface{}) {
	g.l.InfoContext(ctx, fmt.Sprintf(msg, args...))
}
func (g *slogGormLogger) Warn(ctx context.Context, msg string, args ...interface{}) {
	g.l.WarnContext(ctx, fmt.Sprintf(msg, args...))
}
func (g *slogGormLogger) Error(ctx context.Context, msg string, args ...interface{}) {
	g.l.ErrorContext(ctx, fmt.Sprintf(msg, args...))
}
func (g *slogGormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
}

func Open(l *slog.Logger, dbPath string) (*gorm.DB, error) {
	sep := "?"
	if strings.Contains(dbPath, "?") {
		sep = "&"
	}
	dsn := dbPath + sep + "_busy_timeout=30000&_journal_mode=WAL"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: &slogGormLogger{l: l},
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := db.AutoMigrate(
		&kinmodels.Family{},
		&kinmodels.User{},
		&authdb.RefreshToken{},
		&authdb.BlacklistedToken{},
		&WebhookPayload{},
		&UserSettings{},
		&Steps{}, &Distance{}, &ActiveCalories{}, &TotalCalories{}, &Hydration{},
		&HeartRate{}, &HeartRateVariability{}, &Weight{}, &Height{}, &WeightGoal{},
		&BloodGlucose{}, &OxygenSaturation{}, &BodyTemperature{},
		&RespiratoryRate{}, &RestingHeartRate{}, &BasalMetabolicRate{},
		&BodyFat{}, &LeanBodyMass{}, &VO2Max{}, &BoneMass{},
		&BloodPressure{}, &SkinTemperature{},
		&Sleep{}, &SleepStage{},
		&Exercise{}, &Nutrition{},
		&Speed{},
		&FoodMeal{}, &FoodItem{}, &CustomFood{}, &FoodCalibrationSample{}, &FoodSearchTranslation{},
		&FoodDayCompletion{},
	); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	if err := backfillFoodMealLoggedAtToUTC(l, db); err != nil {
		return nil, fmt.Errorf("backfill logged_at: %w", err)
	}
	return db, nil
}

// backfillFoodMealLoggedAtToUTC normalizes any food_meals.logged_at value
// still stored with a non-UTC offset. go-sqlite3 stores time.Time as TEXT
// preserving whatever offset it was given rather than normalizing it, so
// SQLite orders and filters that column as text, not as an instant — a row
// written with a non-UTC offset sorts and compares incorrectly against
// everything else. The write paths (CreateManualMeal, ConfirmMeal,
// PatchMeal) all normalize to UTC before storing now, but that only
// prevents *new* non-UTC rows: it was believed at the time that fix landed
// that no existing row could be affected, since the only shipped client
// always sends UTC — but the API itself accepted arbitrary RFC3339 offsets
// from any direct caller before that fix, and AutoMigrate does not rewrite
// existing rows. This runs once per Open (idempotent — a row already
// normalized has a zero UTC offset and is skipped) so the on-disk data
// actually matches the ordering/filtering guarantee, not just future writes.
//
// Uses UpdateColumn, not Update: a plain Update also advances the model's
// UpdatedAt via GORM's hooks, and UpdatedAt is more than an audit field
// here — it's the analysis lease token Reanalyze/Retry/ClarifyMeal claim
// against, and RetryMeal's clock for judging a processing meal stale. A
// meal legitimately stuck in processing from a crashed worker that happens
// to also need this backfill would have its lease silently refreshed by a
// migration that has nothing to do with analysis state, blocking Retry
// until another full vision timeout elapses for no reason tied to any
// actual attempt. UpdateColumn writes only the given column and skips
// hooks entirely, so the backfill can't manufacture a lease.
func backfillFoodMealLoggedAtToUTC(l *slog.Logger, db *gorm.DB) error {
	var meals []FoodMeal
	if err := db.Select("id", "logged_at").Find(&meals).Error; err != nil {
		return fmt.Errorf("load food_meals for logged_at backfill: %w", err)
	}
	var fixed int
	for _, m := range meals {
		if _, offset := m.LoggedAt.Zone(); offset == 0 {
			continue
		}
		if err := db.Model(&FoodMeal{}).Where("id = ?", m.ID).
			UpdateColumn("logged_at", m.LoggedAt.UTC()).Error; err != nil {
			return fmt.Errorf("backfill logged_at for meal %s: %w", m.ID, err)
		}
		fixed++
	}
	if fixed > 0 {
		l.Info("backfilled non-UTC food_meals.logged_at values", "count", fixed)
	}
	return nil
}
