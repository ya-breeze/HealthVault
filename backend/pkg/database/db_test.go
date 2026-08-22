package database_test

import (
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ya-breeze/healthvault/pkg/database"
	kinmodels "github.com/ya-breeze/kin-core/models"
)

func TestOpen(t *testing.T) {
	db, err := database.Open(slog.New(slog.NewTextHandler(os.Stderr, nil)), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	sqlDB, _ := db.DB()
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func newTestStorage(t *testing.T) database.Storage {
	t.Helper()
	db, err := database.Open(slog.New(slog.NewTextHandler(os.Stderr, nil)), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return database.NewStorage(db)
}

func seedUserAndFamily(t *testing.T, s database.Storage) (userID, familyID uuid.UUID) {
	t.Helper()
	familyID = uuid.New()
	userID = uuid.New()
	family := kinmodels.Family{ID: familyID, Name: "TestFamily"}
	if err := s.DB().Create(&family).Error; err != nil {
		t.Fatalf("create family: %v", err)
	}
	user := kinmodels.User{ID: userID, Username: "testuser", PasswordHash: "x", FamilyID: familyID}
	if err := s.DB().Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return userID, familyID
}

func TestDeleteRecord_OwnRecord(t *testing.T) {
	s := newTestStorage(t)
	userID, familyID := seedUserAndFamily(t, s)

	payloadID := uuid.New()
	rec := database.Weight{
		UserID:          userID,
		SourcePayloadID: &payloadID,
		Time:            time.Now(),
		Kilograms:       70.0,
	}
	rec.ID = uuid.New()
	rec.FamilyID = familyID
	if err := s.DB().Create(&rec).Error; err != nil {
		t.Fatalf("create weight: %v", err)
	}

	if err := s.DeleteRecord("weights", rec.ID, userID); err != nil {
		t.Fatalf("DeleteRecord: %v", err)
	}

	// Confirm row is gone.
	var count int64
	s.DB().Table("weights").Where("id = ?", rec.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 rows after delete, got %d", count)
	}
}

func TestDeleteRecord_OtherUsersRecord(t *testing.T) {
	s := newTestStorage(t)
	userID, familyID := seedUserAndFamily(t, s)
	otherUserID := uuid.New()

	payloadID := uuid.New()
	rec := database.Weight{
		UserID:          userID,
		SourcePayloadID: &payloadID,
		Time:            time.Now(),
		Kilograms:       70.0,
	}
	rec.ID = uuid.New()
	rec.FamilyID = familyID
	if err := s.DB().Create(&rec).Error; err != nil {
		t.Fatalf("create weight: %v", err)
	}

	err := s.DeleteRecord("weights", rec.ID, otherUserID)
	if !errors.Is(err, database.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	// Confirm row still exists.
	var count int64
	s.DB().Table("weights").Where("id = ?", rec.ID).Count(&count)
	if count != 1 {
		t.Errorf("expected row to still exist, got count=%d", count)
	}
}

func TestDeleteRecord_NonExistentID(t *testing.T) {
	s := newTestStorage(t)
	userID, _ := seedUserAndFamily(t, s)

	err := s.DeleteRecord("weights", uuid.New(), userID)
	if !errors.Is(err, database.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestDeleteRecord_FoodMealCascadesItems(t *testing.T) {
	s := newTestStorage(t)
	userID, familyID := seedUserAndFamily(t, s)

	meal := database.FoodMeal{UserID: userID, Status: database.MealStatusConfirmed, LoggedAt: time.Now()}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := s.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}

	item := database.FoodItem{UserID: userID, MealID: meal.ID, Name: "Rice", WeightGrams: 100}
	item.ID = uuid.New()
	item.FamilyID = familyID
	if err := s.DB().Create(&item).Error; err != nil {
		t.Fatalf("create item: %v", err)
	}

	if err := s.DeleteRecord("food_meals", meal.ID, userID); err != nil {
		t.Fatalf("DeleteRecord: %v", err)
	}

	var count int64
	s.DB().Table("food_items").Where("meal_id = ?", meal.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected food_items to be cascade-deleted, got %d rows", count)
	}
}

func TestQueryRecords_FoodMealColumnAllowlist(t *testing.T) {
	s := newTestStorage(t)
	userID, familyID := seedUserAndFamily(t, s)

	meal := database.FoodMeal{
		UserID: userID, Status: database.MealStatusConfirmed, LoggedAt: time.Now(),
		Name: "Lunch", PhotoPath: "secret/path.jpg", RawResponse: `{"raw":"llm output"}`,
	}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := s.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}

	results, err := s.QueryRecords("food_meals", "logged_at", userID,
		database.TimeRange{From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatalf("QueryRecords: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if _, ok := results[0]["photo_path"]; ok {
		t.Error("expected photo_path to be excluded from results")
	}
	if _, ok := results[0]["raw_response"]; ok {
		t.Error("expected raw_response to be excluded from results")
	}
	if results[0]["name"] != "Lunch" {
		t.Errorf("expected name=Lunch, got %v", results[0]["name"])
	}
}

func TestDeleteRecord_SleepCascadesStages(t *testing.T) {
	s := newTestStorage(t)
	userID, familyID := seedUserAndFamily(t, s)

	sleep := database.Sleep{
		UserID:          userID,
		SourcePayloadID: uuid.New(),
		StartTime:       time.Now().Add(-8 * time.Hour),
		SessionEndTime:  time.Now(),
		DurationSeconds: 28800,
	}
	sleep.ID = uuid.New()
	sleep.FamilyID = familyID
	if err := s.DB().Create(&sleep).Error; err != nil {
		t.Fatalf("create sleep: %v", err)
	}

	stage := database.SleepStage{
		SleepID:         sleep.ID,
		Stage:           "deep",
		StartTime:       sleep.StartTime,
		EndTime:         sleep.SessionEndTime,
		DurationSeconds: 28800,
	}
	if err := s.DB().Create(&stage).Error; err != nil {
		t.Fatalf("create sleep_stage: %v", err)
	}

	if err := s.DeleteRecord("sleeps", sleep.ID, userID); err != nil {
		t.Fatalf("DeleteRecord: %v", err)
	}

	var count int64
	s.DB().Table("sleep_stages").Where("sleep_id = ?", sleep.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected sleep_stages to be cascade-deleted, got %d rows", count)
	}
}

// Regression (round 10 review, updated_at check added round 11): normalizing
// logged_at to UTC only on the write paths (CreateManualMeal, ConfirmMeal,
// PatchMeal) does nothing for a row that was already stored with a non-UTC
// offset before that fix existed — AutoMigrate does not rewrite existing
// data, and the API itself never restricted callers to UTC. This simulates
// such a legacy row by writing directly with a fixed-offset time.Time
// (bypassing every .UTC()-calling handler, the same way a pre-fix row or a
// direct API caller not using this frontend would have been stored), then
// reopens the same on-disk database — which is where Open's backfill runs —
// and checks the stored value is now UTC while still representing the same
// instant. Also asserts updated_at is untouched: it's the analysis lease
// token Reanalyze/Retry/ClarifyMeal claim against and RetryMeal's
// stale-processing clock, so a migration that silently refreshes it would
// give an ownerless processing meal a fresh-looking lease, blocking Retry
// for no reason tied to any real attempt.
func TestOpen_BackfillsExistingNonUTCLoggedAt(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	db, err := database.Open(logger, dbPath)
	if err != nil {
		t.Fatalf("Open (initial): %v", err)
	}
	familyID := uuid.New()
	userID := uuid.New()
	if err := db.Create(&kinmodels.Family{ID: familyID, Name: "TestFamily"}).Error; err != nil {
		t.Fatalf("create family: %v", err)
	}
	if err := db.Create(&kinmodels.User{ID: userID, Username: "legacyuser", PasswordHash: "x", FamilyID: familyID}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	// A fixed -08:00 offset, not UTC — as a pre-fix write or a direct API
	// caller could have stored. Status processing, as an ownerless
	// crashed-worker meal would be — the case the updated_at check below
	// specifically protects.
	nonUTC := time.Date(2024, 1, 1, 10, 0, 0, 0, time.FixedZone("", -8*60*60))
	meal := database.FoodMeal{UserID: userID, Status: database.MealStatusProcessing, LoggedAt: nonUTC, Name: "Legacy Meal"}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := db.Create(&meal).Error; err != nil {
		t.Fatalf("create legacy meal: %v", err)
	}
	originalUpdatedAt := meal.UpdatedAt
	sqlDB, _ := db.DB()
	sqlDB.Close() //nolint:errcheck

	// Reopening the same file is where the backfill runs.
	db2, err := database.Open(logger, dbPath)
	if err != nil {
		t.Fatalf("Open (reopen): %v", err)
	}
	var reloaded database.FoodMeal
	if err := db2.Select("id", "logged_at", "updated_at").Where("id = ?", meal.ID).First(&reloaded).Error; err != nil {
		t.Fatalf("reload meal: %v", err)
	}
	if _, offset := reloaded.LoggedAt.Zone(); offset != 0 {
		t.Errorf("expected logged_at to be backfilled to a zero UTC offset, got offset=%d", offset)
	}
	if !reloaded.LoggedAt.Equal(nonUTC) {
		t.Errorf("expected the backfill to preserve the same instant, got %v want %v", reloaded.LoggedAt, nonUTC)
	}
	if !reloaded.UpdatedAt.Equal(originalUpdatedAt) {
		t.Errorf("expected the backfill to leave updated_at untouched (it's the analysis lease token), got %v want %v",
			reloaded.UpdatedAt, originalUpdatedAt)
	}
}

func TestAllTablesCreated(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tmpDir := t.TempDir()
	db, err := database.Open(logger, tmpDir+"/test.db")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	tables := []string{
		"families", "users", "blacklisted_tokens", "refresh_tokens",
		"webhook_payloads",
		"steps", "heart_rates", "heart_rate_variabilities", "sleeps", "sleep_stages",
		"blood_pressures", "distances", "active_calories", "total_calories", "weights",
		"heights", "blood_glucoses", "oxygen_saturations", "body_temperatures",
		"skin_temperatures", "respiratory_rates", "resting_heart_rates", "exercises",
		"hydrations", "nutritions", "basal_metabolic_rates", "body_fats", "lean_body_masses",
		"vo2_maxes", "bone_masses",
		"speeds",
	}
	for _, tbl := range tables {
		if !db.Migrator().HasTable(tbl) {
			t.Errorf("missing table: %s", tbl)
		}
	}
}
