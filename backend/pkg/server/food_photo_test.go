package server_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/server"
	photostorage "github.com/ya-breeze/healthvault/pkg/storage"
)

func photoGetRequest(kind, id string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/food/"+kind+"/"+id+"/photo", nil)
	return mux.SetURLVars(r, map[string]string{"id": id})
}

func TestMealPhoto_OwnerCanRead(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	dir := t.TempDir()
	photos := photostorage.New(dir)

	relPath, err := photos.Save(bytes.NewReader(fakeJPEGBytes), 1<<20, userID, photostorage.OwnerMeal, uuid.New())
	if err != nil {
		t.Fatalf("save photo: %v", err)
	}

	meal := database.FoodMeal{UserID: userID, Status: database.MealStatusConfirmed, PhotoPath: relPath}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}

	h := server.NewFoodHandlers(st, nil, dir)
	w := httptest.NewRecorder()
	h.MealPhoto(w, withClaims(photoGetRequest("meals", meal.ID.String()), userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("expected Content-Type image/jpeg, got %q", ct)
	}
	body, _ := io.ReadAll(w.Body)
	if !bytes.Equal(body, fakeJPEGBytes) {
		t.Errorf("photo bytes did not round-trip")
	}
}

func TestMealPhoto_CrossUserReturns404(t *testing.T) {
	st := newFoodTestStorage(t)
	_, familyID := seedFoodUser(t, st)
	otherUserID := uuid.New()
	dir := t.TempDir()
	photos := photostorage.New(dir)

	relPath, err := photos.Save(bytes.NewReader(fakeJPEGBytes), 1<<20, otherUserID, photostorage.OwnerMeal, uuid.New())
	if err != nil {
		t.Fatalf("save photo: %v", err)
	}

	meal := database.FoodMeal{UserID: otherUserID, Status: database.MealStatusConfirmed, PhotoPath: relPath}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}

	callerID := uuid.New()
	h := server.NewFoodHandlers(st, nil, dir)
	w := httptest.NewRecorder()
	h.MealPhoto(w, withClaims(photoGetRequest("meals", meal.ID.String()), callerID))

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// A family member has read access to a meal's macros via GET /api/data/food_meal
// (data-api spec), but the photo stays owner-only — see food_registry_test.go
// and the data-api spec's "Two access rules meet at food_meal" note.
func TestMealPhoto_FamilyMemberReturns404(t *testing.T) {
	st := newFoodTestStorage(t)
	ownerID, familyID := seedFoodUser(t, st)
	dir := t.TempDir()
	photos := photostorage.New(dir)

	relPath, err := photos.Save(bytes.NewReader(fakeJPEGBytes), 1<<20, ownerID, photostorage.OwnerMeal, uuid.New())
	if err != nil {
		t.Fatalf("save photo: %v", err)
	}
	meal := database.FoodMeal{UserID: ownerID, Status: database.MealStatusConfirmed, PhotoPath: relPath}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}

	// A family member (same family, different user) still gets 404 on the photo.
	familyMemberID := uuid.New()
	h := server.NewFoodHandlers(st, nil, dir)
	w := httptest.NewRecorder()
	h.MealPhoto(w, withClaims(photoGetRequest("meals", meal.ID.String()), familyMemberID))

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for family member photo access, got %d", w.Code)
	}
}

func TestMealPhoto_NoPhotoReturns404(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	meal := database.FoodMeal{UserID: userID, Status: database.MealStatusConfirmed}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.MealPhoto(w, withClaims(photoGetRequest("meals", meal.ID.String()), userID))

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for a meal with no photo, got %d", w.Code)
	}
}

func TestMealPhoto_Unauthenticated(t *testing.T) {
	st := newFoodTestStorage(t)
	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.MealPhoto(w, photoGetRequest("meals", uuid.New().String()))

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestCalibrationSamplePhoto_OwnerCanRead(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	dir := t.TempDir()
	photos := photostorage.New(dir)

	relPath, err := photos.Save(bytes.NewReader(fakeJPEGBytes), 1<<20, userID, photostorage.OwnerCalibration, uuid.New())
	if err != nil {
		t.Fatalf("save photo: %v", err)
	}
	sample := database.FoodCalibrationSample{
		UserID: userID, PhotoPath: relPath, GroundTruth: "[]", CapturedAt: time.Now(),
	}
	sample.ID = uuid.New()
	sample.FamilyID = familyID
	if err := st.DB().Create(&sample).Error; err != nil {
		t.Fatalf("create sample: %v", err)
	}

	h := server.NewFoodHandlers(st, nil, dir)
	w := httptest.NewRecorder()
	h.CalibrationSamplePhoto(w, withClaims(photoGetRequest("calibration-samples", sample.ID.String()), userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCalibrationSamplePhoto_CrossUserReturns404(t *testing.T) {
	st := newFoodTestStorage(t)
	_, familyID := seedFoodUser(t, st)
	otherUserID := uuid.New()

	sample := database.FoodCalibrationSample{
		UserID: otherUserID, PhotoPath: "some/path.jpg", GroundTruth: "[]", CapturedAt: time.Now(),
	}
	sample.ID = uuid.New()
	sample.FamilyID = familyID
	if err := st.DB().Create(&sample).Error; err != nil {
		t.Fatalf("create sample: %v", err)
	}

	callerID := uuid.New()
	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.CalibrationSamplePhoto(w, withClaims(photoGetRequest("calibration-samples", sample.ID.String()), callerID))

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
