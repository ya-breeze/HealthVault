package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
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
	priorUpdatedAt := meal.UpdatedAt

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

	// Atomic claim on the *exact* (status, updated_at) just observed — an
	// optimistic-concurrency check, not just a status match. Two hazards
	// this closes:
	//
	//  1. Status alone ("status IN eligible") isn't enough: ConfirmMeal
	//     transitions pending_review -> confirmed, and Reanalyze is
	//     eligible from both. If ConfirmMeal committed that transition in
	//     the gap between this handler's read above and this claim, a
	//     status-only claim would still match (confirmed is also
	//     eligible), silently claim the meal, and on failure revert to the
	//     *stale* pending_review captured before ConfirmMeal ran —
	//     discarding a legitimate concurrent confirm.
	//  2. RetryMeal treats `processing` as retryable once `updated_at` is
	//     older than the vision timeout — the same threshold this
	//     handler's own vision call runs against. If this attempt's call
	//     is slow enough to cross that threshold right as it fails, a
	//     concurrent RetryMeal can legitimately claim the same meal as
	//     "stale processing" before this handler's revert runs.
	//     `updated_at` doubles as this attempt's lease token: the claim
	//     below writes a fresh one, and every later write for *this*
	//     attempt (persistAnalysis, failMeal, revertReanalyze) is
	//     conditioned on that exact value — so a newer claim (which writes
	//     its own fresh updated_at) silently invalidates this attempt's
	//     right to write anything further, rather than one attempt
	//     clobbering the other's in-flight or completed work.
	lease := time.Now().UTC()
	claim := h.storage.DB().Model(&database.FoodMeal{}).
		Where("id = ? AND status = ? AND updated_at = ?", meal.ID, priorStatus, priorUpdatedAt).
		Updates(map[string]any{
			"status":        database.MealStatusProcessing,
			"clarify_round": 0,
			"clarify_log":   "",
			"updated_at":    lease,
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
	meal.UpdatedAt = lease

	ctx, cancel := context.WithTimeout(r.Context(), h.visionTimeout)
	defer cancel()

	if err := h.runAnalysis(ctx, &meal, hint, lease); err != nil {
		// Non-destructive failure: restore exactly what the claim overwrote.
		// Items and aggregate were never touched — persistAnalysis's
		// delete-and-replace only runs on a successful recognition — so the
		// meal is left exactly as it was found, PROVIDED this attempt's
		// lease is still current (see revertReanalyze). Responding with 502
		// (not 200 with a mutated meal) lets the caller distinguish
		// "reanalysis failed, nothing changed" from a normal state
		// transition.
		revert := h.revertReanalyze(meal.ID, lease, priorStatus, priorClarifyRound, priorClarifyLog)
		if revert.err != nil {
			// The revert itself failed: the meal is genuinely stuck in
			// processing, not "unchanged" — say so distinctly rather than
			// claiming a guarantee that no longer holds. RetryMeal's stale-
			// processing recovery remains available once the vision timeout
			// elapses.
			http.Error(w, "reanalysis failed and the meal could not be restored to its prior state", http.StatusInternalServerError)
			return
		}
		if !revert.applied {
			// The lease was already gone: a newer attempt (e.g. a
			// stale-processing RetryMeal, once this call ran long enough to
			// cross the same vision timeout) has since claimed this meal.
			// That attempt owns the row now — reverting would stomp on
			// whatever it's doing. This request's own attempt still
			// failed, but the meal is NOT guaranteed unchanged — the newer
			// attempt may already have altered it, and this handler has no
			// way to know its outcome. HTTP 412 (Precondition Failed) — the
			// standard code for "your conditional update's precondition, the
			// lease, no longer holds" — signals that distinctly from both
			// the plain "not eligible" 409 (nothing was ever attempted) and
			// the "vision failed, revert succeeded" 502 (genuinely
			// unchanged): the caller should refetch rather than assume
			// nothing changed.
			http.Error(w, "reanalysis failed; the meal was claimed by another operation in the meantime", http.StatusPreconditionFailed)
			return
		}
		http.Error(w, "reanalysis failed; the meal is unchanged", http.StatusBadGateway)
		return
	}

	writeJSON(w, &meal)
}

// revertResult reports whether a revert/persist write conditioned on a lease
// token actually applied (revertReanalyze, persistAnalysis, failMeal all
// follow this pattern): applied is false when the lease no longer matches —
// i.e. a newer analysis attempt has since claimed the row — which is a
// normal, expected outcome, not an error.
type revertResult struct {
	applied bool
	err     error
}

// revertReanalyze restores a meal's status/clarify fields to their pre-claim
// values after a failed reanalysis attempt, conditioned on this attempt's
// lease (the updated_at its own claim wrote) still being current. If a newer
// attempt (see the claim's doc comment above) has since claimed the row,
// updated_at no longer matches and this is a no-op — the newer attempt owns
// the row now, and stomping its in-flight or completed work would be worse
// than leaving this attempt's revert undone.
func (h *foodHandlers) revertReanalyze(
	mealID uuid.UUID, lease time.Time, status string, clarifyRound int, clarifyLog string,
) revertResult {
	res := h.storage.DB().Model(&database.FoodMeal{}).
		Where("id = ? AND updated_at = ?", mealID, lease).
		Updates(map[string]any{
			"status":        status,
			"clarify_round": clarifyRound,
			"clarify_log":   clarifyLog,
		})
	return revertResult{applied: res.RowsAffected > 0, err: res.Error}
}
