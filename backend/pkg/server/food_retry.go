package server

import (
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"gorm.io/gorm"

	"github.com/ya-breeze/healthvault/pkg/database"
)

// RetryMeal handles POST /api/food/meals/{id}/retry: re-runs analysis on the
// stored photo. Accepted only when the meal is failed, or processing with an
// updated_at older than HCW_VISION_TIMEOUT — a live call within that window
// is normal, not a stranded one, and retrying it would start a second
// concurrent analysis of the same meal. See design.md "Synchronous Vision
// Call", "processing is ambiguous, and retry must not treat it as always-stale."
func (h *foodHandlers) RetryMeal(w http.ResponseWriter, r *http.Request) {
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

	staleProcessing := meal.Status == database.MealStatusProcessing &&
		time.Since(meal.UpdatedAt) > h.visionTimeout
	if meal.Status != database.MealStatusFailed && !staleProcessing {
		http.Error(w, "meal is not eligible for retry", http.StatusConflict)
		return
	}
	if meal.PhotoPath == "" {
		http.Error(w, "meal has no photo to retry", http.StatusConflict)
		return
	}

	// A retry restarts analysis from the stored photo, so any clarification
	// rounds from a prior attempt no longer apply — see analyzeMeal's fresh
	// Recognize call, which starts back at round 1.
	//
	// Conditioned on (status, updated_at) exactly matching what was just
	// read — an optimistic-concurrency claim, not a blind write. Without it,
	// two concurrent requests both observing the same stale-processing meal
	// (a double-click, or a race with another Retry/Reanalyze) could both
	// pass the eligibility check above and both restart analysis; this also
	// gives the attempt a lease token (the fresh updated_at written here)
	// that later stages (persistAnalysis, failMeal) condition their own
	// writes on — see analyzeMeal's doc comment.
	lease := time.Now().UTC()
	claim := h.storage.DB().Model(&database.FoodMeal{}).
		Where("id = ? AND status = ? AND updated_at = ?", meal.ID, meal.Status, meal.UpdatedAt).
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
		http.Error(w, "meal is not eligible for retry", http.StatusConflict)
		return
	}
	meal.Status = database.MealStatusProcessing
	meal.ClarifyRound = 0
	meal.ClarifyLog = ""
	meal.UpdatedAt = lease

	applied := h.analyzeMeal(r.Context(), &meal, lease)
	writeJSON(w, h.reloadIfSuperseded(&meal, applied))
}
