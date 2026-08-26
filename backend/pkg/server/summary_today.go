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
type summaryTargetPayload struct {
	Available    bool   `json:"available"`
	Reason       string `json:"reason,omitempty"`
	Calories     int    `json:"calories,omitempty"`
	ProteinGrams int    `json:"protein_grams,omitempty"`
	CarbsGrams   int    `json:"carbs_grams,omitempty"`
	FatGrams     int    `json:"fat_grams,omitempty"`
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

		loc, err := callerTimezone(storage, claims.UserID)
		if err != nil {
			slog.Error("SummaryTodayHandler: read user settings", "err", err, "user_id", claims.UserID)
			http.Error(w, "query error", http.StatusInternalServerError)
			return
		}

		now := time.Now().UTC()

		summary, err := database.TodaySummary(storage.DB(), claims.UserID, loc, now)
		if err != nil {
			writeQueryError(w, "summary today: aggregate meals", err, claims.UserID)
			return
		}

		values, unavailableReason, err := computeUserNutritionTarget(storage, claims.UserID, now)
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
			DisplayLanguage:      DisplayLanguage(storage, claims.UserID),
			Target:               target,
			Recommendation:       nil,
		})
	}
}

// callerTimezone resolves userID's timezone from their stored settings, the
// same "no settings row yet is the ordinary case" pattern
// foodHandlers.callerTimezoneAndThreshold uses: a missing row falls back to
// defaults (UTC, via database.ResolveTimezone) rather than surfacing an
// error, since a user who has never opened settings is normal, not a
// failure. Any other read error is propagated to the caller to turn into a
// 500.
func callerTimezone(storage database.Storage, userID uuid.UUID) (*time.Location, error) {
	settingsJSON, err := storage.GetUserSettings(userID)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		settingsJSON = ""
	}
	return database.ResolveTimezone(settingsJSON), nil
}
