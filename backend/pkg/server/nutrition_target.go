package server

import (
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/ya-breeze/healthvault/pkg/database"
)

// Mifflin-St Jeor sex terms and macro-split constants from design.md's
// "Nutrition target computation" decision.
const (
	sexTermMale            = 5.0
	sexTermFemale          = -161.0
	proteinGramsPerKgGoal  = 1.6
	fatFloorGramsPerKgGoal = 0.8
	kcalPerGramProtein     = 4.0
	kcalPerGramCarb        = 4.0
	kcalPerGramFat         = 9.0
)

// nutritionTargetValues is the computed target plus every input that fed it
// (design.md: computed on read, never cached, so the caller always sees what
// produced the numbers). It is both the 200 response body for
// GET /api/users/me/nutrition-target and the value `computeUserNutritionTarget`
// hands to any other caller that needs to embed a target (e.g. the daily
// summary endpoint), so `NutritionTargetHandler`'s response shape and every
// embedder's inputs can never drift apart.
type nutritionTargetValues struct {
	Calories           int     `json:"calories"`
	ProteinGrams       int     `json:"protein_grams"`
	CarbsGrams         int     `json:"carbs_grams"`
	FatGrams           int     `json:"fat_grams"`
	MeasuredWeightKg   float64 `json:"measured_weight_kg"`
	GoalWeightKg       float64 `json:"goal_weight_kg"`
	HeightM            float64 `json:"height_m"`
	AgeYears           int     `json:"age_years"`
	Sex                string  `json:"sex"`
	ActivityMultiplier float64 `json:"activity_multiplier"`
	ActivityTier       string  `json:"activity_tier"`
}

// NutritionTargetHandler computes GET /api/users/me/nutrition-target fresh on
// every call (design.md "Computed on read, never stored"). Unlike
// summaryHandler it is self-only — no ?user= — since its inputs include
// UserSettings fields that user-settings scopes to the authenticated user
// only (see design.md's "Self-only" decision). Exported for use in tests.
func NutritionTargetHandler(storage database.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := ClaimsFromCtx(r)
		if claims == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		now := time.Now().UTC()
		values, unavailableReason, err := computeUserNutritionTarget(storage, claims.UserID, now)
		if err != nil {
			writeQueryError(w, "nutrition target: compute", err, claims.UserID)
			return
		}
		if unavailableReason != "" {
			writeUnprocessable(w, unavailableReason)
			return
		}
		writeJSON(w, values)
	}
}

// computeUserNutritionTarget runs every precondition check
// (`NutritionTargetHandler`'s original body) and, if all are met, the
// Mifflin-St Jeor computation itself, from the caller's profile, latest
// weight/height/weight_goal records, and activity tier. `now` is an explicit
// parameter (not `time.Now()` internally) so callers can pin it — in
// particular so `SummaryTodayHandler` can pass the same `now` it uses for
// `database.TodaySummary`, keeping the reported day and the target computation
// consistent with each other at a local-midnight boundary.
//
// unavailableReason is one of nutrition-target's four reason codes
// (non-empty) when a precondition isn't met — a normal, expected outcome, not
// an error — in which case values is the zero value and err is nil. err is
// non-nil only for a genuine storage failure.
func computeUserNutritionTarget(
	storage database.Storage, userID uuid.UUID, now time.Time,
) (values nutritionTargetValues, unavailableReason string, err error) {
	profile, err := readUserProfile(storage, userID)
	if err != nil {
		return nutritionTargetValues{}, "", fmt.Errorf("read user profile: %w", err)
	}
	return computeNutritionTargetForProfile(storage, userID, now, profile)
}

// computeNutritionTargetForProfile is computeUserNutritionTarget for a caller
// that has already read the settings blob, so the profile is not fetched a
// second time. Same contract otherwise.
func computeNutritionTargetForProfile(
	storage database.Storage, userID uuid.UUID, now time.Time, profile userProfile,
) (values nutritionTargetValues, unavailableReason string, err error) {
	if !profile.HasBirthdate || !profile.HasSex {
		return nutritionTargetValues{}, "missing_profile", nil
	}

	weightKg, hasWeight, err := latestPointValue(storage, "weights", "time", "kilograms", userID, now)
	if err != nil {
		return nutritionTargetValues{}, "", fmt.Errorf("read weight: %w", err)
	}
	heightM, hasHeight, err := latestPointValue(storage, "heights", "time", "meters", userID, now)
	if err != nil {
		return nutritionTargetValues{}, "", fmt.Errorf("read height: %w", err)
	}
	if !hasWeight || !hasHeight {
		return nutritionTargetValues{}, "missing_measurements", nil
	}

	goalWeightKg, hasGoal, err := latestPointValue(storage, "weight_goals", "time", "kilograms", userID, now)
	if err != nil {
		return nutritionTargetValues{}, "", fmt.Errorf("read goal weight: %w", err)
	}
	if !hasGoal {
		return nutritionTargetValues{}, "missing_goal_weight", nil
	}

	tierName, multiplier, ok, err := resolveActivityTier(storage, userID, profile, now)
	if err != nil {
		return nutritionTargetValues{}, "", fmt.Errorf("resolve activity tier: %w", err)
	}
	if !ok {
		return nutritionTargetValues{}, "insufficient_activity_data", nil
	}

	ageYears := calendarAge(profile.Birthdate, now)
	calories, proteinGrams, carbsGrams, fatGrams := computeNutritionTarget(
		weightKg, heightM, ageYears, profile.Sex, multiplier, goalWeightKg,
	)

	return nutritionTargetValues{
		Calories:           roundToInt(calories),
		ProteinGrams:       roundToInt(proteinGrams),
		CarbsGrams:         roundToInt(carbsGrams),
		FatGrams:           roundToInt(fatGrams),
		MeasuredWeightKg:   weightKg,
		GoalWeightKg:       goalWeightKg,
		HeightM:            heightM,
		AgeYears:           ageYears,
		Sex:                profile.Sex,
		ActivityMultiplier: multiplier,
		ActivityTier:       tierName,
	}, "", nil
}

// computeNutritionTarget implements design.md's Mifflin-St Jeor + activity
// multiplier + protein/carb/fat-with-floor formula. Inputs are in the
// formula's native units (height in metres, converted to cm here); outputs
// are unrounded — the caller rounds once, at the response boundary, since
// the fat-floor recomputation of carbs needs the unrounded remaining kcal.
func computeNutritionTarget(
	weightKg, heightM float64, ageYears int, sex string, activityMultiplier, goalWeightKg float64,
) (calories, proteinGrams, carbsGrams, fatGrams float64) {
	sexTerm := sexTermFemale
	if sex == "male" {
		sexTerm = sexTermMale
	}
	heightCm := heightM * 100
	bmr := 10*weightKg + 6.25*heightCm - 5*float64(ageYears) + sexTerm
	calories = bmr * activityMultiplier

	proteinGrams = proteinGramsPerKgGoal * goalWeightKg
	remainingKcal := calories - proteinGrams*kcalPerGramProtein

	carbsGrams = remainingKcal / 2 / kcalPerGramCarb
	fatGrams = remainingKcal / 2 / kcalPerGramFat

	fatFloorGrams := fatFloorGramsPerKgGoal * goalWeightKg
	if fatGrams < fatFloorGrams {
		fatGrams = fatFloorGrams
		carbsGrams = (remainingKcal - fatFloorGrams*kcalPerGramFat) / kcalPerGramCarb
	}
	if carbsGrams < 0 {
		carbsGrams = 0
	}
	return calories, proteinGrams, carbsGrams, fatGrams
}

// resolveActivityTier resolves the caller's activity tier: their
// activity_override if set and valid, else the trailing-steps inference
// (task 1). No blending between the two (design.md/user-profile: "no
// blending between the override and the inferred value").
//
// The returned error is non-nil only for a genuine storage failure while
// fetching step history; ok=false/err=nil means "insufficient data", a
// normal, expected outcome, never a storage failure.
func resolveActivityTier(
	storage database.Storage, userID uuid.UUID, profile userProfile, today time.Time,
) (string, float64, bool, error) {
	if profile.HasActivityOverride {
		t := activityOverrideTiers[profile.ActivityOverride]
		return t.Name, t.Multiplier, true, nil
	}
	days, err := fetchDailySteps(storage, userID, today)
	if err != nil {
		return "", 0, false, err
	}
	tier, ok := inferActivityTier(today, days)
	if !ok {
		return "", 0, false, nil
	}
	return tier.Name, tier.Multiplier, true, nil
}

// fetchDailySteps loads the per-day step sums trailingStepsAverage needs,
// via QueryAggregateSteps rather than the generic ?bucket=day aggregation:
// the generic path is a plain SUM(count) with no overlap handling, and an
// over-counted step history (see check-the-health-data spec) would push the
// inferred Activity Level tier up and inflate the Nutrition Target's
// calorie budget through the multiplier. The range's upper bound includes
// today itself; that row is harmless here, since trailingStepsAverage never
// looks up "today" in its day map — it only ever walks today-1 through
// today-28.
func fetchDailySteps(storage database.Storage, userID uuid.UUID, today time.Time) ([]dailySteps, error) {
	today = today.UTC().Truncate(24 * time.Hour)
	tr := database.TimeRange{
		From: today.AddDate(0, 0, -trailingWindowDays),
		To:   today,
	}
	rows, err := storage.QueryAggregateSteps(database.BucketDay, userID, tr)
	if err != nil {
		return nil, err
	}
	days := make([]dailySteps, 0, len(rows))
	for _, row := range rows {
		date, ok := parseTimeValue(row["bucket_start"])
		if !ok {
			continue
		}
		days = append(days, dailySteps{Date: date, Sum: toFloat64(row["sum"])})
	}
	return days, nil
}

// latestPointValue returns the most recent valueCol reading at or before now
// for userID from a point-type table (weights/heights/weight_goals), ordered
// by timeCol descending. The now bound matches every other read path's
// convention of hiding future-dated records (see api.go's CreateRecordHandler
// and parseTimeRange's default `to`); those rows can still exist, e.g. via
// the Health Connect/Libra import or webhook ingest paths, which don't
// reject future timestamps the way manual entry does. ok is false if the
// user has no matching rows in the table; the returned error is non-nil only
// for a genuine storage failure, never for "no rows" (GORM's Find, unlike
// First, does not error on an empty result).
func latestPointValue(
	storage database.Storage, table, timeCol, valueCol string, userID uuid.UUID, now time.Time,
) (float64, bool, error) {
	var rows []map[string]any
	err := storage.DB().Table(table).
		Select(valueCol).
		Where("user_id = ? AND "+timeCol+" <= ?", userID, now).
		Order(timeCol + " DESC").
		Limit(1).
		Find(&rows).Error
	if err != nil {
		return 0, false, err
	}
	if len(rows) == 0 {
		return 0, false, nil
	}
	return toFloat64(rows[0][valueCol]), true, nil
}

// writeUnprocessable writes the 422 `{"error": "<reason>"}` shape every
// unmet-precondition reason in this endpoint uses (design.md's "one reason
// code per unmet input").
func writeUnprocessable(w http.ResponseWriter, reason string) {
	writeJSONStatus(w, http.StatusUnprocessableEntity, map[string]string{"error": reason})
}

// writeQueryError reports a genuine storage failure the same way the rest
// of this package does (api.go's GetUserSettingsHandler/queryBucketed):
// a logged 500, never folded into one of this endpoint's 422 reasons, which
// are reserved for "the user hasn't set this up yet".
func writeQueryError(w http.ResponseWriter, context string, err error, userID uuid.UUID) {
	slog.Error(context, "err", err, "user_id", userID)
	http.Error(w, "query error", http.StatusInternalServerError)
}

func roundToInt(v float64) int {
	return int(math.Round(v))
}

// derefAny/toFloat64/parseTimeValue normalize the raw driver values
// QueryAggregate and latestPointValue's ad hoc query return (int64, float64,
// string, or []byte depending on SQLite column affinity, sometimes wrapped
// in *interface{}) into usable Go types.
func derefAny(v any) any {
	if p, ok := v.(*interface{}); ok {
		if p == nil {
			return nil
		}
		return *p
	}
	return v
}

func toFloat64(v any) float64 {
	switch n := derefAny(v).(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	default:
		return 0
	}
}

func parseTimeValue(v any) (time.Time, bool) {
	var s string
	switch x := derefAny(v).(type) {
	case string:
		s = x
	case []byte:
		s = string(x)
	default:
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
