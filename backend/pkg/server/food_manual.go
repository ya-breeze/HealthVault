package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ya-breeze/healthvault/pkg/database"
)

// manualMealItemRequest is one item in a POST /api/food/meals/manual request.
// Source selects how the item's macros are determined:
//   - "reference": scaled from a bound fdc_id or custom_food_id profile by WeightGrams.
//   - "manual": the 7 macro fields are stored exactly as given.
type manualMealItemRequest struct {
	Name         string     `json:"name"`
	Source       string     `json:"source"`
	WeightGrams  float64    `json:"weight_grams"`
	FdcID        *int64     `json:"fdc_id,omitempty"`
	CustomFoodID *uuid.UUID `json:"custom_food_id,omitempty"`
	OffCode      *string    `json:"off_code,omitempty"`

	Calories          float64 `json:"calories"`
	ProteinGrams      float64 `json:"protein_grams"`
	CarbsGrams        float64 `json:"carbs_grams"`
	FatGrams          float64 `json:"fat_grams"`
	SugarGrams        float64 `json:"sugar_grams"`
	SodiumGrams       float64 `json:"sodium_grams"`
	DietaryFiberGrams float64 `json:"dietary_fiber_grams"`
}

type manualMealRequest struct {
	Name     string                  `json:"name"`
	LoggedAt *time.Time              `json:"logged_at,omitempty"`
	Items    []manualMealItemRequest `json:"items"`
}

// CreateManualMeal handles POST /api/food/meals/manual: photo-free entry from
// food references or direct macro values. Every item resolves to usable
// macros at creation time (there is no vision step to leave anything
// unresolved), so the meal is created already confirmed with its aggregate.
func (h *foodHandlers) CreateManualMeal(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromCtx(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req manualMealRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if len(req.Items) == 0 {
		http.Error(w, "at least one item is required", http.StatusBadRequest)
		return
	}

	loggedAt := time.Now().UTC()
	if req.LoggedAt != nil {
		// UTC-normalize before storing — see PatchMeal/ConfirmMeal's
		// identical treatment for why an un-normalized offset breaks
		// history ordering and its keyset cursor.
		loggedAt = req.LoggedAt.UTC()
	}

	familyID := FamilyIDFromCtx(r)
	meal := database.FoodMeal{
		UserID:   claims.UserID,
		Status:   database.MealStatusConfirmed,
		LoggedAt: loggedAt,
		Name:     req.Name,
	}
	meal.ID = uuid.New()
	meal.FamilyID = familyID

	items := make([]database.FoodItem, 0, len(req.Items))
	for _, itemReq := range req.Items {
		item := database.FoodItem{UserID: claims.UserID, MealID: meal.ID, Name: itemReq.Name}
		item.ID = uuid.New()
		item.FamilyID = familyID

		// New check, not an extension of an existing one: this endpoint has
		// never validated reference exclusivity, so both an fdc_id and a
		// custom_food_id (now also an off_code) could be persisted together
		// even though resolveReferenceProfile only ever resolves one of
		// them. Fixed here as part of adding the third field rather than
		// grandfathering the gap forward — see design.md's Risks.
		if refSourcesSet(itemReq.FdcID, itemReq.OffCode, itemReq.CustomFoodID) > 1 {
			http.Error(w, "specify at most one of fdc_id, off_code, or custom_food_id", http.StatusBadRequest)
			return
		}

		switch itemReq.Source {
		case "reference":
			profile, status, err := h.resolveReferenceProfile(claims.UserID, itemReq.FdcID, itemReq.OffCode, itemReq.CustomFoodID)
			if err != nil {
				http.Error(w, err.Error(), status)
				return
			}
			if itemReq.WeightGrams <= 0 {
				http.Error(w, "weight_grams must be positive for a reference item", http.StatusBadRequest)
				return
			}
			item.FdcID = itemReq.FdcID
			item.CustomFoodID = itemReq.CustomFoodID
			item.OffCode = itemReq.OffCode
			item.WeightGrams = itemReq.WeightGrams
			item.ApplyProfile(profile)
		case "manual":
			item.WeightGrams = itemReq.WeightGrams
			item.MacroSource = database.MacroSourceManual
			item.Calories = itemReq.Calories
			item.ProteinGrams = itemReq.ProteinGrams
			item.CarbsGrams = itemReq.CarbsGrams
			item.FatGrams = itemReq.FatGrams
			item.SugarGrams = itemReq.SugarGrams
			item.SodiumGrams = itemReq.SodiumGrams
			item.DietaryFiberGrams = itemReq.DietaryFiberGrams
		default:
			http.Error(w, `item source must be "reference" or "manual"`, http.StatusBadRequest)
			return
		}
		items = append(items, item)
	}

	meal.Aggregate(items)

	err := h.storage.DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&meal).Error; err != nil {
			return err
		}
		return tx.Create(&items).Error
	})
	if err != nil {
		http.Error(w, "create error", http.StatusInternalServerError)
		return
	}

	meal.Items = items
	writeJSONStatus(w, http.StatusCreated, meal)
}

// resolveReferenceProfile looks up the per-100g profile for a reference
// binding (an fdc_id, off_code, or custom_food_id, scoped to userID for
// custom foods). The returned status is only meaningful when err is
// non-nil. Shared by manual meal item creation and the item PATCH/POST
// endpoints (food_item.go).
func (h *foodHandlers) resolveReferenceProfile(
	userID uuid.UUID, fdcID *int64, offCode *string, customFoodID *uuid.UUID,
) (database.NutrientProfile, int, error) {
	switch {
	case fdcID != nil:
		if h.usda == nil {
			return database.NutrientProfile{}, http.StatusBadRequest, errors.New("usda index unavailable")
		}
		food, err := h.usda.ByFdcID(*fdcID)
		if err != nil {
			return database.NutrientProfile{}, http.StatusInternalServerError, errors.New("usda lookup error")
		}
		if food == nil {
			return database.NutrientProfile{}, http.StatusBadRequest, errors.New("unknown fdc_id")
		}
		return food.Profile, 0, nil
	case offCode != nil:
		// "off index unavailable" (no import has ever run — h.off itself is
		// nil) and "unknown off_code" (an open index that simply doesn't
		// contain this code) are different failures with the same HTTP
		// status, mirroring the fdc_id case above and the distinction
		// off.Index.ByCode's own doc comment draws — not one merged check.
		if h.off == nil {
			return database.NutrientProfile{}, http.StatusBadRequest, errors.New("off index unavailable")
		}
		food, err := h.off.ByCode(*offCode)
		if err != nil {
			return database.NutrientProfile{}, http.StatusInternalServerError, errors.New("off lookup error")
		}
		if food == nil {
			return database.NutrientProfile{}, http.StatusBadRequest, errors.New("unknown off_code")
		}
		return food.Profile, 0, nil
	case customFoodID != nil:
		cf, err := h.findOwnedCustomFood(*customFoodID, userID)
		if errors.Is(err, database.ErrNotFound) {
			return database.NutrientProfile{}, http.StatusNotFound, errors.New("custom food not found")
		}
		if err != nil {
			return database.NutrientProfile{}, http.StatusInternalServerError, errors.New("query error")
		}
		return cf.Profile(), 0, nil
	default:
		return database.NutrientProfile{}, http.StatusBadRequest,
			errors.New("reference item requires fdc_id, off_code, or custom_food_id")
	}
}

// refSourcesSet counts how many of fdc_id/off_code/custom_food_id are
// non-nil, so every call site enforces "at most one" against the same rule
// instead of duplicating pairwise combinations (fdc_id+off_code,
// fdc_id+custom_food_id, off_code+custom_food_id, all three).
func refSourcesSet(fdcID *int64, offCode *string, customFoodID *uuid.UUID) int {
	n := 0
	if fdcID != nil {
		n++
	}
	if offCode != nil {
		n++
	}
	if customFoodID != nil {
		n++
	}
	return n
}
