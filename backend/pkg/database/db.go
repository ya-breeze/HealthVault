package database

import (
	"context"
	"fmt"
	"log/slog"
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
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
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
		&Steps{}, &Distance{}, &ActiveCalories{}, &TotalCalories{}, &Hydration{},
		&HeartRate{}, &HeartRateVariability{}, &Weight{}, &Height{},
		&BloodGlucose{}, &OxygenSaturation{}, &BodyTemperature{},
		&RespiratoryRate{}, &RestingHeartRate{}, &BasalMetabolicRate{},
		&BodyFat{}, &LeanBodyMass{}, &VO2Max{}, &BoneMass{},
		&BloodPressure{}, &SkinTemperature{},
		&Sleep{}, &SleepStage{},
		&Exercise{}, &Nutrition{},
	); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}
