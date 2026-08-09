package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

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
	// Count runes, not bytes: the API promises a character limit, and
	// len(string) counts UTF-8 bytes — a non-ASCII hint (e.g. Cyrillic) can
	// be well within 500 characters but well over 500 bytes.
	if utf8.RuneCountInString(hint) > maxHintLength {
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

	eligible := false
	for _, s := range reanalyzeEligibleStatuses {
		if priorStatus == s {
			eligible = true
			break
		}
	}
	if !eligible {
		http.Error(w, "meal is not eligible for reanalysis", http.StatusConflict)
		return
	}

	// Atomic claim on the *exact* status just captured, not "any eligible
	// status": two concurrent requests racing this endpoint for the same
	// meal can't both proceed (only the winner's RowsAffected is non-zero),
	// same as a plain `status IN (...)` claim would give — but an exact
	// match also closes a second race this handler is otherwise exposed to,
	// uniquely among the eligible-from set: ConfirmMeal transitions
	// pending_review -> confirmed, and Reanalyze is eligible from both. If
	// ConfirmMeal committed that transition in the gap between this
	// handler's read above and this claim, a `status IN (...)` claim would
	// still match (confirmed is also eligible) and silently claim the
	// meal — then, on failure, revert to the *stale* pending_review captured
	// before ConfirmMeal ran, discarding a legitimate confirm that happened
	// concurrently. Claiming the exact captured status instead makes that
	// scenario fail the claim (0 rows affected, 409) rather than revert to
	// a value that was never actually current.
	claim := h.storage.DB().Model(&database.FoodMeal{}).
		Where("id = ? AND status = ?", meal.ID, priorStatus).
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
		if revertErr := h.revertReanalyze(meal.ID, priorStatus, priorClarifyRound, priorClarifyLog); revertErr != nil {
			// The revert itself failed: the meal is genuinely stuck in
			// processing, not "unchanged" — say so distinctly rather than
			// claiming a guarantee that no longer holds. RetryMeal's stale-
			// processing recovery remains available once the vision timeout
			// elapses.
			http.Error(w, "reanalysis failed and the meal could not be restored to its prior state", http.StatusInternalServerError)
			return
		}
		http.Error(w, "reanalysis failed; the meal is unchanged", http.StatusBadGateway)
		return
	}

	writeJSON(w, &meal)
}

// revertReanalyze restores a meal's status/clarify fields to their pre-claim
// values after a failed reanalysis attempt. Safe to run unconditionally (no
// WHERE on status): this handler is the only writer while the meal's status
// is processing, by construction of the atomic claim in Reanalyze, so there
// is no concurrent write to race against.
func (h *foodHandlers) revertReanalyze(mealID uuid.UUID, status string, clarifyRound int, clarifyLog string) error {
	return h.storage.DB().Model(&database.FoodMeal{}).Where("id = ?", mealID).
		Updates(map[string]any{
			"status":        status,
			"clarify_round": clarifyRound,
			"clarify_log":   clarifyLog,
		}).Error
}
