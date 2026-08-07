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

func customFoodBody(name string, kcal float64) *bytes.Buffer {
	b, _ := json.Marshal(map[string]any{
		"name":                   name,
		"calories_per_100g":      kcal,
		"protein_per_100g":       5,
		"carbs_per_100g":         10,
		"fat_per_100g":           2,
		"sugar_per_100g":         1,
		"sodium_per_100g":        0.1,
		"dietary_fiber_per_100g": 1,
	})
	return bytes.NewBuffer(b)
}

func TestCreateCustomFood_Success(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	h := server.NewFoodHandlers(st, nil, t.TempDir())

	r := withClaims(httptest.NewRequest(http.MethodPost, "/api/food/custom", customFoodBody("Yogurt", 60)), userID)
	w := httptest.NewRecorder()
	h.CreateCustomFood(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var got database.CustomFood
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Name != "Yogurt" || got.UserID != userID {
		t.Errorf("unexpected result: %+v", got)
	}
}

func TestCreateCustomFood_DuplicateNameReturns409(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	h := server.NewFoodHandlers(st, nil, t.TempDir())

	r1 := withClaims(httptest.NewRequest(http.MethodPost, "/api/food/custom", customFoodBody("Yogurt", 60)), userID)
	h.CreateCustomFood(httptest.NewRecorder(), r1)

	r2 := withClaims(httptest.NewRequest(http.MethodPost, "/api/food/custom", customFoodBody("Yogurt", 61)), userID)
	w2 := httptest.NewRecorder()
	h.CreateCustomFood(w2, r2)

	if w2.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestListCustomFoods_OnlyOwnFoods(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	otherUserID := uuid.New()

	mine := database.CustomFood{UserID: userID, Name: "Mine", CaloriesPer100g: 10}
	mine.ID = uuid.New()
	mine.FamilyID = familyID
	st.DB().Create(&mine) //nolint:errcheck

	theirs := database.CustomFood{UserID: otherUserID, Name: "Theirs", CaloriesPer100g: 10}
	theirs.ID = uuid.New()
	theirs.FamilyID = familyID
	st.DB().Create(&theirs) //nolint:errcheck

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	r := withClaims(httptest.NewRequest(http.MethodGet, "/api/food/custom", nil), userID)
	w := httptest.NewRecorder()
	h.ListCustomFoods(w, r)

	var got []database.CustomFood
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].Name != "Mine" {
		t.Errorf("expected only own food, got %+v", got)
	}
}

func withIDVar(r *http.Request, id string) *http.Request {
	return mux.SetURLVars(r, map[string]string{"id": id})
}

func TestUpdateCustomFood_Success(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	c := database.CustomFood{UserID: userID, Name: "Yogurt", CaloriesPer100g: 60}
	c.ID = uuid.New()
	c.FamilyID = familyID
	if err := st.DB().Create(&c).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	r := withIDVar(withClaims(httptest.NewRequest(http.MethodPut, "/api/food/custom/"+c.ID.String(), customFoodBody("Yogurt", 75)), userID), c.ID.String())
	w := httptest.NewRecorder()
	h.UpdateCustomFood(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var updated database.CustomFood
	st.DB().First(&updated, "id = ?", c.ID) //nolint:errcheck
	if updated.CaloriesPer100g != 75 {
		t.Errorf("expected updated calories 75, got %v", updated.CaloriesPer100g)
	}
}

func TestUpdateCustomFood_CrossUserReturns404(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	otherUserID := uuid.New()

	c := database.CustomFood{UserID: otherUserID, Name: "Yogurt", CaloriesPer100g: 60}
	c.ID = uuid.New()
	c.FamilyID = familyID
	if err := st.DB().Create(&c).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	r := withIDVar(withClaims(httptest.NewRequest(http.MethodPut, "/api/food/custom/"+c.ID.String(), customFoodBody("Yogurt", 75)), userID), c.ID.String())
	w := httptest.NewRecorder()
	h.UpdateCustomFood(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestDeleteCustomFood_Success(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	c := database.CustomFood{UserID: userID, Name: "Yogurt", CaloriesPer100g: 60}
	c.ID = uuid.New()
	c.FamilyID = familyID
	if err := st.DB().Create(&c).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	r := withIDVar(withClaims(httptest.NewRequest(http.MethodDelete, "/api/food/custom/"+c.ID.String(), nil), userID), c.ID.String())
	w := httptest.NewRecorder()
	h.DeleteCustomFood(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	var count int64
	st.DB().Model(&database.CustomFood{}).Where("id = ?", c.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected food to be deleted, count=%d", count)
	}
}

func TestDeleteCustomFood_CrossUserReturns404(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	otherUserID := uuid.New()

	c := database.CustomFood{UserID: otherUserID, Name: "Yogurt", CaloriesPer100g: 60}
	c.ID = uuid.New()
	c.FamilyID = familyID
	if err := st.DB().Create(&c).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	r := withIDVar(withClaims(httptest.NewRequest(http.MethodDelete, "/api/food/custom/"+c.ID.String(), nil), userID), c.ID.String())
	w := httptest.NewRecorder()
	h.DeleteCustomFood(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	var count int64
	st.DB().Model(&database.CustomFood{}).Where("id = ?", c.ID).Count(&count)
	if count != 1 {
		t.Errorf("expected food to still exist, count=%d", count)
	}
}
