package server_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/server"
	photostorage "github.com/ya-breeze/healthvault/pkg/storage"
)

// fakeJPEGBytes is the minimum a real JPEG needs for storage.DetectFormat to
// accept it: the SOI+APP0 marker prefix, checked by its first three bytes.
var fakeJPEGBytes = append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, bytes.Repeat([]byte{0}, 32)...)

func newFoodMealDeleteRequest(id string) *http.Request {
	r := httptest.NewRequest(http.MethodDelete, "/api/data/food_meal/"+id, nil)
	return mux.SetURLVars(r, map[string]string{"type": "food_meal", "id": id})
}

func TestDeleteRecordHandler_FoodMealCascadesItemsAndPhoto(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	photoDir := t.TempDir()
	photos := photostorage.New(photoDir)
	relPath, err := photos.Save(bytes.NewReader(fakeJPEGBytes), 1<<20, userID, photostorage.OwnerMeal, uuid.New())
	if err != nil {
		t.Fatalf("save photo: %v", err)
	}
	if _, err := os.Stat(filepath.Join(photoDir, relPath)); err != nil {
		t.Fatalf("expected photo to exist before delete: %v", err)
	}

	meal := database.FoodMeal{
		UserID: userID, Status: database.MealStatusConfirmed, LoggedAt: time.Now(),
		PhotoPath: relPath,
	}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}
	item := database.FoodItem{UserID: userID, MealID: meal.ID, Name: "Rice", WeightGrams: 100}
	item.ID = uuid.New()
	item.FamilyID = familyID
	if err := st.DB().Create(&item).Error; err != nil {
		t.Fatalf("create item: %v", err)
	}

	h := server.DeleteRecordHandler(st, photos)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, withClaims(newFoodMealDeleteRequest(meal.ID.String()), userID))

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	var itemCount int64
	st.DB().Table("food_items").Where("meal_id = ?", meal.ID).Count(&itemCount)
	if itemCount != 0 {
		t.Errorf("expected items cascade-deleted, got %d", itemCount)
	}
	if _, err := os.Stat(filepath.Join(photoDir, relPath)); !os.IsNotExist(err) {
		t.Errorf("expected photo file removed, stat err = %v", err)
	}
}

func TestDeleteRecordHandler_FoodMealMissingPhotoFileStillSucceeds(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	photos := photostorage.New(t.TempDir())

	meal := database.FoodMeal{
		UserID: userID, Status: database.MealStatusConfirmed, LoggedAt: time.Now(),
		PhotoPath: "nonexistent/path.jpg",
	}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}

	h := server.DeleteRecordHandler(st, photos)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, withClaims(newFoodMealDeleteRequest(meal.ID.String()), userID))

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204 even with a missing photo file, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteRecordHandler_FoodMealCrossUserReturns404AndRemovesNothing(t *testing.T) {
	st := newFoodTestStorage(t)
	_, familyID := seedFoodUser(t, st)
	otherUserID := uuid.New()
	callerID := uuid.New()

	meal := database.FoodMeal{UserID: otherUserID, Status: database.MealStatusConfirmed, LoggedAt: time.Now()}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}

	h := server.DeleteRecordHandler(st, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, withClaims(newFoodMealDeleteRequest(meal.ID.String()), callerID))

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
	var count int64
	st.DB().Table("food_meals").Where("id = ?", meal.ID).Count(&count)
	if count != 1 {
		t.Errorf("expected meal to still exist, count=%d", count)
	}
}
