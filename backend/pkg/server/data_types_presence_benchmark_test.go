package server

import (
	"fmt"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ya-breeze/healthvault/pkg/database"
	kinmodels "github.com/ya-breeze/kin-core/models"
	"gorm.io/gorm"
)

// BenchmarkDashboardReadPath is the reproducible database baseline for the
// dashboard's pre-read-model request wave. Each component is measured both
// alone and as the old browser-driven sequence, with setup and fixture writes
// outside the timed region. The presence operation intentionally mirrors the
// serial COUNT implementation that this idea replaces.
func BenchmarkDashboardReadPath(b *testing.B) {
	for _, profile := range []struct {
		name            string
		highCardinality bool
		settings        bool
	}{{
		name: "empty",
	}, {
		name:     "populated",
		settings: true,
	}, {
		name:            "high_cardinality_presence",
		settings:        true,
		highCardinality: true,
	}} {
		profile := profile
		b.Run(profile.name, func(b *testing.B) {
			fixture := newDashboardBenchmarkFixture(b, profile.settings, profile.highCardinality)

			benchmarks := []struct {
				name string
				fn   func() error
			}{
				{name: "settings", fn: func() error {
					_, err := readUserSettingsJSON(fixture.storage, fixture.userID)
					return err
				}},
				{name: "presence_serial_count", fn: func() error {
					_, err := legacyPresenceForBenchmark(fixture.storage.DB(), fixture.userID)
					return err
				}},
				{name: "presence_union_exists", fn: func() error {
					presence, err := dataTypesPresence(fixture.storage.DB(), fixture.userID)
					dashboardBenchmarkSink = presence
					return err
				}},
				{name: "aggregate_steps", fn: func() error {
					return benchmarkDailyAggregate(fixture, "steps")
				}},
				{name: "aggregate_heart_rate", fn: func() error {
					return benchmarkDailyAggregate(fixture, "heart_rate")
				}},
				{name: "aggregate_sleep", fn: func() error {
					return benchmarkDailyAggregate(fixture, "sleep")
				}},
				{name: "aggregate_heart_rate_variability", fn: func() error {
					return benchmarkDailyAggregate(fixture, "heart_rate_variability")
				}},
				{name: "aggregate_distance", fn: func() error {
					return benchmarkDailyAggregate(fixture, "distance")
				}},
				{name: "aggregate_weight", fn: func() error {
					return benchmarkDailyAggregate(fixture, "weight")
				}},
				{name: "aggregate_blood_pressure", fn: func() error {
					return benchmarkDailyAggregate(fixture, "blood_pressure")
				}},
				{name: "aggregate_oxygen_saturation", fn: func() error {
					return benchmarkDailyAggregate(fixture, "oxygen_saturation")
				}},
				{name: "needs_attention", fn: func() error {
					return benchmarkNeedsAttentionCount(fixture.storage.DB(), fixture.userID)
				}},
				{name: "legacy_fresh_load", fn: func() error {
					return benchmarkLegacyFreshLoad(fixture)
				}},
				{name: "current_fresh_load", fn: func() error {
					return benchmarkCurrentFreshLoad(fixture)
				}},
			}

			for _, benchmark := range benchmarks {
				benchmark := benchmark
				b.Run(benchmark.name, func(b *testing.B) {
					counter := installBenchmarkStatementCounter(b, fixture.storage.DB())
					b.ReportAllocs()
					b.ResetTimer()
					for range b.N {
						if err := benchmark.fn(); err != nil {
							b.Fatal(err)
						}
					}
					b.StopTimer()
					b.ReportMetric(float64(counter.Load())/float64(b.N), "sql-statements/op")
				})
			}
		})
	}
}

type dashboardBenchmarkFixture struct {
	storage database.Storage
	userID  uuid.UUID
	loc     *time.Location
	range_  database.TimeRange
}

func newDashboardBenchmarkFixture(b *testing.B, withSettings, highCardinality bool) dashboardBenchmarkFixture {
	b.Helper()
	db, err := database.Open(slog.New(slog.NewTextHandler(io.Discard, nil)), ":memory:")
	if err != nil {
		b.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = sqlDB.Close() })

	storage := database.NewStorage(db)
	familyID := uuid.New()
	userID := uuid.New()
	if err := db.Create(&kinmodels.Family{ID: familyID, Name: "BenchmarkFamily"}).Error; err != nil {
		b.Fatal(err)
	}
	if err := db.Create(&kinmodels.User{
		ID: userID, Username: "benchmark-user", PasswordHash: "x", FamilyID: familyID,
	}).Error; err != nil {
		b.Fatal(err)
	}
	if withSettings {
		settings := database.UserSettings{
			UserID:       userID,
			SettingsJSON: `{"timezone":"UTC","dashboard_order":[{"type":"steps","hidden":false}]}`,
		}
		settings.ID = uuid.New()
		settings.FamilyID = familyID
		if err := db.Create(&settings).Error; err != nil {
			b.Fatal(err)
		}
	}

	start := time.Date(2026, time.January, 1, 8, 0, 0, 0, time.UTC)
	if withSettings {
		seedDashboardVitals(b, db, userID, familyID, start)
		seedDashboardMeals(b, db, userID, familyID, start)
	}
	if highCardinality {
		seedHighCardinalitySteps(b, db, userID, familyID, start.Add(20*24*time.Hour))
	}

	return dashboardBenchmarkFixture{
		storage: storage,
		userID:  userID,
		loc:     time.UTC,
		range_: database.TimeRange{
			From: start.Add(-24 * time.Hour),
			To:   start.Add(15 * 24 * time.Hour),
		},
	}
}

func seedDashboardVitals(b *testing.B, db *gorm.DB, userID, familyID uuid.UUID, start time.Time) {
	b.Helper()
	for day := range 14 {
		t := start.AddDate(0, 0, day)
		payloadID := uuid.New()

		steps := database.Steps{
			UserID: userID, SourcePayloadID: payloadID, StartTime: t,
			EndTime: t.Add(time.Hour), Count: 1000 + day,
		}
		steps.ID, steps.FamilyID = uuid.New(), familyID
		createBenchmarkRecord(b, db, &steps)

		heartRate := database.HeartRate{
			UserID: userID, SourcePayloadID: uuid.New(), Time: t, BPM: sixtyPlus(day),
		}
		heartRate.ID, heartRate.FamilyID = uuid.New(), familyID
		createBenchmarkRecord(b, db, &heartRate)

		sleep := database.Sleep{
			UserID: userID, SourcePayloadID: uuid.New(), StartTime: t,
			SessionEndTime: t.Add(8 * time.Hour), DurationSeconds: 8 * 60 * 60,
		}
		sleep.ID, sleep.FamilyID = uuid.New(), familyID
		createBenchmarkRecord(b, db, &sleep)

		hrv := database.HeartRateVariability{
			UserID: userID, SourcePayloadID: uuid.New(), Time: t, RmssdMillis: 42.5,
		}
		hrv.ID, hrv.FamilyID = uuid.New(), familyID
		createBenchmarkRecord(b, db, &hrv)

		distance := database.Distance{
			UserID: userID, SourcePayloadID: uuid.New(), StartTime: t,
			EndTime: t.Add(time.Hour), Meters: 1200,
		}
		distance.ID, distance.FamilyID = uuid.New(), familyID
		createBenchmarkRecord(b, db, &distance)

		weight := database.Weight{
			UserID: userID, SourcePayloadID: ptrUUID(uuid.New()), Time: t, Kilograms: 80,
		}
		weight.ID, weight.FamilyID = uuid.New(), familyID
		createBenchmarkRecord(b, db, &weight)

		bloodPressure := database.BloodPressure{
			UserID: userID, SourcePayloadID: uuid.New(), Time: t, Systolic: 120, Diastolic: 80,
		}
		bloodPressure.ID, bloodPressure.FamilyID = uuid.New(), familyID
		createBenchmarkRecord(b, db, &bloodPressure)

		oxygenSaturation := database.OxygenSaturation{
			UserID: userID, SourcePayloadID: uuid.New(), Time: t, Percentage: 98,
		}
		oxygenSaturation.ID, oxygenSaturation.FamilyID = uuid.New(), familyID
		createBenchmarkRecord(b, db, &oxygenSaturation)
	}
}

func seedDashboardMeals(b *testing.B, db *gorm.DB, userID, familyID uuid.UUID, start time.Time) {
	b.Helper()
	for i, status := range []string{
		database.MealStatusProcessing,
		database.MealStatusPendingReview,
		database.MealStatusConfirmed,
	} {
		meal := database.FoodMeal{
			UserID: userID, Status: status, LoggedAt: start.Add(time.Duration(i) * time.Hour), Name: "Benchmark meal",
		}
		meal.ID, meal.FamilyID = uuid.New(), familyID
		createBenchmarkRecord(b, db, &meal)
	}
}

func seedHighCardinalitySteps(b *testing.B, db *gorm.DB, userID, familyID uuid.UUID, start time.Time) {
	b.Helper()
	const rows = 5000
	for i := range rows {
		t := start.Add(time.Duration(i) * time.Minute)
		steps := database.Steps{
			UserID: userID, SourcePayloadID: uuid.New(), StartTime: t,
			EndTime: t.Add(time.Minute), Count: 100 + i%100,
		}
		steps.ID, steps.FamilyID = uuid.New(), familyID
		createBenchmarkRecord(b, db, &steps)
	}
}

func createBenchmarkRecord(b *testing.B, db *gorm.DB, value any) {
	b.Helper()
	if err := db.Create(value).Error; err != nil {
		b.Fatal(err)
	}
}

func ptrUUID(value uuid.UUID) *uuid.UUID { return &value }

func sixtyPlus(day int) int { return 60 + day }

func legacyPresenceForBenchmark(db *gorm.DB, userID uuid.UUID) (map[string]bool, error) {
	presence := make(map[string]bool, len(typeRegistry))
	for name, info := range typeRegistry {
		var count int64
		if err := db.Table(info.table).Where("user_id = ?", userID).Count(&count).Error; err != nil {
			return nil, err
		}
		presence[name] = count > 0
	}
	return presence, nil
}

var dashboardBenchmarkSink any

func benchmarkDailyAggregate(fixture dashboardBenchmarkFixture, typeName string) error {
	info := typeRegistry[typeName]
	rows, err := queryBucketed(
		fixture.storage, typeName, info, database.BucketDay, fixture.loc, fixture.userID, fixture.range_,
	)
	dashboardBenchmarkSink = rows
	return err
}

// benchmarkDashboardDailyAggregate includes the timezone/settings lookup that
// DataHandler performs before every real bucketed dashboard request. The
// component benchmarks above isolate queryBucketed itself; the combined
// fresh-load sequences must include the complete post-user-resolution path.
func benchmarkDashboardDailyAggregate(fixture dashboardBenchmarkFixture, typeName string) error {
	loc, err := resolveViewerTimezone(fixture.storage, fixture.userID)
	if err != nil {
		return err
	}
	info := typeRegistry[typeName]
	rows, err := queryBucketed(
		fixture.storage, typeName, info, database.BucketDay, loc, fixture.userID, fixture.range_,
	)
	dashboardBenchmarkSink = rows
	return err
}

func benchmarkNeedsAttentionCount(db *gorm.DB, userID uuid.UUID) error {
	_, err := needsAttentionCount(db, userID)
	return err
}

func benchmarkLegacyFreshLoad(fixture dashboardBenchmarkFixture) error {
	if _, err := readUserSettingsJSON(fixture.storage, fixture.userID); err != nil {
		return err
	}
	if _, err := legacyPresenceForBenchmark(fixture.storage.DB(), fixture.userID); err != nil {
		return err
	}
	for _, typeName := range []string{
		"steps", "heart_rate", "sleep", "heart_rate_variability", "distance", "weight",
		"blood_pressure", "oxygen_saturation",
	} {
		if err := benchmarkDashboardDailyAggregate(fixture, typeName); err != nil {
			return err
		}
	}
	return benchmarkNeedsAttentionCount(fixture.storage.DB(), fixture.userID)
}

func benchmarkCurrentFreshLoad(fixture dashboardBenchmarkFixture) error {
	if _, err := readUserSettingsJSON(fixture.storage, fixture.userID); err != nil {
		return err
	}
	if _, err := dataTypesPresence(fixture.storage.DB(), fixture.userID); err != nil {
		return err
	}
	for _, typeName := range []string{
		"steps", "heart_rate", "sleep", "heart_rate_variability", "distance", "weight",
		"blood_pressure", "oxygen_saturation",
	} {
		if err := benchmarkDashboardDailyAggregate(fixture, typeName); err != nil {
			return err
		}
	}
	return benchmarkNeedsAttentionCount(fixture.storage.DB(), fixture.userID)
}

func installBenchmarkStatementCounter(b *testing.B, db *gorm.DB) *atomic.Int64 {
	b.Helper()
	var count atomic.Int64
	queryName := fmt.Sprintf("benchmark:sql-statements-query:%p", &count)
	rowName := fmt.Sprintf("benchmark:sql-statements-row:%p", &count)
	db.Callback().Query().Before("gorm:query").Register(queryName, func(*gorm.DB) {
		count.Add(1)
	})
	db.Callback().Row().Before("gorm:row").Register(rowName, func(*gorm.DB) {
		count.Add(1)
	})
	b.Cleanup(func() {
		_ = db.Callback().Query().Remove(queryName)
		_ = db.Callback().Row().Remove(rowName)
	})
	return &count
}
