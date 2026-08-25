package database_test

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ya-breeze/healthvault/pkg/database"
	"gorm.io/gorm"
)

func TestResolveTimezone(t *testing.T) {
	cases := []struct {
		name         string
		settingsJSON string
		want         string // want.String() of the resolved *time.Location
	}{
		{"missing key", `{}`, "UTC"},
		{"empty string", `{"timezone":""}`, "UTC"},
		{"invalid zone name", `{"timezone":"Not/A_Zone"}`, "UTC"},
		{"valid zone name", `{"timezone":"America/Los_Angeles"}`, "America/Los_Angeles"},
		{"malformed json", `not json`, "UTC"},
		{"wrong type", `{"timezone":123}`, "UTC"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := database.ResolveTimezone(tc.settingsJSON)
			if got.String() != tc.want {
				t.Errorf("ResolveTimezone(%q) = %v, want %v", tc.settingsJSON, got, tc.want)
			}
		})
	}
}

// A UTC timestamp just after midnight UTC on 2026-08-21 falls on 2026-08-20
// in America/Los_Angeles (UTC-7 in August, DST) — the day-boundary shift
// this whole feature exists to resolve. See design.md §2 "Local Day boundary".
func TestLocalDate_ShiftsAcrossDayBoundaryByZone(t *testing.T) {
	ts := time.Date(2026, 8, 21, 2, 0, 0, 0, time.UTC)

	utcDate := database.LocalDate(ts, time.UTC)
	if utcDate != "2026-08-21" {
		t.Errorf("LocalDate in UTC = %q, want 2026-08-21", utcDate)
	}

	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	laDate := database.LocalDate(ts, loc)
	if laDate != "2026-08-20" {
		t.Errorf("LocalDate in America/Los_Angeles = %q, want 2026-08-20", laDate)
	}
}

func TestResolveUsualMealsPerDay(t *testing.T) {
	cases := []struct {
		name         string
		settingsJSON string
		want         int
	}{
		{"missing key", `{}`, 3},
		{"zero", `{"usual_meals_per_day":0}`, 3},
		{"negative", `{"usual_meals_per_day":-2}`, 3},
		{"non-integer", `{"usual_meals_per_day":2.5}`, 3},
		{"non-numeric type", `{"usual_meals_per_day":"3"}`, 3},
		{"malformed json", `not json`, 3},
		{"valid positive integer", `{"usual_meals_per_day":5}`, 5},
		{"valid integer as whole float", `{"usual_meals_per_day":4.0}`, 4},
		// int(f) for a float64 outside int range is implementation-defined
		// (on amd64, it produces a large negative number) — must fall back to
		// the default rather than yield a threshold that makes every
		// nonzero-occasion day compute as "complete".
		{"overflows int on conversion", `{"usual_meals_per_day":1e100}`, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := database.ResolveUsualMealsPerDay(tc.settingsJSON)
			if got != tc.want {
				t.Errorf("ResolveUsualMealsPerDay(%q) = %d, want %d", tc.settingsJSON, got, tc.want)
			}
		})
	}
}

func TestSettingsRawString(t *testing.T) {
	cases := []struct {
		name         string
		settingsJSON string
		key          string
		want         string
	}{
		{"missing key", `{}`, "timezone", ""},
		{"present string", `{"timezone":"Europe/Warsaw"}`, "timezone", "Europe/Warsaw"},
		{"wrong type", `{"timezone":42}`, "timezone", ""},
		{"malformed json", `not json`, "timezone", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := database.SettingsRawString(tc.settingsJSON, tc.key)
			if got != tc.want {
				t.Errorf("SettingsRawString(%q, %q) = %q, want %q", tc.settingsJSON, tc.key, got, tc.want)
			}
		})
	}
}

func TestComputeDayState(t *testing.T) {
	cases := []struct {
		name          string
		occasionCount int
		threshold     int
		confirmed     bool
		want          string
	}{
		{"zero occasions, unconfirmed", 0, 3, false, database.DayStateIncomplete},
		{"zero occasions, confirmed (stale row ignored)", 0, 3, true, database.DayStateIncomplete},
		{"below threshold, unconfirmed", 2, 3, false, database.DayStateUnconfirmed},
		{"below threshold, confirmed", 2, 3, true, database.DayStateConfirmedComplete},
		{"at threshold, unconfirmed", 3, 3, false, database.DayStateComplete},
		{"at threshold, confirmed (still Complete, not ConfirmedComplete)", 3, 3, true, database.DayStateComplete},
		{"above threshold, unconfirmed", 5, 3, false, database.DayStateComplete},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := database.ComputeDayState(tc.occasionCount, tc.threshold, tc.confirmed)
			if got != tc.want {
				t.Errorf(
					"ComputeDayState(%d, %d, %v) = %q, want %q",
					tc.occasionCount, tc.threshold, tc.confirmed, got, tc.want,
				)
			}
		})
	}
}

func newCompletenessTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := database.Open(slog.New(slog.NewTextHandler(os.Stderr, nil)), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return db
}

func mustCreateMeal(t *testing.T, db *gorm.DB, userID, familyID uuid.UUID, loggedAt time.Time) {
	t.Helper()
	m := database.FoodMeal{UserID: userID, Status: database.MealStatusConfirmed, LoggedAt: loggedAt}
	m.ID = uuid.New()
	m.FamilyID = familyID
	if err := db.Create(&m).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}
}

func mustCreateConfirmation(t *testing.T, db *gorm.DB, userID, familyID uuid.UUID, localDate string) {
	t.Helper()
	c := database.FoodDayCompletion{UserID: userID, LocalDate: localDate, ConfirmedAt: time.Now()}
	c.ID = uuid.New()
	c.FamilyID = familyID
	if err := db.Create(&c).Error; err != nil {
		t.Fatalf("create confirmation: %v", err)
	}
}

// TestDayRange covers each state, plus the stale-confirmation cleanup side
// effect, across a single 4-day range (design.md §3, tasks.md 3.5):
//
//   - 2026-01-01: 0 meals but a stray confirmation row -> Incomplete, and
//     the stray row is hard-deleted as a side effect.
//   - 2026-01-02: 2 occasions (below threshold 3), no confirmation -> Unconfirmed.
//   - 2026-01-03: 2 occasions, confirmed -> Confirmed Complete.
//   - 2026-01-04: 3 occasions (at threshold) plus a stray confirmation ->
//     Complete, not Confirmed Complete (occasion count wins).
func TestDayRange(t *testing.T) {
	db := newCompletenessTestDB(t)
	userID, familyID := uuid.New(), uuid.New()
	otherUserID := uuid.New()

	day := func(y int, m time.Month, d, hh int) time.Time {
		return time.Date(y, m, d, hh, 0, 0, 0, time.UTC)
	}

	// 2026-01-01: no meals, only a stale confirmation.
	mustCreateConfirmation(t, db, userID, familyID, "2026-01-01")

	// 2026-01-02: two occasions, well outside the 10-minute merge window.
	mustCreateMeal(t, db, userID, familyID, day(2026, 1, 2, 8))
	mustCreateMeal(t, db, userID, familyID, day(2026, 1, 2, 12))

	// 2026-01-03: two occasions, confirmed.
	mustCreateMeal(t, db, userID, familyID, day(2026, 1, 3, 8))
	mustCreateMeal(t, db, userID, familyID, day(2026, 1, 3, 12))
	mustCreateConfirmation(t, db, userID, familyID, "2026-01-03")

	// 2026-01-04: three occasions (meets threshold), plus a stray confirmation.
	mustCreateMeal(t, db, userID, familyID, day(2026, 1, 4, 8))
	mustCreateMeal(t, db, userID, familyID, day(2026, 1, 4, 12))
	mustCreateMeal(t, db, userID, familyID, day(2026, 1, 4, 18))
	mustCreateConfirmation(t, db, userID, familyID, "2026-01-04")

	// Another user's meal/confirmation on the same dates must not leak in.
	mustCreateMeal(t, db, otherUserID, familyID, day(2026, 1, 2, 8))
	mustCreateConfirmation(t, db, otherUserID, familyID, "2026-01-02")

	got, err := database.DayRange(db, userID, time.UTC, 3, "2026-01-01", "2026-01-04")
	if err != nil {
		t.Fatalf("DayRange: %v", err)
	}

	want := []database.DayCompleteness{
		{Date: "2026-01-01", OccasionCount: 0, State: database.DayStateIncomplete},
		{Date: "2026-01-02", OccasionCount: 2, State: database.DayStateUnconfirmed},
		{Date: "2026-01-03", OccasionCount: 2, State: database.DayStateConfirmedComplete},
		{Date: "2026-01-04", OccasionCount: 3, State: database.DayStateComplete},
	}
	if len(got) != len(want) {
		t.Fatalf("DayRange returned %d entries, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("entry %d = %+v, want %+v", i, got[i], w)
		}
	}

	// The stale 2026-01-01 confirmation must be hard-deleted, not just
	// excluded from the result — a plain (soft) delete would permanently
	// occupy the (user, date) unique slot and block re-confirming it.
	var staleCount int64
	if err := db.Unscoped().Model(&database.FoodDayCompletion{}).
		Where("user_id = ? AND local_date = ?", userID, "2026-01-01").
		Count(&staleCount).Error; err != nil {
		t.Fatalf("count stale confirmations: %v", err)
	}
	if staleCount != 0 {
		t.Errorf("stale confirmation row for 2026-01-01 still present (even Unscoped): count=%d", staleCount)
	}

	// The valid 2026-01-03/04 confirmations must survive untouched.
	var survivingCount int64
	if err := db.Model(&database.FoodDayCompletion{}).
		Where("user_id = ? AND local_date IN ?", userID, []string{"2026-01-03", "2026-01-04"}).
		Count(&survivingCount).Error; err != nil {
		t.Fatalf("count surviving confirmations: %v", err)
	}
	if survivingCount != 2 {
		t.Errorf("want 2 surviving confirmations, got %d", survivingCount)
	}
}

// TestDayRange_ThresholdRecomputedEachCall confirms usual_meals_per_day is
// read fresh on every call rather than snapshotted anywhere — the same
// day's data resolves to different states under different thresholds
// (design.md §3, "Settling open question #3 (retroactivity)").
func TestDayRange_ThresholdRecomputedEachCall(t *testing.T) {
	db := newCompletenessTestDB(t)
	userID, familyID := uuid.New(), uuid.New()

	mustCreateMeal(t, db, userID, familyID, time.Date(2026, 1, 2, 8, 0, 0, 0, time.UTC))
	mustCreateMeal(t, db, userID, familyID, time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC))

	lowThreshold, err := database.DayRange(db, userID, time.UTC, 2, "2026-01-02", "2026-01-02")
	if err != nil {
		t.Fatalf("DayRange (threshold 2): %v", err)
	}
	if len(lowThreshold) != 1 || lowThreshold[0].State != database.DayStateComplete {
		t.Errorf("threshold 2: got %+v, want state complete", lowThreshold)
	}

	highThreshold, err := database.DayRange(db, userID, time.UTC, 3, "2026-01-02", "2026-01-02")
	if err != nil {
		t.Fatalf("DayRange (threshold 3): %v", err)
	}
	if len(highThreshold) != 1 || highThreshold[0].State != database.DayStateUnconfirmed {
		t.Errorf("threshold 3: got %+v, want state unconfirmed", highThreshold)
	}
}

// TestDayRange_NonUTCLocationFindsMeals is a regression test: FoodMeal.LoggedAt
// is always stored UTC-normalized (backfillFoodMealLoggedAtToUTC, db.go), and
// go-sqlite3 stores time.Time as TEXT preserving whatever offset it's given
// rather than normalizing it, so SQLite compares that column as text, not as
// an instant. Building the day-window bounds from time.ParseInLocation(...,
// loc) with a non-UTC loc (e.g. America/Los_Angeles) and passing them
// straight into the query previously produced bounds with a non-zero offset
// that string-compared incorrectly against the UTC-offset stored rows,
// silently returning zero meals for a real day even though the underlying
// instants were ordered correctly.
func TestDayRange_NonUTCLocationFindsMeals(t *testing.T) {
	db := newCompletenessTestDB(t)
	userID, familyID := uuid.New(), uuid.New()

	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	// 2026-08-21T02:00:00Z is 2026-08-20T19:00:00-07:00 in America/Los_Angeles.
	ts := time.Date(2026, 8, 21, 2, 0, 0, 0, time.UTC)
	mustCreateMeal(t, db, userID, familyID, ts)
	mustCreateMeal(t, db, userID, familyID, ts.Add(2*time.Hour))

	got, err := database.DayRange(db, userID, loc, 2, "2026-08-20", "2026-08-20")
	if err != nil {
		t.Fatalf("DayRange: %v", err)
	}
	if len(got) != 1 || got[0].OccasionCount != 2 || got[0].State != database.DayStateComplete {
		t.Errorf("got %+v, want a single complete/2-occasion day for 2026-08-20", got)
	}
}
