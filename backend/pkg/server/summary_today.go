package server

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ya-breeze/healthvault/pkg/database"
)

// summaryTargetPayload is the `target` field of summaryTodayResponse
// (design.md §1): either the computed nutrition target, or, when a
// precondition isn't met, `{"available": false, "reason": "<code>"}` — one of
// nutrition-target's existing four reason codes. Never a 422 on this
// endpoint: an unavailable target is a normal, expected state here (a fresh
// user with no profile yet), not an error.
// The five numeric fields deliberately carry no `omitempty`, unlike `Reason`.
// `omitempty` drops an int that is zero, and zero is a legitimate target
// value: computeNutritionTarget clamps carbs to zero whenever protein and the
// fat floor already exhaust the calorie budget, which a goal weight entered in
// pounds against an ordinary TDEE is enough to reach. Omitting the key then
// makes a *present* target look partial — a client that types the field as
// required (as the frontend's TodaySummaryTarget does) reads `undefined` and
// renders "Carbs 130/NaN g". Absence must mean "no target", which `available`
// already says; it must not double as "the number happened to be zero". BMR
// is included for the same reason: a BMR of zero is not realistic in
// practice, but the rule is "no field is special-cased," not "zero happens
// to be safe here."
type summaryTargetPayload struct {
	Available    bool   `json:"available"`
	Reason       string `json:"reason,omitempty"`
	Calories     int    `json:"calories"`
	ProteinGrams int    `json:"protein_grams"`
	CarbsGrams   int    `json:"carbs_grams"`
	FatGrams     int    `json:"fat_grams"`
	BMR          int    `json:"bmr"`
}

// summaryTodayResponse is the 200 response body for GET /api/summary/today
// (design.md §1). `recommendation` is always null in this change (Phase 4's
// non-goal). `LastLoggedAt` is a pointer so a day with no logged meals yet
// serializes as `null`, per design.md's "or null if none".
type summaryTodayResponse struct {
	Date                 string               `json:"date"`
	CaloriesConsumed     float64              `json:"calories_consumed"`
	ProteinGramsConsumed float64              `json:"protein_grams_consumed"`
	CarbsGramsConsumed   float64              `json:"carbs_grams_consumed"`
	FatGramsConsumed     float64              `json:"fat_grams_consumed"`
	MealCount            int                  `json:"meal_count"`
	LastLoggedAt         *time.Time           `json:"last_logged_at"`
	DisplayLanguage      string               `json:"display_language"`
	Target               summaryTargetPayload `json:"target"`
	Recommendation       any                  `json:"recommendation"`
}

// SummaryTodayHandler computes GET /api/summary/today fresh on every call:
// self-only (no ?user=, matching NutritionTargetHandler's "Self-only"
// precedent), it resolves the caller's timezone the same way
// foodHandlers.callerTimezoneAndThreshold does, computes `now := time.Now().UTC()`
// once (matching NutritionTargetHandler's existing convention) and passes
// that same value into both database.TodaySummary and
// computeUserNutritionTarget so the reported `date` and the aggregated
// window never disagree at a local-midnight boundary (design.md §1).
func SummaryTodayHandler(storage database.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := ClaimsFromCtx(r)
		if claims == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// One settings read for all three things this response needs from it
		// — timezone, profile, display language. The endpoint exists so a
		// widget makes one cheap call instead of three; triplicating the row
		// read inside it would give that back.
		settingsJSON, err := readUserSettingsJSON(storage, claims.UserID)
		if err != nil {
			slog.Error("SummaryTodayHandler: read user settings", "err", err, "user_id", claims.UserID)
			http.Error(w, "query error", http.StatusInternalServerError)
			return
		}
		loc := database.ResolveTimezone(settingsJSON)

		now := time.Now().UTC()

		summary, err := database.TodaySummary(storage.DB(), claims.UserID, loc, now)
		if err != nil {
			writeQueryError(w, "summary today: aggregate meals", err, claims.UserID)
			return
		}

		values, unavailableReason, err := computeNutritionTargetForProfile(
			storage, claims.UserID, now, loc, parseUserProfile(settingsJSON))
		if err != nil {
			writeQueryError(w, "summary today: compute nutrition target", err, claims.UserID)
			return
		}
		target := summaryTargetPayload{Available: unavailableReason == ""}
		if unavailableReason != "" {
			target.Reason = unavailableReason
		} else {
			target.Calories = values.Calories
			target.ProteinGrams = values.ProteinGrams
			target.CarbsGrams = values.CarbsGrams
			target.FatGrams = values.FatGrams
			target.BMR = values.BMR
		}

		var lastLoggedAt *time.Time
		if summary.HasLastLoggedAt {
			lastLoggedAt = &summary.LastLoggedAt
		}

		writeJSON(w, summaryTodayResponse{
			Date:                 summary.Date,
			CaloriesConsumed:     summary.CaloriesConsumed,
			ProteinGramsConsumed: summary.ProteinGramsConsumed,
			CarbsGramsConsumed:   summary.CarbsGramsConsumed,
			FatGramsConsumed:     summary.FatGramsConsumed,
			MealCount:            summary.MealCount,
			LastLoggedAt:         lastLoggedAt,
			DisplayLanguage:      displayLanguageFromSettings(settingsJSON),
			Target:               target,
			Recommendation:       nil,
		})
	}
}

// readUserSettingsJSON reads userID's settings blob with the same "no
// settings row yet is the ordinary case" handling
// foodHandlers.callerTimezoneAndThreshold uses: a missing row yields an empty
// blob, which every interpreter below turns into its documented default (UTC
// for the timezone, an empty profile, the default display language), since a
// user who has never opened settings is normal rather than a failure. Any
// other read error is propagated to the caller to turn into a 500.
func readUserSettingsJSON(storage database.Storage, userID uuid.UUID) (string, error) {
	settingsJSON, err := storage.GetUserSettings(userID)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return "", err
		}
		return "", nil
	}
	return settingsJSON, nil
}
