package database

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/ya-breeze/kin-core/models"
	"gorm.io/gorm"
)

// FoodDayCompletion is the user's assertion that a partially-logged day (an
// Unconfirmed day, per the food-day-completeness capability) is actually
// complete. Row presence is the assertion; deleting the row retracts it.
// Not a metric type: this is a flag on a date, not a time-series
// measurement, and doesn't belong in typeRegistry. See design.md §4
// "Storage" under openspec/changes/food-day-completeness.
type FoodDayCompletion struct {
	models.TenantModel
	UserID      uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_food_day_completion_user_date"`
	LocalDate   string    `gorm:"type:varchar(10);not null;uniqueIndex:idx_food_day_completion_user_date"` // YYYY-MM-DD in the user's stored timezone at confirm time
	ConfirmedAt time.Time `gorm:"not null"`
}

// defaultUsualMealsPerDay is the fallback threshold when a user has no
// usable usual_meals_per_day setting (design.md §3 "Day Completeness states").
const defaultUsualMealsPerDay = 3

// SettingsRawString extracts the raw string value for key from an opaque
// UserSettings JSON blob, returning "" if the document doesn't parse, the
// key is absent, or its value isn't a string.
//
// Exported so callers that need the *raw* stored value rather than a
// resolved/defaulted one (the PUT /api/users/me/settings handler's
// timezone-change comparison, see design.md §4 "Storage") can reuse the
// same parsing this file's own resolvers use, rather than re-implementing
// it. That comparison is deliberately on the raw string, not on a resolved
// *time.Location: two different raw values can resolve to the same
// effective zone (e.g. an invalid string and "" both resolve to UTC), and
// the design intentionally still treats that as "changed" — see the
// "Risks / Trade-offs" note on why a first-time explicit save of "UTC"
// clears confirmations even though the effective zone didn't change.
func SettingsRawString(settingsJSON, key string) string {
	var obj map[string]any
	if err := json.Unmarshal([]byte(settingsJSON), &obj); err != nil {
		return ""
	}
	v, ok := obj[key].(string)
	if !ok {
		return ""
	}
	return v
}

// ResolveTimezone parses the "timezone" key from an opaque UserSettings
// JSON blob into an IANA *time.Location, falling back to UTC when the key
// is missing, empty, or names a zone time.LoadLocation rejects. Never
// itself returns an error — fail open, matching the "presence" endpoint's
// precedent of never hard-failing a read over a malformed per-user
// preference (design.md §2 "Local Day boundary").
func ResolveTimezone(settingsJSON string) *time.Location {
	tz := SettingsRawString(settingsJSON, "timezone")
	if tz == "" {
		return time.UTC
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.UTC
	}
	return loc
}

// ResolveUsualMealsPerDay parses the "usual_meals_per_day" key from an
// opaque UserSettings JSON blob, falling back to 3 when the key is missing,
// not a whole number, or not positive (design.md §3 "Day Completeness
// states").
func ResolveUsualMealsPerDay(settingsJSON string) int {
	var obj map[string]any
	if err := json.Unmarshal([]byte(settingsJSON), &obj); err != nil {
		return defaultUsualMealsPerDay
	}
	v, ok := obj["usual_meals_per_day"]
	if !ok {
		return defaultUsualMealsPerDay
	}
	// encoding/json decodes every JSON number into map[string]any as float64.
	// The upper bound guards against converting an out-of-int-range value
	// (e.g. 1e100) to int below — that conversion is implementation-defined
	// per the Go spec and on amd64 produces a large negative number, which
	// would make every nonzero-occasion day compute as "complete" (threshold
	// negative, so occasionCount >= threshold trivially holds). No realistic
	// threshold approaches this bound, so the cap never affects a real value.
	f, ok := v.(float64)
	if !ok || f != math.Trunc(f) || f < 1 || f > math.MaxInt32 {
		return defaultUsualMealsPerDay
	}
	return int(f)
}

// LocalDate formats t as YYYY-MM-DD in loc — the Logged Day / Local Day
// boundary computation shared by every completeness code path (design.md
// §2 "Local Day boundary").
func LocalDate(t time.Time, loc *time.Location) string {
	return t.In(loc).Format("2006-01-02")
}

// Day Completeness states (design.md §3 "Day Completeness states").
const (
	DayStateComplete          = "complete"
	DayStateConfirmedComplete = "confirmed_complete"
	DayStateUnconfirmed       = "unconfirmed"
	DayStateIncomplete        = "incomplete"
)

// ComputeDayState maps an Eating Occasion count, the caller's
// usual_meals_per_day threshold, and whether the day has a
// FoodDayCompletion row to one of the four Day Completeness states, per the
// table in design.md §3. A day at or above threshold is always Complete,
// even if a stray confirmation row exists for it — occasionCount is
// checked before confirmed.
func ComputeDayState(occasionCount, threshold int, confirmed bool) string {
	if occasionCount == 0 {
		return DayStateIncomplete
	}
	if occasionCount >= threshold {
		return DayStateComplete
	}
	if confirmed {
		return DayStateConfirmedComplete
	}
	return DayStateUnconfirmed
}

// DayCompleteness is one day's entry in a completeness range-query result
// (design.md §5 "API").
type DayCompleteness struct {
	Date          string `json:"date"`
	OccasionCount int    `json:"occasion_count"`
	State         string `json:"state"`
}

// DayRange computes, for userID across the inclusive Logged-Day range
// [from, to] (both YYYY-MM-DD strings in loc — the caller is responsible
// for clamping `to` to exclude today before calling this, per design.md §5
// "API"), one DayCompleteness entry per day: occasion count (via
// CollapseOccasions over that day's FoodMeal.LoggedAt values) and state
// (via ComputeDayState, using threshold and any existing FoodDayCompletion
// row for that day).
//
// As a side effect, hard-deletes (Unscoped) any FoodDayCompletion row for a
// day whose occasion count comes back 0 — e.g. every meal on that day was
// since deleted, or moved off the date via a logged_at edit — so a later
// unrelated meal on that date doesn't silently inherit the old
// confirmation (design.md §3, tasks.md 3.4).
func DayRange(
	db *gorm.DB, userID uuid.UUID, loc *time.Location, threshold int, from, to string,
) ([]DayCompleteness, error) {
	fromDate, err := time.ParseInLocation("2006-01-02", from, loc)
	if err != nil {
		return nil, fmt.Errorf("parse from date %q: %w", from, err)
	}
	toDate, err := time.ParseInLocation("2006-01-02", to, loc)
	if err != nil {
		return nil, fmt.Errorf("parse to date %q: %w", to, err)
	}
	if toDate.Before(fromDate) {
		return nil, nil
	}

	// Exclusive upper bound: the instant the day after `to` begins in loc.
	// UTC-normalized before querying: FoodMeal.LoggedAt is always stored
	// UTC-normalized (backfillFoodMealLoggedAtToUTC, db.go), and go-sqlite3
	// stores time.Time as TEXT preserving whatever offset it's given rather
	// than normalizing it, so SQLite compares that column as text, not as
	// an instant. A non-UTC-offset window bound (e.g. `loc` = a zone with a
	// non-zero offset) would then string-compare incorrectly against the
	// UTC-offset stored rows even when the underlying instants are ordered
	// correctly — silently returning zero meals for a real day.
	windowStart, windowEnd := fromDate.UTC(), toDate.AddDate(0, 0, 1).UTC()

	var meals []FoodMeal
	if err := db.Select("logged_at").
		Where("user_id = ? AND logged_at >= ? AND logged_at < ?", userID, windowStart, windowEnd).
		Find(&meals).Error; err != nil {
		return nil, fmt.Errorf("query meals: %w", err)
	}
	loggedAtByDate := make(map[string][]time.Time, len(meals))
	for _, m := range meals {
		d := LocalDate(m.LoggedAt, loc)
		loggedAtByDate[d] = append(loggedAtByDate[d], m.LoggedAt)
	}

	var confirmations []FoodDayCompletion
	if err := db.Where("user_id = ? AND local_date >= ? AND local_date <= ?", userID, from, to).
		Find(&confirmations).Error; err != nil {
		return nil, fmt.Errorf("query confirmations: %w", err)
	}
	confirmedDates := make(map[string]bool, len(confirmations))
	for _, c := range confirmations {
		confirmedDates[c.LocalDate] = true
	}

	var staleDates []string
	result := make([]DayCompleteness, 0, int(toDate.Sub(fromDate).Hours()/24)+1)
	for d := fromDate; !d.After(toDate); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("2006-01-02")
		occasionCount := CollapseOccasions(loggedAtByDate[dateStr])
		confirmed := confirmedDates[dateStr]
		if occasionCount == 0 && confirmed {
			staleDates = append(staleDates, dateStr)
		}
		result = append(result, DayCompleteness{
			Date:          dateStr,
			OccasionCount: occasionCount,
			State:         ComputeDayState(occasionCount, threshold, confirmed),
		})
	}

	if len(staleDates) > 0 {
		if err := db.Unscoped().
			Where("user_id = ? AND local_date IN ?", userID, staleDates).
			Delete(&FoodDayCompletion{}).Error; err != nil {
			return nil, fmt.Errorf("delete stale confirmations: %w", err)
		}
	}

	return result, nil
}

// TodaySummaryRow is one user's aggregated "today" activity for
// GET /api/summary/today (design.md §1): consumed macro sums restricted to
// confirmed meals, a raw row count and max LoggedAt across every status.
// HasLastLoggedAt is false (LastLoggedAt the zero time.Time) when there are
// no rows at all for today.
type TodaySummaryRow struct {
	Date                 string
	CaloriesConsumed     float64
	ProteinGramsConsumed float64
	CarbsGramsConsumed   float64
	FatGramsConsumed     float64
	MealCount            int
	LastLoggedAt         time.Time
	HasLastLoggedAt      bool
}

// TodaySummary computes userID's Local Day (`LocalDate(now, loc)`) and folds
// every FoodMeal row Logged on that day into a TodaySummaryRow: macro sums
// restricted to `status = confirmed` rows (design.md: a processing/
// pending_review/pending_clarification/failed row's macro fields are zero or
// stale, per `Aggregate()` in models_food.go, so summing across all statuses
// would silently show a partial or zero-inflated number), a row count and max
// LoggedAt across every status (design.md: "a processing row still means the
// user just took a photo").
//
// `now` is threaded in explicitly rather than calling time.Now() internally,
// matching computeUserNutritionTarget's pattern, so a caller can pass the
// same `now` into both and keep the reported date and this aggregation
// window consistent with each other at a local-midnight boundary. The UTC
// window bounds are computed the same way DayRange does for a single day —
// required because FoodMeal.LoggedAt is stored UTC-normalized and go-sqlite3
// stores time.Time as TEXT preserving whatever offset it's given, so SQLite
// compares it as text, not as an instant (see DayRange's own comment above).
func TodaySummary(db *gorm.DB, userID uuid.UUID, loc *time.Location, now time.Time) (TodaySummaryRow, error) {
	dateStr := LocalDate(now, loc)
	dayStart, err := time.ParseInLocation("2006-01-02", dateStr, loc)
	if err != nil {
		return TodaySummaryRow{}, fmt.Errorf("parse local date %q: %w", dateStr, err)
	}
	windowStart, windowEnd := dayStart.UTC(), dayStart.AddDate(0, 0, 1).UTC()

	var meals []FoodMeal
	if err := db.Select("status", "calories", "protein_grams", "carbs_grams", "fat_grams", "logged_at").
		Where("user_id = ? AND logged_at >= ? AND logged_at < ?", userID, windowStart, windowEnd).
		Find(&meals).Error; err != nil {
		return TodaySummaryRow{}, fmt.Errorf("query meals: %w", err)
	}

	row := TodaySummaryRow{Date: dateStr}
	for _, m := range meals {
		row.MealCount++
		if m.LoggedAt.After(row.LastLoggedAt) {
			row.LastLoggedAt = m.LoggedAt
		}
		if m.Status == MealStatusConfirmed {
			row.CaloriesConsumed += m.Calories
			row.ProteinGramsConsumed += m.ProteinGrams
			row.CarbsGramsConsumed += m.CarbsGrams
			row.FatGramsConsumed += m.FatGrams
		}
	}
	row.HasLastLoggedAt = row.MealCount > 0

	return row, nil
}
