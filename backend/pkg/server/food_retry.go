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

	if err := h.storage.DB().Model(&database.FoodMeal{}).Where("id = ?", meal.ID).
		Update("status", database.MealStatusProcessing).Error; err != nil {
		http.Error(w, "update error", http.StatusInternalServerError)
		return
	}
	meal.Status = database.MealStatusProcessing

	h.analyzeMeal(r.Context(), &meal)
	writeJSON(w, meal)
}
