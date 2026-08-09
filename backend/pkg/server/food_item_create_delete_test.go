package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/server"
)

func createItemRequest(mealID string, body map[string]any) *http.Request {
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(http.MethodPost, "/api/food/meals/"+mealID+"/items", bytes.NewBuffer(b))
	return mux.SetURLVars(r, map[string]string{"id": mealID})
}

func deleteItemRequest(mealID, itemID string) *http.Request {
	r := httptest.NewRequest(http.MethodDelete, "/api/food/meals/"+mealID+"/items/"+itemID, nil)
	return mux.SetURLVars(r, map[string]string{"id": mealID, "item_id": itemID})
}

// --- CreateMealItem ---

func TestCreateMealItem_ManualOnConfirmedMealUpdatesAggregate(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createUnresolvedMeal(t, st, userID, familyID)
	if err := st.DB().Model(&database.FoodMeal{}).Where("id = ?", meal.ID).
		Update("status", database.MealStatusConfirmed).Error; err != nil {
		t.Fatalf("confirm meal: %v", err)
	}

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	r := createItemRequest(meal.ID.String(), map[string]any{
		"name": "Side salad", "manual": true, "calories": 150, "protein_grams": 3,
	})
	h.CreateMealItem(w, withClaims(r, userID))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var got database.FoodMeal
	json.NewDecoder(w.Body).Decode(&got) //nolint:errcheck
	if len(got.Items) != 2 {
		t.Fatalf("expected the meal to now have 2 items, got %d", len(got.Items))
	}
	if got.Calories != 150 {
		t.Errorf("expected meal aggregate to include the new item's 150 kcal, got %v", got.Calories)
	}
}

func TestCreateMealItem_ReferenceRequiresPositiveWeight(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createUnresolvedMeal(t, st, userID, familyID)
	idx := buildUSDAIndex(t, usdaFood(7, "Chicken breast", 165))
	h := server.NewFoodHandlers(st, idx, t.TempDir())

	for _, weight := range []any{nil, 0, -5} {
		body := map[string]any{"name": "Chicken", "fdc_id": 7}
		if weight != nil {
			body["weight_grams"] = weight
		}
		w := httptest.NewRecorder()
		h.CreateMealItem(w, withClaims(createItemRequest(meal.ID.String(), body), userID))
		if w.Code != http.StatusBadRequest {
			t.Errorf("weight=%v: expected 400, got %d: %s", weight, w.Code, w.Body.String())
		}
	}
}

func TestCreateMealItem_MissingNameReturns400(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createUnresolvedMeal(t, st, userID, familyID)
	h := server.NewFoodHandlers(st, nil, t.TempDir())

	w := httptest.NewRecorder()
	r := createItemRequest(meal.ID.String(), map[string]any{"manual": true, "calories": 100})
	h.CreateMealItem(w, withClaims(r, userID))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateMealItem_NoSourceReturns400(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createUnresolvedMeal(t, st, userID, familyID)
	h := server.NewFoodHandlers(st, nil, t.TempDir())

	w := httptest.NewRecorder()
	r := createItemRequest(meal.ID.String(), map[string]any{"name": "Mystery item"})
	h.CreateMealItem(w, withClaims(r, userID))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateMealItem_NonEditableStatusReturns409(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createUnresolvedMeal(t, st, userID, familyID)
	if err := st.DB().Model(&database.FoodMeal{}).Where("id = ?", meal.ID).
		Update("status", database.MealStatusFailed).Error; err != nil {
		t.Fatalf("set status: %v", err)
	}
	h := server.NewFoodHandlers(st, nil, t.TempDir())

	w := httptest.NewRecorder()
	r := createItemRequest(meal.ID.String(), map[string]any{"name": "x", "manual": true, "calories": 1})
	h.CreateMealItem(w, withClaims(r, userID))
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateMealItem_CrossUserReturns404(t *testing.T) {
	st := newFoodTestStorage(t)
	_, familyID := seedFoodUser(t, st)
	otherUserID := uuid.New()
	meal := createUnresolvedMeal(t, st, otherUserID, familyID)
	h := server.NewFoodHandlers(st, nil, t.TempDir())

	w := httptest.NewRecorder()
	r := createItemRequest(meal.ID.String(), map[string]any{"name": "x", "manual": true, "calories": 1})
	h.CreateMealItem(w, withClaims(r, uuid.New()))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	var items []database.FoodItem
	st.DB().Where("meal_id = ?", meal.ID).Find(&items) //nolint:errcheck
	if len(items) != 1 {
		t.Errorf("expected no item created, still have the original 1, got %d", len(items))
	}
}

// --- DeleteMealItem ---

func TestDeleteMealItem_ConfirmedMealUpdatesAggregate(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createUnresolvedMeal(t, st, userID, familyID)
	if err := st.DB().Model(&database.FoodItem{}).Where("id = ?", meal.Items[0].ID).
		Updates(map[string]any{"macro_source": database.MacroSourceManual, "calories": 400}).Error; err != nil {
		t.Fatalf("seed item macros: %v", err)
	}
	if err := st.DB().Model(&database.FoodMeal{}).Where("id = ?", meal.ID).
		Updates(map[string]any{"status": database.MealStatusConfirmed, "calories": 400}).Error; err != nil {
		t.Fatalf("confirm meal: %v", err)
	}

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.DeleteMealItem(w, withClaims(deleteItemRequest(meal.ID.String(), meal.Items[0].ID.String()), userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got database.FoodMeal
	json.NewDecoder(w.Body).Decode(&got) //nolint:errcheck
	if len(got.Items) != 0 {
		t.Errorf("expected 0 items after delete, got %d", len(got.Items))
	}
	if got.Calories != 0 {
		t.Errorf("expected aggregate to drop to 0 after deleting the only item, got %v", got.Calories)
	}
}

func TestDeleteMealItem_NonexistentItemReturns404(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createUnresolvedMeal(t, st, userID, familyID)
	h := server.NewFoodHandlers(st, nil, t.TempDir())

	w := httptest.NewRecorder()
	h.DeleteMealItem(w, withClaims(deleteItemRequest(meal.ID.String(), uuid.New().String()), userID))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestDeleteMealItem_NonEditableStatusReturns409(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createUnresolvedMeal(t, st, userID, familyID)
	if err := st.DB().Model(&database.FoodMeal{}).Where("id = ?", meal.ID).
		Update("status", database.MealStatusPendingClarification).Error; err != nil {
		t.Fatalf("set status: %v", err)
	}
	h := server.NewFoodHandlers(st, nil, t.TempDir())

	w := httptest.NewRecorder()
	h.DeleteMealItem(w, withClaims(deleteItemRequest(meal.ID.String(), meal.Items[0].ID.String()), userID))
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteMealItem_CrossUserReturns404(t *testing.T) {
	st := newFoodTestStorage(t)
	_, familyID := seedFoodUser(t, st)
	otherUserID := uuid.New()
	meal := createUnresolvedMeal(t, st, otherUserID, familyID)
	h := server.NewFoodHandlers(st, nil, t.TempDir())

	w := httptest.NewRecorder()
	h.DeleteMealItem(w, withClaims(deleteItemRequest(meal.ID.String(), meal.Items[0].ID.String()), uuid.New()))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	var item database.FoodItem
	if err := st.DB().Where("id = ?", meal.Items[0].ID).First(&item).Error; err != nil {
		t.Errorf("expected the item to still exist after a rejected cross-user delete, got: %v", err)
	}
}
