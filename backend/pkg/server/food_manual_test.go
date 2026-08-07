package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/server"
)

func manualMealRequest(t *testing.T, body map[string]any) *http.Request {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return httptest.NewRequest(http.MethodPost, "/api/food/meals/manual", bytes.NewBuffer(b))
}

func TestCreateManualMeal_FromUSDAReference(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	idx := buildUSDAIndex(t, usdaFood(1, "Chicken breast", 165))
	h := server.NewFoodHandlers(st, idx, t.TempDir())

	r := withClaims(manualMealRequest(t, map[string]any{
		"name": "Lunch",
		"items": []map[string]any{
			{"name": "Chicken breast", "source": "reference", "fdc_id": 1, "weight_grams": 180},
		},
	}), userID)
	w := httptest.NewRecorder()
	h.CreateManualMeal(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var meal database.FoodMeal
	if err := json.NewDecoder(w.Body).Decode(&meal); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if meal.Status != database.MealStatusConfirmed {
		t.Errorf("expected confirmed status, got %s", meal.Status)
	}
	// 165 kcal/100g * 1.8 = 297
	if meal.Calories < 296.9 || meal.Calories > 297.1 {
		t.Errorf("expected calories ~297, got %v", meal.Calories)
	}
	if len(meal.Items) != 1 || meal.Items[0].MacroSource != database.MacroSourceReference {
		t.Errorf("expected one reference item, got %+v", meal.Items)
	}
}

func TestCreateManualMeal_FromCustomFood(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	custom := database.CustomFood{UserID: userID, Name: "Protein Shake", CaloriesPer100g: 100, ProteinPer100g: 20}
	custom.ID = uuid.New()
	custom.FamilyID = familyID
	if err := st.DB().Create(&custom).Error; err != nil {
		t.Fatalf("create custom food: %v", err)
	}

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	r := withClaims(manualMealRequest(t, map[string]any{
		"items": []map[string]any{
			{"name": "Shake", "source": "reference", "custom_food_id": custom.ID.String(), "weight_grams": 250},
		},
	}), userID)
	w := httptest.NewRecorder()
	h.CreateManualMeal(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var meal database.FoodMeal
	json.NewDecoder(w.Body).Decode(&meal) //nolint:errcheck
	if meal.Calories < 249.9 || meal.Calories > 250.1 {
		t.Errorf("expected calories ~250, got %v", meal.Calories)
	}
}

func TestCreateManualMeal_DirectMacros(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	h := server.NewFoodHandlers(st, nil, t.TempDir())

	r := withClaims(manualMealRequest(t, map[string]any{
		"items": []map[string]any{
			{"name": "Package label item", "source": "manual", "calories": 200, "protein_grams": 10, "carbs_grams": 20, "fat_grams": 5},
		},
	}), userID)
	w := httptest.NewRecorder()
	h.CreateManualMeal(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var meal database.FoodMeal
	json.NewDecoder(w.Body).Decode(&meal) //nolint:errcheck
	if meal.Calories != 200 {
		t.Errorf("expected aggregate calories 200, got %v", meal.Calories)
	}
	if len(meal.Items) != 1 || meal.Items[0].MacroSource != database.MacroSourceManual {
		t.Errorf("expected one manual item, got %+v", meal.Items)
	}
}

func TestCreateManualMeal_NeverEntersProcessing(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	h := server.NewFoodHandlers(st, nil, t.TempDir())

	r := withClaims(manualMealRequest(t, map[string]any{
		"items": []map[string]any{
			{"name": "Item", "source": "manual", "calories": 100},
		},
	}), userID)
	w := httptest.NewRecorder()
	h.CreateManualMeal(w, r)

	var meal database.FoodMeal
	json.NewDecoder(w.Body).Decode(&meal) //nolint:errcheck
	if meal.Status == database.MealStatusProcessing {
		t.Errorf("manual meal must never enter processing, got %s", meal.Status)
	}
	if meal.PhotoPath != "" {
		t.Errorf("expected empty photo path, got %q", meal.PhotoPath)
	}
}

func TestCreateManualMeal_ExplicitLoggedAt(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	h := server.NewFoodHandlers(st, nil, t.TempDir())

	yesterday := time.Now().UTC().AddDate(0, 0, -1).Truncate(time.Second)
	r := withClaims(manualMealRequest(t, map[string]any{
		"logged_at": yesterday.Format(time.RFC3339),
		"items": []map[string]any{
			{"name": "Item", "source": "manual", "calories": 100},
		},
	}), userID)
	w := httptest.NewRecorder()
	h.CreateManualMeal(w, r)

	var meal database.FoodMeal
	json.NewDecoder(w.Body).Decode(&meal) //nolint:errcheck
	if !meal.LoggedAt.Equal(yesterday) {
		t.Errorf("expected logged_at %v, got %v", yesterday, meal.LoggedAt)
	}
}

func TestCreateManualMeal_UnknownFdcIDReturns400(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	idx := buildUSDAIndex(t, usdaFood(1, "Filler", 1))
	h := server.NewFoodHandlers(st, idx, t.TempDir())

	r := withClaims(manualMealRequest(t, map[string]any{
		"items": []map[string]any{
			{"name": "Ghost food", "source": "reference", "fdc_id": 999999, "weight_grams": 100},
		},
	}), userID)
	w := httptest.NewRecorder()
	h.CreateManualMeal(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateManualMeal_CrossUserCustomFoodReturns404(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	otherUserID := uuid.New()

	custom := database.CustomFood{UserID: otherUserID, Name: "Not Yours", CaloriesPer100g: 50}
	custom.ID = uuid.New()
	custom.FamilyID = familyID
	if err := st.DB().Create(&custom).Error; err != nil {
		t.Fatalf("create custom food: %v", err)
	}

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	r := withClaims(manualMealRequest(t, map[string]any{
		"items": []map[string]any{
			{"name": "X", "source": "reference", "custom_food_id": custom.ID.String(), "weight_grams": 100},
		},
	}), userID)
	w := httptest.NewRecorder()
	h.CreateManualMeal(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateManualMeal_MissingWeightReturns400(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	idx := buildUSDAIndex(t, usdaFood(1, "Filler", 1))
	h := server.NewFoodHandlers(st, idx, t.TempDir())

	r := withClaims(manualMealRequest(t, map[string]any{
		"items": []map[string]any{
			{"name": "X", "source": "reference", "fdc_id": 1},
		},
	}), userID)
	w := httptest.NewRecorder()
	h.CreateManualMeal(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateManualMeal_InvalidSourceReturns400(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	h := server.NewFoodHandlers(st, nil, t.TempDir())

	r := withClaims(manualMealRequest(t, map[string]any{
		"items": []map[string]any{
			{"name": "X", "source": "made_up"},
		},
	}), userID)
	w := httptest.NewRecorder()
	h.CreateManualMeal(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateManualMeal_NoItemsReturns400(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	h := server.NewFoodHandlers(st, nil, t.TempDir())

	r := withClaims(manualMealRequest(t, map[string]any{"items": []map[string]any{}}), userID)
	w := httptest.NewRecorder()
	h.CreateManualMeal(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateManualMeal_Unauthenticated(t *testing.T) {
	st := newFoodTestStorage(t)
	h := server.NewFoodHandlers(st, nil, t.TempDir())

	r := manualMealRequest(t, map[string]any{"items": []map[string]any{{"name": "X", "source": "manual"}}})
	w := httptest.NewRecorder()
	h.CreateManualMeal(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestCreateManualMeal_MixedResolvedAndManualAggregatesBoth(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	idx := buildUSDAIndex(t, usdaFood(1, "Rice", 130))
	h := server.NewFoodHandlers(st, idx, t.TempDir())

	r := withClaims(manualMealRequest(t, map[string]any{
		"items": []map[string]any{
			{"name": "Rice", "source": "reference", "fdc_id": 1, "weight_grams": 100},
			{"name": "Sauce", "source": "manual", "calories": 50},
		},
	}), userID)
	w := httptest.NewRecorder()
	h.CreateManualMeal(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var meal database.FoodMeal
	json.NewDecoder(w.Body).Decode(&meal) //nolint:errcheck
	// 130 (rice @ 100g) + 50 (manual sauce) = 180
	if meal.Calories < 179.9 || meal.Calories > 180.1 {
		t.Errorf("expected aggregate calories ~180, got %v", meal.Calories)
	}
}
