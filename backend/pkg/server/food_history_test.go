package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/server"
	"github.com/ya-breeze/healthvault/pkg/vision"
)

func historyRequest(id uuid.UUID, body any) *http.Request {
	b, _ := json.Marshal(body)
	return mux.SetURLVars(httptest.NewRequest(http.MethodPost, "/history", strings.NewReader(string(b))), map[string]string{"id": id.String()})
}
func historyMeal(t *testing.T, w *httptest.ResponseRecorder, want int) database.FoodMeal {
	t.Helper()
	if w.Code != want {
		t.Fatalf("status=%d want=%d body=%s", w.Code, want, w.Body.String())
	}
	var m database.FoodMeal
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	return m
}
func seedHistorySource(t *testing.T, st database.Storage, user, family uuid.UUID) database.FoodMeal {
	t.Helper()
	m := createEstimatedItemMeal(t, st, user, family)
	m.Name = "Corrected vegetables"
	m.Status = database.MealStatusConfirmed
	m.CookingContext = &database.MealCookingContext{LowAddedSalt: true}
	m.LoggedAt = time.Now().UTC().Add(-time.Hour)
	// Current manual correction differs deliberately from persisted Recognize estimate.
	it := &m.Items[0]
	it.Name = "Vegetables"
	it.WeightGrams = 200
	it.MacroSource = database.MacroSourceManual
	it.Calories = 77
	it.ProteinGrams = 4
	it.FatGrams = .6
	it.CarbsGrams = 14
	it.SugarGrams = 3
	it.SodiumGrams = .02
	it.DietaryFiberGrams = 6
	it.SaturatedFatGrams = .1
	if err := st.DB().Save(it).Error; err != nil {
		t.Fatal(err)
	}
	m.Aggregate(m.Items)
	if err := st.DB().Omit("Items").Save(&m).Error; err != nil {
		t.Fatal(err)
	}
	return m
}
func TestRepeatMeal_FrozenCorrectedProfilesReplayWeightEditAndConfirm(t *testing.T) {
	st := newFoodTestStorage(t)
	user, family := seedFoodUser(t, st)
	src := seedHistorySource(t, st, user, family)
	h := server.NewFoodHandlers(st, nil, t.TempDir())
	key := uuid.NewString()
	repeat := func(id uuid.UUID) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.RepeatMeal(w, withClaims(historyRequest(id, map[string]string{"request_id": key}), user))
		return w
	}
	copied := historyMeal(t, repeat(src.ID), 201)
	if copied.ID == src.ID || copied.Items[0].ID == src.Items[0].ID || copied.Status != database.MealStatusPendingReview || copied.Calories != 0 || copied.PhotoPath != "" || copied.Description != "" || copied.HistoryVersion == "" {
		t.Fatalf("invalid repeat %+v", copied)
	}
	it := copied.Items[0]
	if it.Calories != 77 || it.CarbsGrams != 14 || it.HistoryOriginalSource != database.MacroSourceManual || it.MacroSource != database.MacroSourceHistory || it.HistorySourceMealID == nil || *it.HistorySourceMealID != src.ID {
		t.Fatalf("wrong profile %+v", it)
	}
	replay := historyMeal(t, repeat(src.ID), 200)
	if replay.ID != copied.ID {
		t.Fatal("replay duplicated meal")
	}
	if w := repeat(uuid.New()); w.Code != 409 {
		t.Fatalf("key reused for different source: %d", w.Code)
	}
	// Later source deletion cannot affect the independent frozen profile.
	if err := st.DB().Delete(&src).Error; err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.PatchMealItem(w, withClaims(itemPatchRequest(copied.ID.String(), it.ID.String(), map[string]any{"weight_grams": 100}), user))
	edited := historyMeal(t, w, 200)
	e := edited.Items[0]
	if e.Calories != 38.5 || e.CarbsGrams != 7 || e.DietaryFiberGrams != 3 || e.SaturatedFatGrams != .05 {
		t.Fatalf("snapshot did not rescale %+v", e)
	}
	w = httptest.NewRecorder()
	h.ConfirmMeal(w, withClaims(mealDetailRequest(http.MethodPut, copied.ID.String()), user))
	saved := historyMeal(t, w, 200)
	if saved.Calories != 38.5 || saved.Status != database.MealStatusConfirmed {
		t.Fatalf("history omitted from total %+v", saved)
	}
	w = httptest.NewRecorder()
	h.PatchMealItem(w, withClaims(itemPatchRequest(copied.ID.String(), it.ID.String(), map[string]any{"manual": true, "calories": 20}), user))
	manual := historyMeal(t, w, 200)
	if manual.Items[0].HistoryProfile != nil || manual.Items[0].HistorySourceMealID != nil || manual.Items[0].HistoryOriginalSource != "" {
		t.Fatal("manual replacement retained history")
	}
}
func TestPhotoHistory_ExplicitAcceptancePreservesFreshWeightsAndUnmatchedItem(t *testing.T) {
	st := newFoodTestStorage(t)
	user, family := seedFoodUser(t, st)
	src := seedHistorySource(t, st, user, family)
	fake := &vision.Fake{RecognizeResult: &vision.RecognizeResult{Items: []vision.Item{{Name: "Vegetables", WeightGrams: 300, EstimatedProfile: &database.NutrientProfile{CaloriesPer100g: 100, CarbsPer100g: 20}}, {Name: "Apple", WeightGrams: 100, EstimatedProfile: &database.NutrientProfile{CaloriesPer100g: 50}}}, HistoryMatch: &vision.HistoryMatch{SourceMealIndex: 0, Mappings: []vision.HistoryItemMapping{{ItemIndex: 0, SourceItemIndex: 0}}}}}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)
	w := httptest.NewRecorder()
	h.CreateMeal(w, withClaims(newMealUploadRequest(t, "food.jpg", fakeJPEGBytes), user))
	m := historyMeal(t, w, 201)
	if m.HistorySuggestion == nil || m.HistorySuggestion.SourceMealID != src.ID || len(m.HistorySuggestion.SourceItems) != 1 {
		t.Fatalf("missing suggestion %+v", m)
	}
	if m.Items[0].Calories != 300 || m.Items[0].MacroSource != database.MacroSourceEstimated {
		t.Fatal("history applied before acceptance")
	}
	if len(fake.RecognizeWithHistoryCalls) != 1 || fake.RecognizeWithHistoryCalls[0].History[0].Items[0].Profile.CarbsPer100g != 7 {
		t.Fatalf("incorrect history context %+v", fake.RecognizeWithHistoryCalls)
	}
	w = httptest.NewRecorder()
	h.ResolveMealHistory(w, withClaims(historyRequest(m.ID, map[string]string{"action": "apply", "expected_version": m.HistoryVersion}), user))
	applied := historyMeal(t, w, 200)
	var reused, apple database.FoodItem
	for _, it := range applied.Items {
		if it.Name == "Apple" {
			apple = it
		} else {
			reused = it
		}
	}
	if reused.WeightGrams != 300 || reused.Calories != 115.5 || reused.CarbsGrams != 21 || reused.HistoryOriginalSource != database.MacroSourceManual || apple.Calories != 50 || applied.HistorySuggestion != nil || applied.Status != database.MealStatusPendingReview {
		t.Fatalf("incorrect acceptance %+v", applied)
	}
	w = httptest.NewRecorder()
	h.ResolveMealHistory(w, withClaims(historyRequest(m.ID, map[string]string{"action": "apply", "expected_version": m.HistoryVersion}), user))
	if w.Code != 409 {
		t.Fatal("old acceptance replay must conflict")
	}
}
func TestPhotoHistory_StaleSourceTargetDismissAndInvalidMappings(t *testing.T) {
	for _, mode := range []string{"source", "target", "dismiss", "invalid"} {
		t.Run(mode, func(t *testing.T) {
			st := newFoodTestStorage(t)
			user, family := seedFoodUser(t, st)
			src := seedHistorySource(t, st, user, family)
			match := &vision.HistoryMatch{SourceMealIndex: 0, Mappings: []vision.HistoryItemMapping{{ItemIndex: 0, SourceItemIndex: 0}}}
			if mode == "invalid" {
				match.Mappings = append(match.Mappings, match.Mappings[0])
			}
			fake := &vision.Fake{RecognizeResult: &vision.RecognizeResult{Items: []vision.Item{{Name: "Vegetables", WeightGrams: 100, EstimatedProfile: &database.NutrientProfile{CaloriesPer100g: 50}}}, HistoryMatch: match}}
			h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)
			w := httptest.NewRecorder()
			h.CreateMeal(w, withClaims(newMealUploadRequest(t, "food.jpg", fakeJPEGBytes), user))
			m := historyMeal(t, w, 201)
			if mode == "invalid" {
				if m.HistorySuggestion != nil {
					t.Fatal("duplicate mapping accepted")
				}
				return
			}
			action := "apply"
			if mode == "source" {
				if err := st.DB().Model(&database.FoodItem{}).Where("id = ?", src.Items[0].ID).Update("calories", 999).Error; err != nil {
					t.Fatal(err)
				}
			}
			if mode == "target" {
				w = httptest.NewRecorder()
				h.PatchMealItem(w, withClaims(itemPatchRequest(m.ID.String(), m.Items[0].ID.String(), map[string]any{"weight_grams": 150}), user))
				historyMeal(t, w, 200)
			}
			if mode == "dismiss" {
				action = "dismiss"
			}
			w = httptest.NewRecorder()
			h.ResolveMealHistory(w, withClaims(historyRequest(m.ID, map[string]string{"action": action, "expected_version": m.HistoryVersion}), user))
			if mode == "dismiss" {
				out := historyMeal(t, w, 200)
				if out.HistorySuggestion != nil || out.Items[0].MacroSource != database.MacroSourceEstimated {
					t.Fatal("dismiss changed macros")
				}
			} else if w.Code != 409 {
				t.Fatalf("stale %s accepted: %d %s", mode, w.Code, w.Body.String())
			}
		})
	}
}
func TestRepeatMeal_IsolationAndEligibilityRollback(t *testing.T) {
	st := newFoodTestStorage(t)
	user, family := seedFoodUser(t, st)
	source := seedHistorySource(t, st, user, family)
	h := server.NewFoodHandlers(st, nil, t.TempDir())
	for i, mode := range []string{"other", "old", "future", "unconfirmed", "zero"} {
		t.Run(mode, func(t *testing.T) {
			src := source
			caller := user
			status := 409
			switch mode {
			case "other":
				caller = uuid.New()
				status = 404
			case "old":
				st.DB().Model(&src).Update("logged_at", time.Now().UTC().AddDate(0, 0, -8))
			case "future":
				st.DB().Model(&src).Update("logged_at", time.Now().UTC().Add(time.Hour))
			case "unconfirmed":
				st.DB().Model(&src).Updates(map[string]any{"logged_at": time.Now().UTC(), "status": database.MealStatusPendingReview})
			case "zero":
				status = 422
				st.DB().Model(&src).Update("status", database.MealStatusConfirmed)
				st.DB().Model(&database.FoodItem{}).Where("id = ?", src.Items[0].ID).Update("weight_grams", 0)
			}
			w := httptest.NewRecorder()
			h.RepeatMeal(w, withClaims(historyRequest(src.ID, map[string]string{"request_id": fmt.Sprintf("key-%d", i)}), caller))
			if w.Code != status {
				t.Fatalf("got %d %s", w.Code, w.Body.String())
			}
			var count int64
			st.DB().Model(&database.FoodMeal{}).Where("request_id = ?", fmt.Sprintf("key-%d", i)).Count(&count)
			if count != 0 {
				t.Fatal("failed repeat left row")
			}
		})
	}
}

func TestPhotoHistory_ConcurrentChangeBeforeWriterClaimRejectsAcceptance(t *testing.T) {
	for _, change := range []string{"source", "target"} {
		t.Run(change, func(t *testing.T) {
			st := newFileFoodTestStorage(t)
			user, family := seedFoodUser(t, st)
			src := seedHistorySource(t, st, user, family)
			fake := &vision.Fake{RecognizeResult: &vision.RecognizeResult{Items: []vision.Item{{Name: "vegetables", WeightGrams: 100, EstimatedProfile: &database.NutrientProfile{CaloriesPer100g: 50}}}, HistoryMatch: &vision.HistoryMatch{SourceMealIndex: 0, Mappings: []vision.HistoryItemMapping{{ItemIndex: 0, SourceItemIndex: 0}}}}}
			h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)
			w := httptest.NewRecorder()
			h.CreateMeal(w, withClaims(newMealUploadRequest(t, "meal.jpg", fakeJPEGBytes), user))
			meal := historyMeal(t, w, 201)
			registerRaceSimulation(t, st.DB(), func() {
				db, err := st.DB().DB()
				if err != nil {
					t.Fatal(err)
				}
				if change == "source" {
					_, err = db.Exec("UPDATE food_items SET calories = 900 WHERE id = ?", src.Items[0].ID)
				} else {
					_, err = db.Exec("UPDATE food_meals SET status = ? WHERE id = ?", database.MealStatusProcessing, meal.ID)
				}
				if err != nil {
					t.Fatal(err)
				}
			})
			w = httptest.NewRecorder()
			h.ResolveMealHistory(w, withClaims(historyRequest(meal.ID, map[string]string{"action": "apply", "expected_version": meal.HistoryVersion}), user))
			if w.Code != 409 {
				t.Fatalf("concurrent %s write overwritten: %d %s", change, w.Code, w.Body.String())
			}
			var item database.FoodItem
			if err := st.DB().First(&item, "id = ?", meal.Items[0].ID).Error; err != nil {
				t.Fatal(err)
			}
			if item.MacroSource != database.MacroSourceEstimated || item.Calories != 50 {
				t.Fatal("conflict partially applied history")
			}
		})
	}
}

func TestPhotoHistory_ClarificationRetainsRecentCandidatesAndOffersProposal(t *testing.T) {
	st := newFoodTestStorage(t)
	user, family := seedFoodUser(t, st)
	source := seedHistorySource(t, st, user, family)
	fake := &vision.Fake{RecognizeResult: &vision.RecognizeResult{Items: []vision.Item{{Name: "Vegetables", WeightGrams: 200}}, ClarificationQuestions: []string{"Any oil?"}}, ClarifyResult: &vision.RecognizeResult{Items: []vision.Item{{Name: "Vegetables", WeightGrams: 150, EstimatedProfile: &database.NutrientProfile{CaloriesPer100g: 50}}}, HistoryMatch: &vision.HistoryMatch{SourceMealIndex: 0, Mappings: []vision.HistoryItemMapping{{ItemIndex: 0, SourceItemIndex: 0}}}}}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)
	w := httptest.NewRecorder()
	h.CreateMeal(w, withClaims(newMealUploadRequest(t, "meal.jpg", fakeJPEGBytes), user))
	meal := historyMeal(t, w, 201)
	if meal.Status != database.MealStatusPendingClarification {
		t.Fatal("missing clarification")
	}
	w = httptest.NewRecorder()
	h.ClarifyMeal(w, withClaims(clarifyRequest(meal.ID.String(), []string{"No oil"}), user))
	review := historyMeal(t, w, 200)
	if review.Status != database.MealStatusPendingReview || review.HistorySuggestion == nil || review.HistorySuggestion.SourceMealID != source.ID || review.Items[0].WeightGrams != 150 {
		t.Fatalf("photo clarification lost history or fresh weight: %+v", review)
	}
}

func TestRepeatMeal_ConcurrentSameKeyCreatesOneIndependentDraft(t *testing.T) {
	st := newFileFoodTestStorage(t)
	user, family := seedFoodUser(t, st)
	source := seedHistorySource(t, st, user, family)
	h := server.NewFoodHandlers(st, nil, t.TempDir())
	key := uuid.NewString()
	start := make(chan struct{})
	results := make(chan *httptest.ResponseRecorder, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			w := httptest.NewRecorder()
			h.RepeatMeal(w, withClaims(historyRequest(source.ID, map[string]string{"request_id": key}), user))
			results <- w
		}()
	}
	close(start)
	first, second := <-results, <-results
	if first.Code != 200 && first.Code != 201 {
		t.Fatalf("first %d %s", first.Code, first.Body.String())
	}
	if second.Code != 200 && second.Code != 201 {
		t.Fatalf("second %d %s", second.Code, second.Body.String())
	}
	a := historyMeal(t, first, first.Code)
	b := historyMeal(t, second, second.Code)
	if a.ID != b.ID {
		t.Fatal("simultaneous replay created two meals")
	}
	var count int64
	st.DB().Model(&database.FoodMeal{}).Where("user_id = ? AND request_id = ?", user, key).Count(&count)
	if count != 1 {
		t.Fatalf("created %d rows", count)
	}
}

func TestHistoryReferenceRebindClearsSnapshotAndCrossUserAcceptanceIsHidden(t *testing.T) {
	st := newFileFoodTestStorage(t)
	user, family := seedFoodUser(t, st)
	source := seedHistorySource(t, st, user, family)
	fake := &vision.Fake{RecognizeResult: &vision.RecognizeResult{Items: []vision.Item{{Name: "Vegetables", WeightGrams: 150, EstimatedProfile: &database.NutrientProfile{CaloriesPer100g: 50}}}, HistoryMatch: &vision.HistoryMatch{SourceMealIndex: 0, Mappings: []vision.HistoryItemMapping{{ItemIndex: 0, SourceItemIndex: 0}}}}}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)
	w := httptest.NewRecorder()
	h.CreateMeal(w, withClaims(newMealUploadRequest(t, "meal.jpg", fakeJPEGBytes), user))
	meal := historyMeal(t, w, 201)
	w = httptest.NewRecorder()
	h.ResolveMealHistory(w, withClaims(historyRequest(meal.ID, map[string]string{"action": "apply", "expected_version": meal.HistoryVersion}), uuid.New()))
	if w.Code != 404 {
		t.Fatalf("other user got %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.RepeatMeal(w, withClaims(historyRequest(source.ID, map[string]string{"request_id": uuid.NewString()}), user))
	copied := historyMeal(t, w, 201)
	cf := database.CustomFood{UserID: user, Name: "Replacement", CaloriesPer100g: 200}
	cf.ID = uuid.New()
	cf.FamilyID = family
	if err := st.DB().Create(&cf).Error; err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	h.PatchMealItem(w, withClaims(itemPatchRequest(copied.ID.String(), copied.Items[0].ID.String(), map[string]any{"custom_food_id": cf.ID}), user))
	out := historyMeal(t, w, 200)
	it := out.Items[0]
	if it.MacroSource != database.MacroSourceReference || it.HistoryProfile != nil || it.HistoryOriginalSource != "" || it.HistorySourceMealID != nil || it.Calories != 400 {
		t.Fatalf("reference replacement retained copied profile: %+v", it)
	}
}
