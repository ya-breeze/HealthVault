package database

import (
	"encoding/json"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/ya-breeze/kin-core/models"
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
	f, ok := v.(float64)
	if !ok || f != math.Trunc(f) || f < 1 {
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
