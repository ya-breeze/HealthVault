package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"gorm.io/gorm"

	"github.com/ya-breeze/healthvault/pkg/database"
)

// patchItemRequest is the JSON body for PATCH /api/food/meals/{id}/items/{item_id}.
// Exactly one of these applies, in this precedence:
//  1. Manual: the 7 macro fields are stored as given, macro_source = manual.
//  2. FdcID or CustomFoodID: bound to that reference food, macro_source =
//     reference, macros scaled from its profile by WeightGrams.
//  3. WeightGrams alone: rescales the item from its existing binding, if any.
type patchItemRequest struct {
	Manual       bool       `json:"manual,omitempty"`
	FdcID        *int64     `json:"fdc_id,omitempty"`
	CustomFoodID *uuid.UUID `json:"custom_food_id,omitempty"`
	WeightGrams  *float64   `json:"weight_grams,omitempty"`

	Calories          float64 `json:"calories,omitempty"`
	ProteinGrams      float64 `json:"protein_grams,omitempty"`
	CarbsGrams        float64 `json:"carbs_grams,omitempty"`
	FatGrams          float64 `json:"fat_grams,omitempty"`
	SugarGrams        float64 `json:"sugar_grams,omitempty"`
	SodiumGrams       float64 `json:"sodium_grams,omitempty"`
	DietaryFiberGrams float64 `json:"dietary_fiber_grams,omitempty"`
}

// PatchMealItem handles PATCH /api/food/meals/{id}/items/{item_id}: resolve
// one item by binding it to a reference food, supplying macros directly, or
// changing its weight. Rejected with 409 once the owning meal is confirmed —
// confirm finalizes a meal, it does not itself bind foods.
func (h *foodHandlers) PatchMealItem(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromCtx(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	vars := mux.Vars(r)
	mealID, err := uuid.Parse(vars["id"])
	if err != nil {
		http.Error(w, "invalid meal id", http.StatusBadRequest)
		return
	}
	itemID, err := uuid.Parse(vars["item_id"])
	if err != nil {
		http.Error(w, "invalid item id", http.StatusBadRequest)
		return
	}

	var meal database.FoodMeal
	err = h.storage.DB().Select("id", "status").
		Where("id = ? AND user_id = ?", mealID, claims.UserID).First(&meal).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}
	if meal.Status == database.MealStatusConfirmed {
		http.Error(w, "meal is already confirmed", http.StatusConflict)
		return
	}

	var item database.FoodItem
	err = h.storage.DB().
		Where("id = ? AND meal_id = ? AND user_id = ?", itemID, mealID, claims.UserID).
		First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}

	var req patchItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	switch {
	case req.Manual:
		item.FdcID = nil
		item.CustomFoodID = nil
		item.MacroSource = database.MacroSourceManual
		if req.WeightGrams != nil {
			item.WeightGrams = *req.WeightGrams
		}
		item.Calories = req.Calories
		item.ProteinGrams = req.ProteinGrams
		item.CarbsGrams = req.CarbsGrams
		item.FatGrams = req.FatGrams
		item.SugarGrams = req.SugarGrams
		item.SodiumGrams = req.SodiumGrams
		item.DietaryFiberGrams = req.DietaryFiberGrams
	case req.FdcID != nil || req.CustomFoodID != nil:
		profile, status, err := h.resolveReferenceProfile(claims.UserID, req.FdcID, req.CustomFoodID)
		if err != nil {
			http.Error(w, err.Error(), status)
			return
		}
		item.FdcID = req.FdcID
		item.CustomFoodID = req.CustomFoodID
		if req.WeightGrams != nil {
			item.WeightGrams = *req.WeightGrams
		}
		item.ApplyProfile(profile)
	case req.WeightGrams != nil:
		item.WeightGrams = *req.WeightGrams
		if item.MacroSource == database.MacroSourceReference {
			profile, status, err := h.resolveReferenceProfile(claims.UserID, item.FdcID, item.CustomFoodID)
			if err != nil {
				http.Error(w, err.Error(), status)
				return
			}
			item.ApplyProfile(profile)
		}
	default:
		http.Error(w, "nothing to update: specify manual, fdc_id, custom_food_id, or weight_grams", http.StatusBadRequest)
		return
	}

	if err := h.storage.DB().Save(&item).Error; err != nil {
		http.Error(w, "update error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, item)
}
