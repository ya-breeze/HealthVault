package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"gorm.io/gorm"

	"github.com/ya-breeze/healthvault/pkg/database"
)

const (
	// maxReanalyzeBodyBytes bounds the request body read before decoding, so
	// an oversized payload is rejected before any JSON parsing work.
	maxReanalyzeBodyBytes = 4 * 1024
	// maxHintLength bounds the caller-controlled text forwarded into the
	// vision prompt for that call.
	maxHintLength = 500
)

// reanalyzeEligibleStatuses are the meal statuses Reanalyze can be called
// against. processing and pending_clarification are excluded: the former has
// a call already running (or is a stale remnant Retry already handles), the
// latter has its own clarify flow.
var reanalyzeEligibleStatuses = []string{
	database.MealStatusFailed, database.MealStatusPendingReview, database.MealStatusConfirmed,
}

type reanalyzeRequest struct {
	Hint string `json:"hint"`
}

// Reanalyze handles POST /api/food/meals/{id}/reanalyze: re-runs vision
// recognition on the stored photo with a required free-text hint, replacing
// the meal's items. Eligible from failed, pending_review, or confirmed —
// unlike Retry, which only recovers from failed/stale-processing and takes
// no hint. See design.md "Reanalyze claims the meal atomically" and
// "Reanalyze failure reverts to the meal's prior state" for the rationale.
func (h *foodHandlers) Reanalyze(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromCtx(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	var meal database.FoodMeal
	err = h.storage.DB().Where("id = ? AND user_id = ?", id, claims.UserID).First(&meal).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxReanalyzeBodyBytes)
	var req reanalyzeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	hint := strings.TrimSpace(req.Hint)
	if hint == "" {
		http.Error(w, "hint is required", http.StatusBadRequest)
		return
	}
	if len(hint) > maxHintLength {
		http.Error(w, "hint must be at most 500 characters", http.StatusBadRequest)
		return
	}
	if meal.PhotoPath == "" {
		http.Error(w, "meal has no photo to reanalyze", http.StatusConflict)
		return
	}

	// Captured before the atomic claim below overwrites them, for use on
	// failure — see revertReanalyze.
	priorStatus := meal.Status
	priorClarifyRound := meal.ClarifyRound
	priorClarifyLog := meal.ClarifyLog

	// Atomic claim: the WHERE clause repeats the eligibility check above, but
	// as part of the same conditional UPDATE rather than a prior read, so two
	// requests racing this endpoint for the same meal can't both proceed —
	// only the winner's RowsAffected is non-zero. Resetting clarify_round/log
	// here (not as a separate step) matches RetryMeal: without it, a meal
	// that previously went through clarification could skip questions past
	// the old round cap or mix an old round into the new run.
	claim := h.storage.DB().Model(&database.FoodMeal{}).
		Where("id = ? AND status IN ?", meal.ID, reanalyzeEligibleStatuses).
		Updates(map[string]any{
			"status":        database.MealStatusProcessing,
			"clarify_round": 0,
			"clarify_log":   "",
		})
	if claim.Error != nil {
		http.Error(w, "update error", http.StatusInternalServerError)
		return
	}
	if claim.RowsAffected == 0 {
		http.Error(w, "meal is not eligible for reanalysis", http.StatusConflict)
		return
	}
	meal.Status = database.MealStatusProcessing
	meal.ClarifyRound = 0
	meal.ClarifyLog = ""

	ctx, cancel := context.WithTimeout(r.Context(), h.visionTimeout)
	defer cancel()

	if err := h.runAnalysis(ctx, &meal, hint); err != nil {
		// Non-destructive failure: restore exactly what the claim overwrote.
		// Items and aggregate were never touched — persistAnalysis's
		// delete-and-replace only runs on a successful recognition — so the
		// meal is left exactly as it was found. Responding with 502 (not 200
		// with a mutated meal) lets the caller distinguish "reanalysis
		// failed, nothing changed" from a normal state transition.
		h.revertReanalyze(meal.ID, priorStatus, priorClarifyRound, priorClarifyLog)
		http.Error(w, "reanalysis failed; the meal is unchanged", http.StatusBadGateway)
		return
	}

	writeJSON(w, &meal)
}

// revertReanalyze restores a meal's status/clarify fields to their pre-claim
// values after a failed reanalysis attempt. Safe to run unconditionally:
// this handler is the only writer while the meal's status is processing, by
// construction of the atomic claim in Reanalyze, so there is no concurrent
// write to race against.
func (h *foodHandlers) revertReanalyze(mealID uuid.UUID, status string, clarifyRound int, clarifyLog string) {
	h.storage.DB().Model(&database.FoodMeal{}).Where("id = ?", mealID).
		Updates(map[string]any{
			"status":        status,
			"clarify_round": clarifyRound,
			"clarify_log":   clarifyLog,
		}) //nolint:errcheck
}
