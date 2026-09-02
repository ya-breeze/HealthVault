package server

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	kinmodels "github.com/ya-breeze/kin-core/models"

	"github.com/ya-breeze/healthvault/pkg/database"
)

func newActivityDSTTestStorage(t *testing.T) database.Storage {
	t.Helper()
	db, err := database.Open(slog.New(slog.NewTextHandler(io.Discard, nil)), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return database.NewStorage(db)
}

func seedActivityDSTUserAndFamily(t *testing.T, s database.Storage) (userID, familyID uuid.UUID) {
	t.Helper()
	familyID = uuid.New()
	userID = uuid.New()
	if err := s.DB().Create(&kinmodels.Family{ID: familyID, Name: "TestFamily"}).Error; err != nil {
		t.Fatalf("create family: %v", err)
	}
	user := kinmodels.User{ID: userID, Username: "activitydsttestuser", PasswordHash: "x", FamilyID: familyID}
	if err := s.DB().Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return userID, familyID
}

// fetchDailySteps + trailingStepsAverage must count each local calendar day
// in the 28-day trailing window exactly once, even when the window straddles
// a DST transition. America/Los_Angeles falls back on 2026-11-01, giving
// that local day 25 real hours — a day that a naive UTC-based walk could
// double-count or (on the spring-forward side) skip entirely.
func TestFetchDailySteps_TrailingWindowCrossesFallBackDST(t *testing.T) {
	s := newActivityDSTTestStorage(t)
	userID, familyID := seedActivityDSTUserAndFamily(t, s)

	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}

	// now: local noon on 2026-11-15, so the 28-day trailing window
	// (today-1 .. today-28) is 2026-10-18 through 2026-11-14 — squarely
	// spanning the 2026-11-01 fall-back transition.
	now := time.Date(2026, time.November, 15, 12, 0, 0, 0, loc)

	const stepsPerDay = 6000
	for i := 1; i <= trailingWindowDays; i++ {
		day := time.Date(2026, time.November, 15, 0, 0, 0, 0, loc).AddDate(0, 0, -i)
		// Local noon is never ambiguous or skipped across a spring-forward/
		// fall-back transition, unlike the 1am-2am hour DST repeats or skips.
		ts := time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, loc)
		rec := database.Steps{UserID: userID, SourcePayloadID: uuid.New(), StartTime: ts, EndTime: ts, Count: stepsPerDay}
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		if err := s.DB().Create(&rec).Error; err != nil {
			t.Fatalf("create steps: %v", err)
		}
	}

	days, err := fetchDailySteps(s, userID, loc, now)
	if err != nil {
		t.Fatalf("fetchDailySteps: %v", err)
	}
	if len(days) != trailingWindowDays {
		t.Fatalf("expected %d days, got %d: %+v", trailingWindowDays, len(days), days)
	}

	_, todayLabel := localCalendarToday(now, loc)
	avg, validDays := trailingStepsAverage(todayLabel, days)
	if validDays != trailingWindowDays {
		t.Errorf(
			"validDays = %d, want %d (the fall-back day must count exactly once, not be lost or doubled)",
			validDays, trailingWindowDays,
		)
	}
	if avg != stepsPerDay {
		t.Errorf("avg = %v, want %v", avg, float64(stepsPerDay))
	}
}
