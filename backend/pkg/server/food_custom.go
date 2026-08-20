package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"gorm.io/gorm"

	"github.com/ya-breeze/healthvault/pkg/database"
)

// customFoodRequest is the JSON body for creating or updating a custom food.
type customFoodRequest struct {
	Name                string  `json:"name"`
	CaloriesPer100g     float64 `json:"calories_per_100g"`
	ProteinPer100g      float64 `json:"protein_per_100g"`
	CarbsPer100g        float64 `json:"carbs_per_100g"`
	FatPer100g          float64 `json:"fat_per_100g"`
	SugarPer100g        float64 `json:"sugar_per_100g"`
	SodiumPer100g       float64 `json:"sodium_per_100g"`
	DietaryFiberPer100g float64 `json:"dietary_fiber_per_100g"`
}

func (req customFoodRequest) applyTo(c *database.CustomFood) {
	c.Name = req.Name
	c.CaloriesPer100g = req.CaloriesPer100g
	c.ProteinPer100g = req.ProteinPer100g
	c.CarbsPer100g = req.CarbsPer100g
	c.FatPer100g = req.FatPer100g
	c.SugarPer100g = req.SugarPer100g
	c.SodiumPer100g = req.SodiumPer100g
	c.DietaryFiberPer100g = req.DietaryFiberPer100g
}

// isUniqueViolation reports whether err is a unique-constraint failure. GORM
// does not normalize this across drivers, so it is matched on the message —
// consistent with how the model layer's own tests assert it (models_food_test.go).
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique")
}

// CreateCustomFood handles POST /api/food/custom.
func (h *foodHandlers) CreateCustomFood(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromCtx(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req customFoodRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	c := database.CustomFood{UserID: claims.UserID}
	c.ID = uuid.New()
	c.FamilyID = FamilyIDFromCtx(r)
	req.applyTo(&c)

	if err := h.storage.DB().Create(&c).Error; err != nil {
		if isUniqueViolation(err) {
			http.Error(w, "a custom food with this name already exists", http.StatusConflict)
			return
		}
		http.Error(w, "create error", http.StatusInternalServerError)
		return
	}
	writeJSONStatus(w, http.StatusCreated, c)
}

// ListCustomFoods handles GET /api/food/custom.
func (h *foodHandlers) ListCustomFoods(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromCtx(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	foods, err := h.customFoodsForUser(claims.UserID)
	if err != nil {
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}
	sort.Slice(foods, func(i, j int) bool { return foods[i].Name < foods[j].Name })
	if foods == nil {
		foods = []database.CustomFood{}
	}
	writeJSON(w, foods)
}

// customFoodsForUser loads every custom food owned by userID, unordered.
// Shared by ListCustomFoods, Search (food.go), and resolveItems
// (food_upload.go) so the same "all of a user's custom foods" query isn't
// hand-rolled separately in each — a future scoping change (e.g. an explicit
// soft-delete filter) only needs to be made here.
func (h *foodHandlers) customFoodsForUser(userID uuid.UUID) ([]database.CustomFood, error) {
	var foods []database.CustomFood
	err := h.storage.DB().Where("user_id = ?", userID).Find(&foods).Error
	return foods, err
}

// findOwnedCustomFood loads a custom food by ID, scoped to userID. Returns
// database.ErrNotFound if it does not exist or belongs to someone else —
// callers must surface that as 404, never 403, so a family member or stranger
// cannot distinguish "not yours" from "does not exist".
func (h *foodHandlers) findOwnedCustomFood(id, userID uuid.UUID) (*database.CustomFood, error) {
	var c database.CustomFood
	err := h.storage.DB().Where("id = ? AND user_id = ?", id, userID).First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, database.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// UpdateCustomFood handles PUT /api/food/custom/{id}.
func (h *foodHandlers) UpdateCustomFood(w http.ResponseWriter, r *http.Request) {
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

	c, err := h.findOwnedCustomFood(id, claims.UserID)
	if errors.Is(err, database.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}

	var req customFoodRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	// Trimmed once, up front, and used for both the rename check below and
	// the stored value (via applyTo): comparing a trimmed request name
	// against c.Name (also always stored trimmed) while leaving req.Name
	// untrimmed would treat incidental leading/trailing whitespace as a
	// "rename" on every no-op edit, wiping CanonicalName for nothing.
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	// A renamed custom food invalidates whatever Canonical Name recognition
	// (or a save_as_custom_food copy) produced for the old name — carrying
	// it forward would pair the corrected name with a stale, unrelated
	// English gloss in Expert Mode. Unlike a meal item's PATCH (see
	// food_item.go's PatchMealItem), this endpoint is a full-resource PUT
	// with no partial-rename-only mode, so every name change here is
	// equivalent to the "manual correction" case that clears it there.
	if req.Name != c.Name {
		c.CanonicalName = ""
	}
	req.applyTo(c)

	if err := h.storage.DB().Save(c).Error; err != nil {
		if isUniqueViolation(err) {
			http.Error(w, "a custom food with this name already exists", http.StatusConflict)
			return
		}
		http.Error(w, "update error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, c)
}

// DeleteCustomFood handles DELETE /api/food/custom/{id}.
func (h *foodHandlers) DeleteCustomFood(w http.ResponseWriter, r *http.Request) {
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

	// Unscoped: CustomFood embeds TenantModel, so a plain Delete soft-deletes
	// (sets deleted_at) rather than removing the row. The (user_id, name)
	// unique index has no deleted_at clause, so a soft-deleted row would
	// permanently block ever creating that name again — the opposite of
	// "delete and re-add" being a working correction path.
	res := h.storage.DB().Unscoped().
		Where("id = ? AND user_id = ?", id, claims.UserID).Delete(&database.CustomFood{})
	if res.Error != nil {
		http.Error(w, "delete error", http.StatusInternalServerError)
		return
	}
	if res.RowsAffected == 0 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
