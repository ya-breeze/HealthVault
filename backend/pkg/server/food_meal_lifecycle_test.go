package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/server"
	photostorage "github.com/ya-breeze/healthvault/pkg/storage"
	"github.com/ya-breeze/healthvault/pkg/vision"
)

func mealDetailRequest(method, id string) *http.Request {
	r := httptest.NewRequest(method, "/api/food/meals/"+id, nil)
	return mux.SetURLVars(r, map[string]string{"id": id})
}

func createUnresolvedMeal(t *testing.T, st database.Storage, userID, familyID uuid.UUID) database.FoodMeal {
	t.Helper()
	meal := database.FoodMeal{UserID: userID, Status: database.MealStatusPendingReview, LoggedAt: time.Now()}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}
	item := database.FoodItem{
		UserID: userID, MealID: meal.ID, Name: "Chicken", WeightGrams: 100,
		MacroSource: database.MacroSourceNone,
	}
	item.ID = uuid.New()
	item.FamilyID = familyID
	if err := st.DB().Create(&item).Error; err != nil {
		t.Fatalf("create item: %v", err)
	}
	meal.Items = []database.FoodItem{item}
	return meal
}

// --- GetMeal ---

func TestGetMeal_ReturnsMealWithItems(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createUnresolvedMeal(t, st, userID, familyID)

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.GetMeal(w, withClaims(mealDetailRequest(http.MethodGet, meal.ID.String()), userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got database.FoodMeal
	json.NewDecoder(w.Body).Decode(&got) //nolint:errcheck
	if len(got.Items) != 1 {
		t.Errorf("expected 1 item, got %d", len(got.Items))
	}
}

func TestGetMeal_CrossUserReturns404(t *testing.T) {
	st := newFoodTestStorage(t)
	_, familyID := seedFoodUser(t, st)
	otherUserID := uuid.New()
	meal := createUnresolvedMeal(t, st, otherUserID, familyID)

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.GetMeal(w, withClaims(mealDetailRequest(http.MethodGet, meal.ID.String()), uuid.New()))

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// --- ConfirmMeal ---

func TestConfirmMeal_AggregatesReferenceAndManualExcludesNone(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	meal := database.FoodMeal{UserID: userID, Status: database.MealStatusPendingReview, LoggedAt: time.Now()}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}

	ref := database.FoodItem{UserID: userID, MealID: meal.ID, Name: "Rice", WeightGrams: 100, MacroSource: database.MacroSourceReference, Calories: 130}
	ref.ID = uuid.New()
	ref.FamilyID = familyID
	manual := database.FoodItem{UserID: userID, MealID: meal.ID, Name: "Sauce", MacroSource: database.MacroSourceManual, Calories: 50}
	manual.ID = uuid.New()
	manual.FamilyID = familyID
	none := database.FoodItem{UserID: userID, MealID: meal.ID, Name: "Mystery", MacroSource: database.MacroSourceNone}
	none.ID = uuid.New()
	none.FamilyID = familyID
	for _, it := range []database.FoodItem{ref, manual, none} {
		if err := st.DB().Create(&it).Error; err != nil {
			t.Fatalf("create item: %v", err)
		}
	}

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.ConfirmMeal(w, withClaims(mealDetailRequest(http.MethodPut, meal.ID.String()), userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got database.FoodMeal
	json.NewDecoder(w.Body).Decode(&got) //nolint:errcheck
	if got.Status != database.MealStatusConfirmed {
		t.Errorf("expected confirmed, got %s", got.Status)
	}
	if got.Calories != 180 {
		t.Errorf("expected aggregate 180 (130+50, excluding none), got %v", got.Calories)
	}

	var persisted database.FoodMeal
	st.DB().First(&persisted, "id = ?", meal.ID) //nolint:errcheck
	if persisted.Calories != 180 || persisted.Status != database.MealStatusConfirmed {
		t.Errorf("expected persisted aggregate 180 and confirmed status, got %+v", persisted)
	}
}

func TestConfirmMeal_CorrectsLoggedAt(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := database.FoodMeal{UserID: userID, Status: database.MealStatusPendingReview, LoggedAt: time.Now()}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}

	yesterday := time.Now().UTC().AddDate(0, 0, -1).Truncate(time.Second)
	body, _ := json.Marshal(map[string]any{"logged_at": yesterday.Format(time.RFC3339)})
	r := mux.SetURLVars(httptest.NewRequest(http.MethodPut, "/api/food/meals/"+meal.ID.String()+"/confirm", bytes.NewBuffer(body)),
		map[string]string{"id": meal.ID.String()})

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.ConfirmMeal(w, withClaims(r, userID))

	var got database.FoodMeal
	json.NewDecoder(w.Body).Decode(&got) //nolint:errcheck
	if !got.LoggedAt.Equal(yesterday) {
		t.Errorf("expected logged_at %v, got %v", yesterday, got.LoggedAt)
	}
}

func TestConfirmMeal_NoNutritionRowWritten(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := database.FoodMeal{UserID: userID, Status: database.MealStatusPendingReview, LoggedAt: time.Now()}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}

	var before int64
	st.DB().Table("nutritions").Count(&before)

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.ConfirmMeal(w, withClaims(mealDetailRequest(http.MethodPut, meal.ID.String()), userID))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var after int64
	st.DB().Table("nutritions").Count(&after)
	if after != before {
		t.Errorf("expected no nutrition rows written, before=%d after=%d", before, after)
	}
}

func TestConfirmMeal_CrossUserReturns404(t *testing.T) {
	st := newFoodTestStorage(t)
	_, familyID := seedFoodUser(t, st)
	otherUserID := uuid.New()
	meal := database.FoodMeal{UserID: otherUserID, Status: database.MealStatusPendingReview, LoggedAt: time.Now()}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.ConfirmMeal(w, withClaims(mealDetailRequest(http.MethodPut, meal.ID.String()), uuid.New()))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// Only pending_review is a valid state to confirm from — not processing (a
// live or stale analysis racing its own status write), not failed (no
// usable items), not pending_clarification (items not yet resolved), and
// not an already-confirmed meal (items are frozen past that point, so a
// second confirm has nothing meaningful left to do).
func TestConfirmMeal_RejectsNonPendingReviewStatuses(t *testing.T) {
	for _, status := range []string{
		database.MealStatusProcessing,
		database.MealStatusFailed,
		database.MealStatusPendingClarification,
		database.MealStatusConfirmed,
	} {
		t.Run(status, func(t *testing.T) {
			st := newFoodTestStorage(t)
			userID, familyID := seedFoodUser(t, st)
			meal := database.FoodMeal{UserID: userID, Status: status, LoggedAt: time.Now()}
			meal.ID = uuid.New()
			meal.FamilyID = familyID
			if err := st.DB().Create(&meal).Error; err != nil {
				t.Fatalf("create meal: %v", err)
			}

			h := server.NewFoodHandlers(st, nil, t.TempDir())
			w := httptest.NewRecorder()
			h.ConfirmMeal(w, withClaims(mealDetailRequest(http.MethodPut, meal.ID.String()), userID))
			if w.Code != http.StatusConflict {
				t.Errorf("status=%s: expected 409, got %d: %s", status, w.Code, w.Body.String())
			}

			var persisted database.FoodMeal
			st.DB().First(&persisted, "id = ?", meal.ID) //nolint:errcheck
			if persisted.Status != status {
				t.Errorf("status=%s: expected status to stay unchanged, got %s", status, persisted.Status)
			}
		})
	}
}

// --- PatchMealItem ---

func itemPatchRequest(mealID, itemID string, body map[string]any) *http.Request {
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(http.MethodPatch, "/api/food/meals/"+mealID+"/items/"+itemID, bytes.NewBuffer(b))
	return mux.SetURLVars(r, map[string]string{"id": mealID, "item_id": itemID})
}

func TestPatchMealItem_BindToFdcID(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createUnresolvedMeal(t, st, userID, familyID)
	idx := buildUSDAIndex(t, usdaFood(7, "Chicken breast", 165))

	h := server.NewFoodHandlers(st, idx, t.TempDir())
	w := httptest.NewRecorder()
	r := itemPatchRequest(meal.ID.String(), meal.Items[0].ID.String(), map[string]any{"fdc_id": 7})
	h.PatchMealItem(w, withClaims(r, userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got database.FoodItem
	json.NewDecoder(w.Body).Decode(&got) //nolint:errcheck
	if got.MacroSource != database.MacroSourceReference {
		t.Errorf("expected reference, got %s", got.MacroSource)
	}
	// 165 kcal/100g * 1.0 (100g) = 165
	if got.Calories < 164.9 || got.Calories > 165.1 {
		t.Errorf("expected calories ~165, got %v", got.Calories)
	}
}

func TestPatchMealItem_SupplyMacrosDirectly(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createUnresolvedMeal(t, st, userID, familyID)

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	r := itemPatchRequest(meal.ID.String(), meal.Items[0].ID.String(), map[string]any{
		"manual": true, "calories": 300, "protein_grams": 25,
	})
	h.PatchMealItem(w, withClaims(r, userID))

	var got database.FoodItem
	json.NewDecoder(w.Body).Decode(&got) //nolint:errcheck
	if got.MacroSource != database.MacroSourceManual {
		t.Errorf("expected manual, got %s", got.MacroSource)
	}
	if got.Calories != 300 || got.ProteinGrams != 25 {
		t.Errorf("expected supplied macros to be stored as given, got %+v", got)
	}
}

func TestPatchMealItem_WeightOnlyChangeRescalesFromExistingBinding(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createUnresolvedMeal(t, st, userID, familyID)
	idx := buildUSDAIndex(t, usdaFood(9, "Rice, cooked", 130))

	h := server.NewFoodHandlers(st, idx, t.TempDir())

	// First bind to a reference food at 100g.
	bindReq := itemPatchRequest(meal.ID.String(), meal.Items[0].ID.String(), map[string]any{"fdc_id": 9, "weight_grams": 100})
	w1 := httptest.NewRecorder()
	h.PatchMealItem(w1, withClaims(bindReq, userID))
	if w1.Code != http.StatusOK {
		t.Fatalf("expected 200 on bind, got %d: %s", w1.Code, w1.Body.String())
	}

	// Then change only the weight — macros must rescale from the same profile.
	weightReq := itemPatchRequest(meal.ID.String(), meal.Items[0].ID.String(), map[string]any{"weight_grams": 200})
	w2 := httptest.NewRecorder()
	h.PatchMealItem(w2, withClaims(weightReq, userID))
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 on weight-only change, got %d: %s", w2.Code, w2.Body.String())
	}
	var got database.FoodItem
	json.NewDecoder(w2.Body).Decode(&got) //nolint:errcheck
	if got.MacroSource != database.MacroSourceReference {
		t.Errorf("expected binding to persist across a weight-only change, got %s", got.MacroSource)
	}
	// 130 kcal/100g * 2.0 (200g) = 260
	if got.Calories < 259.9 || got.Calories > 260.1 {
		t.Errorf("expected calories ~260 after rescale, got %v", got.Calories)
	}
}

func TestPatchMealItem_ConfirmedMealReturns409(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createUnresolvedMeal(t, st, userID, familyID)
	if err := st.DB().Model(&database.FoodMeal{}).Where("id = ?", meal.ID).
		Update("status", database.MealStatusConfirmed).Error; err != nil {
		t.Fatalf("confirm meal: %v", err)
	}

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	r := itemPatchRequest(meal.ID.String(), meal.Items[0].ID.String(), map[string]any{"manual": true, "calories": 1})
	h.PatchMealItem(w, withClaims(r, userID))

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPatchMealItem_CrossUserReturns404(t *testing.T) {
	st := newFoodTestStorage(t)
	_, familyID := seedFoodUser(t, st)
	otherUserID := uuid.New()
	meal := createUnresolvedMeal(t, st, otherUserID, familyID)

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	r := itemPatchRequest(meal.ID.String(), meal.Items[0].ID.String(), map[string]any{"manual": true, "calories": 1})
	h.PatchMealItem(w, withClaims(r, uuid.New()))

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// --- RetryMeal ---

func TestRetryMeal_FailedMealIsAccepted(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	dir := t.TempDir()

	meal := createFailedMealWithPhoto(t, st, dir, userID, familyID)
	fake := &vision.Fake{
		RecognizeResult: &vision.RecognizeResult{Items: []vision.Item{{Name: "Rice", WeightGrams: 100}}},
	}
	h := server.NewFoodHandlers(st, nil, dir).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.RetryMeal(w, withClaims(mealDetailRequest(http.MethodPost, meal.ID.String()), userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got database.FoodMeal
	json.NewDecoder(w.Body).Decode(&got) //nolint:errcheck
	if got.Status != database.MealStatusPendingReview {
		t.Errorf("expected pending_review after retry, got %s", got.Status)
	}
	if len(fake.RecognizeCalls) != 1 {
		t.Errorf("expected exactly one Recognize call, got %d", len(fake.RecognizeCalls))
	}
}

func TestRetryMeal_LiveProcessingRejectedWith409(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	meal := database.FoodMeal{UserID: userID, Status: database.MealStatusProcessing, LoggedAt: time.Now(), PhotoPath: "x/y.jpg"}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}

	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(&vision.Fake{}, 10<<20, time.Minute)
	w := httptest.NewRecorder()
	h.RetryMeal(w, withClaims(mealDetailRequest(http.MethodPost, meal.ID.String()), userID))

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409 for a live in-flight analysis, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRetryMeal_StaleProcessingIsAccepted(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	dir := t.TempDir()

	meal := createFailedMealWithPhoto(t, st, dir, userID, familyID)
	if err := st.DB().Model(&database.FoodMeal{}).Where("id = ?", meal.ID).
		Update("status", database.MealStatusProcessing).Error; err != nil {
		t.Fatalf("set processing: %v", err)
	}
	// Backdate updated_at past the (very short) vision timeout used below.
	past := time.Now().Add(-time.Hour)
	if err := st.DB().Model(&database.FoodMeal{}).Where("id = ?", meal.ID).
		Update("updated_at", past).Error; err != nil {
		t.Fatalf("backdate updated_at: %v", err)
	}

	fake := &vision.Fake{RecognizeResult: &vision.RecognizeResult{}}
	h := server.NewFoodHandlers(st, nil, dir).WithVision(fake, 10<<20, time.Second)
	w := httptest.NewRecorder()
	h.RetryMeal(w, withClaims(mealDetailRequest(http.MethodPost, meal.ID.String()), userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for stale processing, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRetryMeal_ConfirmedMealRejectedWith409(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := database.FoodMeal{UserID: userID, Status: database.MealStatusConfirmed, LoggedAt: time.Now(), PhotoPath: "x/y.jpg"}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.RetryMeal(w, withClaims(mealDetailRequest(http.MethodPost, meal.ID.String()), userID))

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestRetryMeal_ReplacesItemsNotAppends(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	dir := t.TempDir()
	meal := createFailedMealWithPhoto(t, st, dir, userID, familyID)

	// Seed a leftover item from a hypothetical earlier partial run.
	leftover := database.FoodItem{UserID: userID, MealID: meal.ID, Name: "Stale", MacroSource: database.MacroSourceNone}
	leftover.ID = uuid.New()
	leftover.FamilyID = familyID
	if err := st.DB().Create(&leftover).Error; err != nil {
		t.Fatalf("create leftover item: %v", err)
	}

	fake := &vision.Fake{
		RecognizeResult: &vision.RecognizeResult{Items: []vision.Item{{Name: "Fresh", WeightGrams: 50}}},
	}
	h := server.NewFoodHandlers(st, nil, dir).WithVision(fake, 10<<20, time.Second)
	w := httptest.NewRecorder()
	h.RetryMeal(w, withClaims(mealDetailRequest(http.MethodPost, meal.ID.String()), userID))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var items []database.FoodItem
	st.DB().Where("meal_id = ?", meal.ID).Find(&items) //nolint:errcheck
	if len(items) != 1 || items[0].Name != "Fresh" {
		t.Errorf("expected exactly the new item set, got %+v", items)
	}

	// Unscoped: a plain Find on FoodItem (which embeds TenantModel) silently
	// applies the same soft-delete filter as the bug this test needs to
	// catch, so it can't tell "hard-deleted" from "soft-deleted and hidden"
	// apart — exactly like TestDeleteCustomFood_Success couldn't, before
	// persistAnalysis was fixed to Unscoped().Delete().
	var allRowsEver int64
	st.DB().Unscoped().Model(&database.FoodItem{}).Where("meal_id = ?", meal.ID).Count(&allRowsEver)
	if allRowsEver != 1 {
		t.Errorf("expected the leftover row to be hard-deleted (1 row total), got %d rows including soft-deleted", allRowsEver)
	}
}

func TestRetryMeal_CrossUserReturns404(t *testing.T) {
	st := newFoodTestStorage(t)
	_, familyID := seedFoodUser(t, st)
	otherUserID := uuid.New()
	meal := database.FoodMeal{UserID: otherUserID, Status: database.MealStatusFailed, LoggedAt: time.Now(), PhotoPath: "x/y.jpg"}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.RetryMeal(w, withClaims(mealDetailRequest(http.MethodPost, meal.ID.String()), uuid.New()))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// createFailedMealWithPhoto creates a failed meal with a real stored photo
// (so RetryMeal's photos.Read succeeds), via the same photostorage.Store the
// handler under test will use.
func createFailedMealWithPhoto(t *testing.T, st database.Storage, dir string, userID, familyID uuid.UUID) database.FoodMeal {
	t.Helper()
	photos := photostorage.New(dir)
	mealID := uuid.New()
	relPath, err := photos.Save(bytes.NewReader(fakeJPEGBytes), 1<<20, userID, photostorage.OwnerMeal, mealID)
	if err != nil {
		t.Fatalf("save photo: %v", err)
	}
	meal := database.FoodMeal{UserID: userID, Status: database.MealStatusFailed, LoggedAt: time.Now(), PhotoPath: relPath}
	meal.ID = mealID
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}
	return meal
}
