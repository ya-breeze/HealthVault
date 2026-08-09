package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"gorm.io/gorm"

	"github.com/ya-breeze/healthvault/pkg/database"
)

// patchItemRequest is the JSON body for PATCH /api/food/meals/{id}/items/{item_id}.
// Exactly one of Manual/FdcID-or-CustomFoodID/WeightGrams-alone applies, in
// this precedence:
//  1. Manual: the 7 macro fields are stored as given, macro_source = manual.
//  2. FdcID or CustomFoodID: bound to that reference food, macro_source =
//     reference, macros scaled from its profile by WeightGrams.
//  3. WeightGrams alone: rescales the item from its existing binding, if any.
//
// Name is independent of the above and may be sent alongside any of them, or
// alone: it corrects the item's displayed description (e.g. the vision
// model's guess was "dark berries" but it's actually cherries) without
// implying anything about macro_source.
type patchItemRequest struct {
	Manual       bool       `json:"manual,omitempty"`
	FdcID        *int64     `json:"fdc_id,omitempty"`
	CustomFoodID *uuid.UUID `json:"custom_food_id,omitempty"`
	WeightGrams  *float64   `json:"weight_grams,omitempty"`
	Name         *string    `json:"name,omitempty"`

	Calories          float64 `json:"calories,omitempty"`
	ProteinGrams      float64 `json:"protein_grams,omitempty"`
	CarbsGrams        float64 `json:"carbs_grams,omitempty"`
	FatGrams          float64 `json:"fat_grams,omitempty"`
	SugarGrams        float64 `json:"sugar_grams,omitempty"`
	SodiumGrams       float64 `json:"sodium_grams,omitempty"`
	DietaryFiberGrams float64 `json:"dietary_fiber_grams,omitempty"`
}

// editableMealStatus reports whether a meal's items (and its name/logged_at)
// can be mutated. pending_review and confirmed both have a stable, reviewable
// item set; processing/pending_clarification/failed do not.
func editableMealStatus(status string) bool {
	return status == database.MealStatusPendingReview || status == database.MealStatusConfirmed
}

// errMealNoLongerEditable is returned by applyItemMutation when the meal's
// status has moved out of pending_review/confirmed between the handler's
// initial status check and the transaction actually running — e.g. a
// concurrent Reanalyze claimed the meal (and, on success, replaced its items
// and zeroed its aggregate) in that window. Callers map it to HTTP 409.
var errMealNoLongerEditable = errors.New("meal is no longer editable")

// applyItemMutation runs fn (an item create/update/delete) inside a
// transaction. It re-reads the meal's status *inside* that transaction and
// aborts with errMealNoLongerEditable if it's no longer pending_review or
// confirmed, before fn runs — closing the gap between the handler's earlier
// status check and this write. Without that re-check, a concurrent Reanalyze
// could replace/delete the meal's items and zero its aggregate in between,
// and this transaction would then blindly resurrect a since-deleted item
// (GORM's Save inserts if the primary key no longer exists) and overwrite
// the freshly-zeroed aggregate using the stale status captured before the
// race. If the meal is still confirmed, it also reloads its current items
// and recomputes+persists its macro aggregate within the same transaction
// before committing, so "the item change" and "the meal's stored total
// reflects it" commit or roll back together, and two concurrent edits to the
// same meal can't compute their aggregate from different snapshots. Returns
// the fully updated meal (current items + aggregate) for the response.
// pending_review meals are left untouched, matching today's behavior
// (aggregate computed only at confirm).
func (h *foodHandlers) applyItemMutation(
	mealID, userID uuid.UUID, fn func(tx *gorm.DB) error,
) (*database.FoodMeal, error) {
	err := h.storage.DB().Transaction(func(tx *gorm.DB) error {
		var meal database.FoodMeal
		if err := tx.Select("id", "status").Where("id = ?", mealID).First(&meal).Error; err != nil {
			return err
		}
		if !editableMealStatus(meal.Status) {
			return errMealNoLongerEditable
		}
		if err := fn(tx); err != nil {
			return err
		}
		if meal.Status != database.MealStatusConfirmed {
			return nil
		}
		var items []database.FoodItem
		if err := tx.Where("meal_id = ?", mealID).Find(&items).Error; err != nil {
			return err
		}
		var agg database.FoodMeal
		agg.Aggregate(items)
		return tx.Model(&database.FoodMeal{}).Where("id = ?", mealID).Updates(map[string]any{
			"calories":            agg.Calories,
			"protein_grams":       agg.ProteinGrams,
			"carbs_grams":         agg.CarbsGrams,
			"fat_grams":           agg.FatGrams,
			"sugar_grams":         agg.SugarGrams,
			"sodium_grams":        agg.SodiumGrams,
			"dietary_fiber_grams": agg.DietaryFiberGrams,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return h.loadOwnedMeal(mealID, userID)
}

// PatchMealItem handles PATCH /api/food/meals/{id}/items/{item_id}: resolve
// one item by binding it to a reference food, supplying macros directly, or
// changing its weight. Permitted for pending_review and confirmed meals —
// see editableMealStatus. The response is the full updated meal, not just
// the item, so a confirmed meal's freshly recomputed aggregate is visible in
// the same response (see applyItemMutation).
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
	if !editableMealStatus(meal.Status) {
		http.Error(w, "meal is not editable in its current status", http.StatusConflict)
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

	// A name that's empty or whitespace-only carries nothing to apply — treat
	// it as absent here so it can't alone satisfy "something to update" and
	// then silently no-op past the guard below.
	hasName := req.Name != nil && strings.TrimSpace(*req.Name) != ""

	if !req.Manual && req.FdcID == nil && req.CustomFoodID == nil && req.WeightGrams == nil && !hasName {
		http.Error(w, "nothing to update: specify manual, fdc_id, custom_food_id, weight_grams, or name", http.StatusBadRequest)
		return
	}
	if req.FdcID != nil && req.CustomFoodID != nil {
		http.Error(w, "specify at most one of fdc_id or custom_food_id", http.StatusBadRequest)
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
	}

	if req.Name != nil {
		if name := strings.TrimSpace(*req.Name); name != "" {
			item.Name = name
		}
	}

	updated, err := h.applyItemMutation(mealID, claims.UserID, func(tx *gorm.DB) error {
		return tx.Save(&item).Error
	})
	if errors.Is(err, errMealNoLongerEditable) {
		http.Error(w, "meal is not editable in its current status", http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, "update error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, updated)
}

// createItemRequest is the JSON body for POST /api/food/meals/{id}/items.
// Unlike patchItemRequest, a newly created item has no existing state to
// fall back on: a bare weight_grams or name alone is only meaningful as an
// edit to something that already has a name/weight/macro source. Creation
// therefore requires a non-blank Name plus exactly one of Manual+macros or
// a reference (FdcID/CustomFoodID) with a positive WeightGrams.
type createItemRequest struct {
	Name         string     `json:"name"`
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

// CreateMealItem handles POST /api/food/meals/{id}/items: add a new item to
// a meal whose status is pending_review or confirmed. Returns the full
// updated meal, same as PatchMealItem.
func (h *foodHandlers) CreateMealItem(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromCtx(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	mealID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "invalid meal id", http.StatusBadRequest)
		return
	}

	var meal database.FoodMeal
	err = h.storage.DB().Select("id", "status", "family_id").
		Where("id = ? AND user_id = ?", mealID, claims.UserID).First(&meal).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}
	if !editableMealStatus(meal.Status) {
		http.Error(w, "meal is not editable in its current status", http.StatusConflict)
		return
	}

	var req createItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	// Reject ambiguous combinations before building anything: a request must
	// specify exactly one macro source. Accepting `manual` alongside a
	// reference would silently prefer manual and discard the reference the
	// caller also sent; accepting both fdc_id and custom_food_id would
	// resolve only one of them (resolveReferenceProfile's fdc_id-first
	// precedence) while persisting both IDs on the item, leaving it claiming
	// a binding to a profile it was never actually scaled from.
	hasReference := req.FdcID != nil || req.CustomFoodID != nil
	if req.FdcID != nil && req.CustomFoodID != nil {
		http.Error(w, "specify at most one of fdc_id or custom_food_id", http.StatusBadRequest)
		return
	}
	if req.Manual && hasReference {
		http.Error(w, "specify manual macros or a food reference, not both", http.StatusBadRequest)
		return
	}

	item := database.FoodItem{UserID: claims.UserID, MealID: mealID, Name: name}
	item.ID = uuid.New()
	item.FamilyID = meal.FamilyID

	switch {
	case req.Manual:
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
		if req.WeightGrams == nil || *req.WeightGrams <= 0 {
			http.Error(w, "weight_grams must be positive for a reference item", http.StatusBadRequest)
			return
		}
		profile, status, err := h.resolveReferenceProfile(claims.UserID, req.FdcID, req.CustomFoodID)
		if err != nil {
			http.Error(w, err.Error(), status)
			return
		}
		item.FdcID = req.FdcID
		item.CustomFoodID = req.CustomFoodID
		item.WeightGrams = *req.WeightGrams
		item.ApplyProfile(profile)
	default:
		http.Error(w, "specify manual macros or a food reference (fdc_id/custom_food_id) with weight_grams", http.StatusBadRequest)
		return
	}

	updated, err := h.applyItemMutation(mealID, claims.UserID, func(tx *gorm.DB) error {
		return tx.Create(&item).Error
	})
	if errors.Is(err, errMealNoLongerEditable) {
		http.Error(w, "meal is not editable in its current status", http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, "create error", http.StatusInternalServerError)
		return
	}
	writeJSONStatus(w, http.StatusCreated, updated)
}

// DeleteMealItem handles DELETE /api/food/meals/{id}/items/{item_id}: remove
// an item from a meal whose status is pending_review or confirmed. Returns
// the full updated meal, same as PatchMealItem/CreateMealItem.
func (h *foodHandlers) DeleteMealItem(w http.ResponseWriter, r *http.Request) {
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
	if !editableMealStatus(meal.Status) {
		http.Error(w, "meal is not editable in its current status", http.StatusConflict)
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

	updated, err := h.applyItemMutation(mealID, claims.UserID, func(tx *gorm.DB) error {
		return tx.Delete(&item).Error
	})
	if errors.Is(err, errMealNoLongerEditable) {
		http.Error(w, "meal is not editable in its current status", http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, "delete error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, updated)
}
