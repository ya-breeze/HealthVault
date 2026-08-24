package server

import (
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

// nutritionTargetResponse is the 200 response body for
// GET /api/users/me/nutrition-target: the computed target plus every input
// that fed it (design.md: computed on read, never cached, so the caller
// always sees what produced the numbers).
type nutritionTargetResponse struct {
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
// every call (design.md "Computed on read, never stored") from the caller's
// profile, latest weight/height/weight_goal records, and activity tier.
// Unlike summaryHandler it is self-only — no ?user= — since its inputs
// include UserSettings fields that user-settings scopes to the authenticated
// user only (see design.md's "Self-only" decision). Exported for use in
// tests.
func NutritionTargetHandler(storage database.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := ClaimsFromCtx(r)
		if claims == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		profile := readUserProfile(storage, claims.UserID)
		if !profile.HasBirthdate || !profile.HasSex {
			writeUnprocessable(w, "missing_profile")
			return
		}

		weightKg, hasWeight := latestPointValue(storage, "weights", "time", "kilograms", claims.UserID)
		heightM, hasHeight := latestPointValue(storage, "heights", "time", "meters", claims.UserID)
		if !hasWeight || !hasHeight {
			writeUnprocessable(w, "missing_measurements")
			return
		}

		goalWeightKg, hasGoal := latestPointValue(storage, "weight_goals", "time", "kilograms", claims.UserID)
		if !hasGoal {
			writeUnprocessable(w, "missing_goal_weight")
			return
		}

		now := time.Now().UTC()
		tierName, multiplier, ok := resolveActivityTier(storage, claims.UserID, profile, now)
		if !ok {
			writeUnprocessable(w, "insufficient_activity_data")
			return
		}

		ageYears := calendarAge(profile.Birthdate, now)
		calories, proteinGrams, carbsGrams, fatGrams := computeNutritionTarget(
			weightKg, heightM, ageYears, profile.Sex, multiplier, goalWeightKg,
		)

		writeJSON(w, nutritionTargetResponse{
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
		})
	}
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
func resolveActivityTier(
	storage database.Storage, userID uuid.UUID, profile userProfile, today time.Time,
) (string, float64, bool) {
	if profile.HasActivityOverride {
		t := activityOverrideTiers[profile.ActivityOverride]
		return t.Name, t.Multiplier, true
	}
	days, err := fetchDailySteps(storage, userID, today)
	if err != nil {
		return "", 0, false
	}
	tier, ok := inferActivityTier(today, days)
	if !ok {
		return "", 0, false
	}
	return tier.Name, tier.Multiplier, true
}

// fetchDailySteps loads the per-day step sums trailingStepsAverage needs,
// reusing the existing GET /api/data/steps?bucket=day aggregation
// (design.md: "without needing per-record timestamps finer than what
// ...already returns"). The range's upper bound includes today itself; that
// row is harmless here, since trailingStepsAverage never looks up "today" in
// its day map — it only ever walks today-1 through today-28.
func fetchDailySteps(storage database.Storage, userID uuid.UUID, today time.Time) ([]dailySteps, error) {
	today = today.UTC().Truncate(24 * time.Hour)
	tr := database.TimeRange{
		From: today.AddDate(0, 0, -trailingWindowDays),
		To:   today,
	}
	rows, err := storage.QueryAggregate(
		"steps", "start_time", "count", database.AggFamilyCumulative, database.BucketDay, userID, tr,
	)
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

// latestPointValue returns the most recent valueCol reading for userID from
// a point-type table (weights/heights/weight_goals), ordered by timeCol
// descending. ok is false if the user has no rows in the table at all.
func latestPointValue(storage database.Storage, table, timeCol, valueCol string, userID uuid.UUID) (float64, bool) {
	var rows []map[string]any
	err := storage.DB().Table(table).
		Select(valueCol).
		Where("user_id = ?", userID).
		Order(timeCol + " DESC").
		Limit(1).
		Find(&rows).Error
	if err != nil || len(rows) == 0 {
		return 0, false
	}
	return toFloat64(rows[0][valueCol]), true
}

// writeUnprocessable writes the 422 `{"error": "<reason>"}` shape every
// unmet-precondition reason in this endpoint uses (design.md's "one reason
// code per unmet input").
func writeUnprocessable(w http.ResponseWriter, reason string) {
	writeJSONStatus(w, http.StatusUnprocessableEntity, map[string]string{"error": reason})
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
