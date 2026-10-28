package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/vision"
	"github.com/ya-breeze/kin-core/auth"
)

func TestHistoryIdentity_RejectionDiagnosticOmitsFoodContents(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	source := database.FoodMeal{Items: []database.FoodItem{{Name: "private source food", WeightGrams: 100, Calories: 16, MacroSource: database.MacroSourceEstimated}}}
	source.ID, source.Items[0].ID = uuid.New(), uuid.New()
	target := database.FoodItem{Name: "private target food", WeightGrams: 150}
	target.ID, target.MealID = uuid.New(), uuid.New()
	match := &vision.HistoryMatch{SourceMealIndex: 0, Mappings: []vision.HistoryItemMapping{{ItemIndex: 0, SourceItemIndex: 0}}}
	if buildHistorySuggestion(match, []database.FoodMeal{source}, []database.FoodItem{target}) != nil {
		t.Fatal("unrelated foods accepted")
	}
	var row map[string]any
	if err := json.Unmarshal(output.Bytes(), &row); err != nil {
		t.Fatalf("expected one structured diagnostic: %v", err)
	}
	for key, value := range map[string]string{"stage": "suggest", "reason": "food_identity", "meal_id": target.MealID.String(), "item_id": target.ID.String(), "source_meal_id": source.ID.String(), "source_item_id": source.Items[0].ID.String()} {
		if row[key] != value {
			t.Errorf("%s=%v, want %s", key, row[key], value)
		}
	}
	if strings.Contains(output.String(), "private") {
		t.Fatal("food contents leaked into diagnostic")
	}
}

func TestHistoryIdentity_SuggestionRequiresMatchingFoods(t *testing.T) {
	for _, tc := range []struct {
		name           string
		source, target database.FoodItem
		want           bool
	}{
		{"unrelated canonical names", database.FoodItem{Name: "огурец", CanonicalName: "cucumber"}, database.FoodItem{Name: "шарлотка", CanonicalName: "apple pie"}, false},
		{"unrelated legacy names", database.FoodItem{Name: "lettuce"}, database.FoodItem{Name: "cottage cheese pancakes"}, false},
		{"canonical names across languages", database.FoodItem{Name: "сырники", CanonicalName: "cottage cheese pancakes"}, database.FoodItem{Name: "tvarohové lívance", CanonicalName: "cottage cheese pancakes"}, true},
		{"case and whitespace", database.FoodItem{Name: "old", CanonicalName: "  APPLE\tPIE "}, database.FoodItem{Name: "new", CanonicalName: "apple  pie"}, true},
		{"legacy display fallback", database.FoodItem{Name: " Apple\tPie "}, database.FoodItem{Name: "apple pie", CanonicalName: "apple pie"}, true},
		{"both legacy displays", database.FoodItem{Name: " Apple Pie "}, database.FoodItem{Name: "apple  pie"}, true},
		{"canonical conflict beats equal display", database.FoodItem{Name: "food", CanonicalName: "cucumber"}, database.FoodItem{Name: "food", CanonicalName: "apple pie"}, false},
		{"empty names", database.FoodItem{}, database.FoodItem{}, false},
		{"different preparation", database.FoodItem{Name: "potato", Preparation: "boiled"}, database.FoodItem{Name: "potato", Preparation: "fried"}, false},
		{"different state", database.FoodItem{Name: "rice", State: "raw"}, database.FoodItem{Name: "rice", State: "cooked"}, false},
		{"normalized preparation and state", database.FoodItem{Name: "rice", Preparation: " BOILED ", State: " Cooked "}, database.FoodItem{Name: "rice", Preparation: "boiled", State: "cooked"}, true},
		{"unknown preparation allowed", database.FoodItem{Name: "rice"}, database.FoodItem{Name: "rice", Preparation: "boiled", State: "cooked"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.source.ID = uuid.New()
			tc.source.WeightGrams = 100
			tc.source.Calories = 100
			tc.source.MacroSource = database.MacroSourceEstimated
			tc.target.ID = uuid.New()
			tc.target.WeightGrams = 150
			source := database.FoodMeal{Items: []database.FoodItem{tc.source}}
			source.ID = uuid.New()
			match := &vision.HistoryMatch{SourceMealIndex: 0, Mappings: []vision.HistoryItemMapping{{ItemIndex: 0, SourceItemIndex: 0}}}
			got := buildHistorySuggestion(match, []database.FoodMeal{source}, []database.FoodItem{tc.target})
			if (got != nil) != tc.want {
				t.Fatalf("suggestion accepted=%v, want %v", got != nil, tc.want)
			}
		})
	}
}

func TestHistoryIdentity_RecognitionRejectsUnrelatedProfilesAndPreservesEstimates(t *testing.T) {
	st, user, family := newDashboardInternalStorage(t)
	h := NewFoodHandlers(st, nil, t.TempDir())
	meal := database.FoodMeal{UserID: user, Status: database.MealStatusProcessing, LoggedAt: time.Now().UTC()}
	meal.ID, meal.FamilyID = uuid.New(), family
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatal(err)
	}
	source := database.FoodMeal{UserID: user, Status: database.MealStatusConfirmed}
	source.ID = uuid.New()
	for _, food := range []struct {
		name string
		kcal float64
	}{{"cucumber", 16}, {"lettuce", 15}} {
		it := database.FoodItem{Name: food.name, CanonicalName: food.name, WeightGrams: 100, Calories: food.kcal, MacroSource: database.MacroSourceEstimated}
		it.ID = uuid.New()
		source.Items = append(source.Items, it)
	}
	recognized := &vision.RecognizeResult{Items: []vision.Item{
		{Name: "шарлотка", CanonicalName: "apple pie", WeightGrams: 150, EstimatedProfile: &database.NutrientProfile{CaloriesPer100g: 165}},
		{Name: "творожные сырники", CanonicalName: "cottage cheese pancakes", WeightGrams: 200, EstimatedProfile: &database.NutrientProfile{CaloriesPer100g: 175}},
	}, HistoryMatch: &vision.HistoryMatch{SourceMealIndex: 0, Mappings: []vision.HistoryItemMapping{{ItemIndex: 0, SourceItemIndex: 0}, {ItemIndex: 1, SourceItemIndex: 1}}}}
	if err := h.processRecognition(context.Background(), &meal, recognized, meal.UpdatedAt, false, "ru", []database.FoodMeal{source}); err != nil {
		t.Fatal(err)
	}
	got, err := h.loadOwnedMeal(meal.ID, user)
	if err != nil {
		t.Fatal(err)
	}
	if got.HistorySuggestion != nil {
		t.Error("unrelated vegetable profiles were offered for pastry items")
	}
	if got.Status != database.MealStatusPendingReview {
		t.Fatalf("status=%s", got.Status)
	}
	preview := *got
	preview.Aggregate(got.Items)
	if preview.Calories != 597.5 {
		t.Fatalf("independent estimates changed: calories=%v", preview.Calories)
	}
	for _, it := range got.Items {
		if it.MacroSource != database.MacroSourceEstimated || it.HistoryProfile != nil {
			t.Fatalf("estimate replaced: %+v", it)
		}
		if (it.CanonicalName == "apple pie" && it.WeightGrams != 150) || (it.CanonicalName == "cottage cheese pancakes" && it.WeightGrams != 200) {
			t.Fatalf("portion changed: %+v", it)
		}
	}
}

func TestHistoryIdentity_ApplyRejectsLegacyUnsafeSuggestionAtomically(t *testing.T) {
	st, user, family := newDashboardInternalStorage(t)
	h := NewFoodHandlers(st, nil, t.TempDir())
	seed := func(status string, names []string) database.FoodMeal {
		meal := database.FoodMeal{UserID: user, Status: status, LoggedAt: time.Now().UTC().Add(-time.Hour)}
		meal.ID, meal.FamilyID = uuid.New(), family
		if err := st.DB().Create(&meal).Error; err != nil {
			t.Fatal(err)
		}
		for _, name := range names {
			it := database.FoodItem{UserID: user, MealID: meal.ID, Name: name, CanonicalName: name, WeightGrams: 150}
			it.ID, it.FamilyID = uuid.New(), family
			it.SetEstimatedProfile(&database.NutrientProfile{CaloriesPer100g: 200})
			it.ApplyEstimatedProfile()
			if err := st.DB().Create(&it).Error; err != nil {
				t.Fatal(err)
			}
		}
		loaded, err := loadHistoryMeal(st.DB(), meal.ID, user)
		if err != nil {
			t.Fatal(err)
		}
		return *loaded
	}
	source := seed(database.MealStatusConfirmed, []string{"apple pie", "lettuce"})
	target := seed(database.MealStatusPendingReview, []string{"apple pie", "cottage cheese pancakes"})
	// Store a pre-fix snapshot directly: first mapping is valid, second is unsafe.
	// This catches implementations that apply the first update before rejecting the next.
	find := func(items []database.FoodItem, name string) database.FoodItem {
		for _, it := range items {
			if it.Name == name {
				return it
			}
		}
		t.Fatalf("missing %s", name)
		return database.FoodItem{}
	}
	suggestion := &database.MealHistorySuggestion{SourceMealID: source.ID, SourceVersion: source.HistoryVersion}
	for _, pair := range [][2]string{{"apple pie", "apple pie"}, {"cottage cheese pancakes", "lettuce"}} {
		to, from := find(target.Items, pair[0]), find(source.Items, pair[1])
		profile, _ := from.CurrentProfile()
		suggestion.Mappings = append(suggestion.Mappings, database.MealHistoryMapping{ItemID: to.ID, SourceItemID: from.ID, SourceName: from.Name, Profile: profile, OriginalSource: database.MacroSourceEstimated})
	}
	if err := st.DB().Model(&database.FoodMeal{}).Where("id = ?", target.ID).UpdateColumn("history_suggestion", historySuggestionJSON(suggestion)).Error; err != nil {
		t.Fatal(err)
	}
	before, err := loadHistoryMeal(st.DB(), target.ID, user)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"action": "apply", "expected_version": before.HistoryVersion})
	r := mux.SetURLVars(httptest.NewRequest(http.MethodPost, "/history", strings.NewReader(string(body))), map[string]string{"id": target.ID.String()})
	r = r.WithContext(context.WithValue(r.Context(), ClaimsContextKey, &auth.Claims{UserID: user}))
	w := httptest.NewRecorder()
	h.ResolveMealHistory(w, r)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "foods do not match") {
		t.Errorf("status=%d, want explicit 422 mismatch; body=%s", w.Code, w.Body.String())
	}
	after, err := loadHistoryMeal(st.DB(), target.ID, user)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Error("unsafe suggestion changed target meal or partially applied nutrition")
	}
}
