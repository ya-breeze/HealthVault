package database_test

import (
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	kinmodels "github.com/ya-breeze/kin-core/models"
	"github.com/ya-breeze/healthvault/pkg/database"
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

	rec := database.Weight{
		UserID:          userID,
		SourcePayloadID: uuid.New(),
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

	rec := database.Weight{
		UserID:          userID,
		SourcePayloadID: uuid.New(),
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
