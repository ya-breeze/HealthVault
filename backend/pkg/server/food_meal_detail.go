package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"gorm.io/gorm"

	"github.com/ya-breeze/healthvault/pkg/database"
)

// loadOwnedMeal loads a meal with its items, scoped to userID. Returns
// database.ErrNotFound if it does not exist or belongs to someone else.
func (h *foodHandlers) loadOwnedMeal(id, userID uuid.UUID) (*database.FoodMeal, error) {
	var meal database.FoodMeal
	err := h.storage.DB().Preload("Items").
		Where("id = ? AND user_id = ?", id, userID).First(&meal).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, database.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &meal, nil
}

// GetMeal handles GET /api/food/meals/{id}: the meal with its items, for the
// review UI. Owner-only — unlike GET /api/data/food_meal, which is
// family-visible but omits items, photo, and raw_response.
func (h *foodHandlers) GetMeal(w http.ResponseWriter, r *http.Request) {
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

	meal, err := h.loadOwnedMeal(id, claims.UserID)
	if errors.Is(err, database.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, meal)
}

type confirmMealRequest struct {
	LoggedAt *time.Time `json:"logged_at,omitempty"`
}

// ConfirmMeal handles PUT /api/food/meals/{id}/confirm: aggregates the 7
// macros across items whose macro_source is reference or manual, stores the
// aggregate on the meal, optionally corrects logged_at, and sets status
// confirmed. Never writes to the Nutrition table.
func (h *foodHandlers) ConfirmMeal(w http.ResponseWriter, r *http.Request) {
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

	meal, err := h.loadOwnedMeal(id, claims.UserID)
	if errors.Is(err, database.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}

	var req confirmMealRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.LoggedAt != nil {
		meal.LoggedAt = *req.LoggedAt
	}
	meal.Aggregate(meal.Items)
	meal.Status = database.MealStatusConfirmed

	updates := map[string]any{
		"status":              meal.Status,
		"logged_at":           meal.LoggedAt,
		"calories":            meal.Calories,
		"protein_grams":       meal.ProteinGrams,
		"carbs_grams":         meal.CarbsGrams,
		"fat_grams":           meal.FatGrams,
		"sugar_grams":         meal.SugarGrams,
		"sodium_grams":        meal.SodiumGrams,
		"dietary_fiber_grams": meal.DietaryFiberGrams,
	}
	if err := h.storage.DB().Model(&database.FoodMeal{}).Where("id = ?", meal.ID).Updates(updates).Error; err != nil {
		http.Error(w, "update error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, meal)
}
