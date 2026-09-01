package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/server"
	"github.com/ya-breeze/healthvault/pkg/vision"
)

var errClarifyBoom = errors.New("vision provider unavailable")

func clarifyRequest(mealID string, answers []string) *http.Request {
	body, _ := json.Marshal(map[string]any{"answers": answers})
	r := httptest.NewRequest(http.MethodPost, "/api/food/meals/"+mealID+"/clarify", bytes.NewBuffer(body))
	return mux.SetURLVars(r, map[string]string{"id": mealID})
}

// createPendingClarificationMeal creates a meal in pending_clarification with
// one round-1 pending question, mirroring what analyzeMeal/processRecognition
// would have written.
func createPendingClarificationMeal(t *testing.T, st database.Storage, userID, familyID uuid.UUID) database.FoodMeal {
	t.Helper()
	log, err := json.Marshal([]database.ClarifyEntry{
		{Round: 1, Question: "Is this sauce cream-based or tomato-based?", Answer: ""},
	})
	if err != nil {
		t.Fatalf("marshal clarify log: %v", err)
	}
	meal := database.FoodMeal{
		UserID: userID, Status: database.MealStatusPendingClarification, LoggedAt: time.Now(),
		ClarifyRound: 0, ClarifyLog: string(log),
	}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}
	item := database.FoodItem{
		UserID: userID, MealID: meal.ID, Name: "Sauce", WeightGrams: 30, MacroSource: database.MacroSourceNone,
	}
	item.ID = uuid.New()
	item.FamilyID = familyID
	if err := st.DB().Create(&item).Error; err != nil {
		t.Fatalf("create item: %v", err)
	}
	return meal
}

func TestClarifyMeal_AnswerResolvesToReviewCarriesNoImage(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createPendingClarificationMeal(t, st, userID, familyID)

	fake := &vision.Fake{
		ClarifyResult: &vision.RecognizeResult{
			Items: []vision.Item{{Name: "Tomato sauce", Preparation: "simmered", WeightGrams: 30, Confidence: 0.8}},
		},
	}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.ClarifyMeal(w, withClaims(clarifyRequest(meal.ID.String(), []string{"Tomato-based"}), userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got database.FoodMeal
	json.NewDecoder(w.Body).Decode(&got) //nolint:errcheck
	if got.Status != database.MealStatusPendingReview {
		t.Errorf("expected pending_review, got %s", got.Status)
	}
	if got.ClarifyRound != 1 {
		t.Errorf("expected clarify_round 1, got %d", got.ClarifyRound)
	}

	if len(fake.ClarifyCalls) != 1 {
		t.Fatalf("expected exactly one Clarify call, got %d", len(fake.ClarifyCalls))
	}
	call := fake.ClarifyCalls[0]
	if len(call.History) != 1 || call.History[0].Answer != "Tomato-based" {
		t.Errorf("expected the history to carry the submitted answer, got %+v", call.History)
	}
	if len(call.PriorItems) != 1 || call.PriorItems[0].Name != "Sauce" {
		t.Errorf("expected prior items to be replayed, got %+v", call.PriorItems)
	}
	// Clarify's signature carries no image parameter at all — a structural
	// guarantee that no photo bytes can be sent on a clarify round.
}

// An item's persisted estimate (set by the original photo-based Recognize
// call) is fed to Clarify as context and, when the round concludes with
// still no candidate match, used as the item's fallback macros — even
// though Clarify's own response (text-only, no photo access) does not
// reproduce a fresh estimate itself.
func TestClarifyMeal_EstimatePersistsThroughClarificationRound(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createPendingClarificationMeal(t, st, userID, familyID)

	// Simulate the original Recognize call having already persisted an
	// estimate on the pending item, the way unresolvedItemsFrom does.
	if err := st.DB().Model(&database.FoodItem{}).Where("meal_id = ?", meal.ID).Updates(map[string]any{
		"has_estimate": true, "estimated_calories_per100g": 90, "estimated_protein_per100g": 3,
	}).Error; err != nil {
		t.Fatalf("seed estimate: %v", err)
	}

	fake := &vision.Fake{
		// Deliberately carries no estimated_profile of its own — Clarify is
		// text-only and the fake mirrors that by not fabricating one.
		ClarifyResult: &vision.RecognizeResult{
			Items: []vision.Item{{Name: "Tomato sauce", Preparation: "simmered", WeightGrams: 30, Confidence: 0.8}},
		},
	}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.ClarifyMeal(w, withClaims(clarifyRequest(meal.ID.String(), []string{"Tomato-based"}), userID))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if len(fake.ClarifyCalls) != 1 || len(fake.ClarifyCalls[0].PriorItems) != 1 {
		t.Fatalf("expected one Clarify call with one prior item, got %+v", fake.ClarifyCalls)
	}
	priorEstimate := fake.ClarifyCalls[0].PriorItems[0].EstimatedProfile
	if priorEstimate == nil || priorEstimate.CaloriesPer100g != 90 {
		t.Fatalf("expected the persisted estimate replayed to Clarify as context, got %+v", priorEstimate)
	}

	var got database.FoodMeal
	json.NewDecoder(w.Body).Decode(&got) //nolint:errcheck
	if len(got.Items) != 1 {
		t.Fatalf("expected one item, got %+v", got.Items)
	}
	item := got.Items[0]
	if item.MacroSource != database.MacroSourceEstimated {
		t.Fatalf("expected the item to fall back to its persisted estimate after clarification, got %s", item.MacroSource)
	}
	// 90 kcal/100g * 0.3 (30g) = 27
	if item.Calories < 26.9 || item.Calories > 27.1 {
		t.Errorf("expected calories ~27 from the carried-forward estimate, got %v", item.Calories)
	}
}

// Regression: Clarify is text-only but shares Recognize's prompt/schema, so
// its response can still carry its own non-null estimated_profile — a guess
// made with no photo access. That guess must be discarded in favor of the
// persisted photo-grounded estimate, not silently trusted over it.
func TestClarifyMeal_ConflictingClarifyEstimateIsDiscarded(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createPendingClarificationMeal(t, st, userID, familyID)

	if err := st.DB().Model(&database.FoodItem{}).Where("meal_id = ?", meal.ID).Updates(map[string]any{
		"has_estimate": true, "estimated_calories_per100g": 90, "estimated_protein_per100g": 3,
	}).Error; err != nil {
		t.Fatalf("seed estimate: %v", err)
	}

	fake := &vision.Fake{
		// A conflicting, photo-blind guess wildly different from the
		// persisted 90 kcal/100g estimate.
		ClarifyResult: &vision.RecognizeResult{
			Items: []vision.Item{{
				Name: "Tomato sauce", Preparation: "simmered", WeightGrams: 30, Confidence: 0.8,
				EstimatedProfile: &database.NutrientProfile{CaloriesPer100g: 500, ProteinPer100g: 500},
			}},
		},
	}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.ClarifyMeal(w, withClaims(clarifyRequest(meal.ID.String(), []string{"Tomato-based"}), userID))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var got database.FoodMeal
	json.NewDecoder(w.Body).Decode(&got) //nolint:errcheck
	if len(got.Items) != 1 {
		t.Fatalf("expected one item, got %+v", got.Items)
	}
	item := got.Items[0]
	if item.MacroSource != database.MacroSourceEstimated {
		t.Fatalf("expected macro_source estimated, got %s", item.MacroSource)
	}
	// 90 kcal/100g * 0.3 (30g) = 27, not 500 * 0.3 = 150 from Clarify's own guess.
	if item.Calories < 26.9 || item.Calories > 27.1 {
		t.Errorf("expected calories ~27 from the persisted estimate, not Clarify's own conflicting guess, got %v", item.Calories)
	}
}

// Regression: a same-count clarify round must not misattribute a persisted
// estimate across a reorder. Two prior items, each with a distinct estimate,
// come back from Clarify in swapped order and under unrelated names
// ("grilled chicken" / "mixed salad" share no substring either way). If
// carryForwardPriorFields matched purely by position, each item would silently
// inherit the *other's* estimate; the fix requires the name at a position to
// still be plausibly the same item, so neither should resolve to
// macro_source=estimated here (no candidate match, no carried estimate).
func TestClarifyMeal_ReorderedItemsDoNotMisattributeEstimates(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	log, err := json.Marshal([]database.ClarifyEntry{
		{Round: 1, Question: "What are these two items?", Answer: ""},
	})
	if err != nil {
		t.Fatalf("marshal clarify log: %v", err)
	}
	meal := database.FoodMeal{
		UserID: userID, Status: database.MealStatusPendingClarification, LoggedAt: time.Now(),
		ClarifyRound: 0, ClarifyLog: string(log),
	}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}
	chicken := database.FoodItem{
		UserID: userID, MealID: meal.ID, Name: "grilled chicken", WeightGrams: 150, MacroSource: database.MacroSourceNone,
	}
	chicken.ID = uuid.New()
	chicken.FamilyID = familyID
	if err := st.DB().Create(&chicken).Error; err != nil {
		t.Fatalf("create chicken item: %v", err)
	}
	salad := database.FoodItem{
		UserID: userID, MealID: meal.ID, Name: "mixed salad", WeightGrams: 100, MacroSource: database.MacroSourceNone,
	}
	salad.ID = uuid.New()
	salad.FamilyID = familyID
	if err := st.DB().Create(&salad).Error; err != nil {
		t.Fatalf("create salad item: %v", err)
	}
	if err := st.DB().Model(&database.FoodItem{}).Where("id = ?", chicken.ID).Updates(map[string]any{
		"has_estimate": true, "estimated_calories_per100g": 200,
	}).Error; err != nil {
		t.Fatalf("seed chicken estimate: %v", err)
	}
	if err := st.DB().Model(&database.FoodItem{}).Where("id = ?", salad.ID).Updates(map[string]any{
		"has_estimate": true, "estimated_calories_per100g": 30,
	}).Error; err != nil {
		t.Fatalf("seed salad estimate: %v", err)
	}

	fake := &vision.Fake{
		// Deliberately no estimated_profile of its own (text-only round), and
		// deliberately in swapped order relative to the items' creation order.
		ClarifyResult: &vision.RecognizeResult{
			Items: []vision.Item{
				{Name: "mixed salad", WeightGrams: 100, Confidence: 0.8},
				{Name: "grilled chicken", WeightGrams: 150, Confidence: 0.8},
			},
		},
	}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.ClarifyMeal(w, withClaims(clarifyRequest(meal.ID.String(), []string{"Chicken and salad"}), userID))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(fake.ClarifyCalls) != 1 || len(fake.ClarifyCalls[0].PriorItems) != 2 {
		t.Fatalf("expected one Clarify call with two prior items, got %+v", fake.ClarifyCalls)
	}
	if fake.ClarifyCalls[0].PriorItems[0].Name != "grilled chicken" {
		t.Skip("prior item order was not [chicken, salad] as this test assumes; skipping")
	}

	var got database.FoodMeal
	json.NewDecoder(w.Body).Decode(&got) //nolint:errcheck
	if len(got.Items) != 2 {
		t.Fatalf("expected two items, got %+v", got.Items)
	}
	for _, it := range got.Items {
		if it.MacroSource == database.MacroSourceEstimated {
			t.Errorf("expected no misattributed estimate after a reorder, but item %q resolved via macro_source=estimated with calories %v",
				it.Name, it.Calories)
		}
	}
}

// Regression coverage for split-distinct-plated-foods (tasks.md 3.1/4.3): a
// Clarify response is free to return a different item count than the prior
// items it was given (e.g. clarification reveals a single "some sauce" item
// was actually two distinct plated foods), and processRecognition persists
// one FoodItem per item in that response — there is no separate item-count
// guidance elsewhere in the clarify path that would re-merge them back down
// to the prior count.
func TestClarifyMeal_MultiItemClarifyResponseCreatesOneFoodItemPerItem(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createPendingClarificationMeal(t, st, userID, familyID)

	fake := &vision.Fake{
		ClarifyResult: &vision.RecognizeResult{
			Items: []vision.Item{
				{Name: "stewed cabbage", WeightGrams: 150, Confidence: 0.8},
				{Name: "baked white fish", WeightGrams: 120, Confidence: 0.8},
			},
		},
	}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.ClarifyMeal(w, withClaims(clarifyRequest(meal.ID.String(), []string{"It's cabbage and fish, not just sauce"}), userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got database.FoodMeal
	json.NewDecoder(w.Body).Decode(&got) //nolint:errcheck
	if got.Status != database.MealStatusPendingReview {
		t.Errorf("expected pending_review, got %s", got.Status)
	}
	if len(got.Items) != 2 {
		t.Fatalf("expected two persisted items, one per clarified item, got %+v", got.Items)
	}
	names := map[string]bool{}
	for _, it := range got.Items {
		names[it.Name] = true
	}
	if !names["stewed cabbage"] || !names["baked white fish"] {
		t.Errorf("expected both clarified items to be persisted by name, got %+v", got.Items)
	}

	var count int64
	if err := st.DB().Model(&database.FoodItem{}).Where("meal_id = ?", meal.ID).Count(&count).Error; err != nil {
		t.Fatalf("count items: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 FoodItem rows persisted, got %d", count)
	}
}

// Regression (design.md Risks / tasks.md 4.4, 6.5): the clarification-round
// reconstruction in food_clarify.go previously rebuilt vision.Item from the
// stored FoodItem without carrying Brand forward, silently downgrading a
// clarified item back to brand-less USDA-only matching even though
// recognition had actually seen a brand. This asserts both the direct fix
// (the replayed prior item still carries Brand) and its actual consequence
// (resolution after the clarify round still routes through Open Food Facts,
// not just USDA, using the brand carried through).
func TestClarifyMeal_PriorItemBrandIsReplayedAndStillRoutesToOFF(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	log, err := json.Marshal([]database.ClarifyEntry{
		{Round: 1, Question: "Which yogurt is this?", Answer: ""},
	})
	if err != nil {
		t.Fatalf("marshal clarify log: %v", err)
	}
	meal := database.FoodMeal{
		UserID: userID, Status: database.MealStatusPendingClarification, LoggedAt: time.Now(),
		ClarifyRound: 0, ClarifyLog: string(log),
	}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}
	item := database.FoodItem{
		UserID: userID, MealID: meal.ID, Name: "yogurt", Brand: "Olma",
		WeightGrams: 150, MacroSource: database.MacroSourceNone,
	}
	item.ID = uuid.New()
	item.FamilyID = familyID
	if err := st.DB().Create(&item).Error; err != nil {
		t.Fatalf("create item: %v", err)
	}

	offIdx := buildOFFIndex(t, offFood("8594001222227", "yogurt", "Olma", 65))
	fake := &vision.Fake{
		// The model preserves the brand it was told about in priorItems —
		// this is what processRecognition/resolveItems actually resolves
		// against next.
		ClarifyResult: &vision.RecognizeResult{
			Items: []vision.Item{{Name: "yogurt", Brand: "Olma", WeightGrams: 150, Confidence: 0.9}},
		},
		SelectResult: &vision.SelectResult{
			Selections: []vision.Selection{{ItemIndex: 0, CandidateIndex: 0}},
		},
	}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second).WithOFF(offIdx)

	w := httptest.NewRecorder()
	h.ClarifyMeal(w, withClaims(clarifyRequest(meal.ID.String(), []string{"The Olma one"}), userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if len(fake.ClarifyCalls) != 1 {
		t.Fatalf("expected exactly one Clarify call, got %d", len(fake.ClarifyCalls))
	}
	if got := fake.ClarifyCalls[0].PriorItems; len(got) != 1 || got[0].Brand != "Olma" {
		t.Fatalf("expected the replayed prior item to carry Brand \"Olma\", got %+v", got)
	}

	var gotMeal database.FoodMeal
	json.NewDecoder(w.Body).Decode(&gotMeal) //nolint:errcheck
	if gotMeal.Status != database.MealStatusPendingReview {
		t.Fatalf("expected pending_review, got %s", gotMeal.Status)
	}
	if len(gotMeal.Items) != 1 || gotMeal.Items[0].OffCode == nil || *gotMeal.Items[0].OffCode != "8594001222227" {
		t.Fatalf("expected re-resolution to bind via Open Food Facts using the carried-through brand, got %+v", gotMeal.Items)
	}
}

func TestClarifyMeal_PersistsAnswerInClarifyLog(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createPendingClarificationMeal(t, st, userID, familyID)

	fake := &vision.Fake{ClarifyResult: &vision.RecognizeResult{Items: []vision.Item{{Name: "Sauce", WeightGrams: 30}}}}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.ClarifyMeal(w, withClaims(clarifyRequest(meal.ID.String(), []string{"Tomato-based"}), userID))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var persisted database.FoodMeal
	if err := st.DB().First(&persisted, "id = ?", meal.ID).Error; err != nil {
		t.Fatalf("reload meal: %v", err)
	}
	var entries []database.ClarifyEntry
	if err := json.Unmarshal([]byte(persisted.ClarifyLog), &entries); err != nil {
		t.Fatalf("unmarshal clarify log: %v", err)
	}
	if len(entries) != 1 || entries[0].Answer != "Tomato-based" {
		t.Errorf("expected the answer to be persisted in clarify_log, got %+v", entries)
	}
}

func TestClarifyMeal_MultipleRoundsUpToCapThenPendingReview(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createPendingClarificationMeal(t, st, userID, familyID)

	// Round 1 and 2 still ask another question; round 3 (the cap) resolves.
	askAgain := &vision.RecognizeResult{
		Items:                  []vision.Item{{Name: "Sauce", WeightGrams: 30}},
		ClarificationQuestions: []string{"Still unclear — any other hints?"},
	}
	resolved := &vision.RecognizeResult{Items: []vision.Item{{Name: "Sauce", WeightGrams: 30}}}
	fake := &vision.Fake{ClarifyResults: []*vision.RecognizeResult{askAgain, askAgain, resolved}}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	// Round 1.
	w1 := httptest.NewRecorder()
	h.ClarifyMeal(w1, withClaims(clarifyRequest(meal.ID.String(), []string{"a1"}), userID))
	if w1.Code != http.StatusOK {
		t.Fatalf("round 1: expected 200, got %d: %s", w1.Code, w1.Body.String())
	}
	var afterRound1 database.FoodMeal
	json.NewDecoder(w1.Body).Decode(&afterRound1) //nolint:errcheck
	if afterRound1.Status != database.MealStatusPendingClarification {
		t.Fatalf("round 1: expected still pending_clarification, got %s", afterRound1.Status)
	}

	// Round 2.
	w2 := httptest.NewRecorder()
	h.ClarifyMeal(w2, withClaims(clarifyRequest(meal.ID.String(), []string{"a2"}), userID))
	if w2.Code != http.StatusOK {
		t.Fatalf("round 2: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
	var afterRound2 database.FoodMeal
	json.NewDecoder(w2.Body).Decode(&afterRound2) //nolint:errcheck
	if afterRound2.Status != database.MealStatusPendingClarification {
		t.Fatalf("round 2: expected still pending_clarification, got %s", afterRound2.Status)
	}

	// Round 3 (== MaxClarifyRounds): must move to pending_review regardless
	// of the fact the fake keeps asking, since the cap is reached.
	w3 := httptest.NewRecorder()
	h.ClarifyMeal(w3, withClaims(clarifyRequest(meal.ID.String(), []string{"a3"}), userID))
	if w3.Code != http.StatusOK {
		t.Fatalf("round 3: expected 200, got %d: %s", w3.Code, w3.Body.String())
	}
	var afterRound3 database.FoodMeal
	json.NewDecoder(w3.Body).Decode(&afterRound3) //nolint:errcheck
	if afterRound3.Status != database.MealStatusPendingReview {
		t.Errorf("round 3: expected pending_review at the round cap, got %s", afterRound3.Status)
	}
	if afterRound3.ClarifyRound != database.MaxClarifyRounds {
		t.Errorf("expected clarify_round %d, got %d", database.MaxClarifyRounds, afterRound3.ClarifyRound)
	}

	if len(fake.ClarifyCalls) != 3 {
		t.Fatalf("expected 3 Clarify calls, got %d", len(fake.ClarifyCalls))
	}
	// Round 3's history must replay all three answers.
	if len(fake.ClarifyCalls[2].History) != 3 {
		t.Errorf("expected round 3 history to carry all 3 answers, got %+v", fake.ClarifyCalls[2].History)
	}
}

func TestClarifyMeal_AnswerCountMismatchReturns400(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createPendingClarificationMeal(t, st, userID, familyID)

	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(&vision.Fake{}, 10<<20, time.Second)
	w := httptest.NewRecorder()
	h.ClarifyMeal(w, withClaims(clarifyRequest(meal.ID.String(), []string{"a1", "a2"}), userID))

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestClarifyMeal_NotPendingClarificationReturns409(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := database.FoodMeal{UserID: userID, Status: database.MealStatusPendingReview, LoggedAt: time.Now()}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}

	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(&vision.Fake{}, 10<<20, time.Second)
	w := httptest.NewRecorder()
	h.ClarifyMeal(w, withClaims(clarifyRequest(meal.ID.String(), []string{"a1"}), userID))

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestClarifyMeal_CrossUserReturns404(t *testing.T) {
	st := newFoodTestStorage(t)
	_, familyID := seedFoodUser(t, st)
	otherUserID := uuid.New()
	meal := createPendingClarificationMeal(t, st, otherUserID, familyID)

	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(&vision.Fake{}, 10<<20, time.Second)
	w := httptest.NewRecorder()
	h.ClarifyMeal(w, withClaims(clarifyRequest(meal.ID.String(), []string{"a1"}), uuid.New()))

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestClarifyMeal_VisionErrorMarksFailed(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createPendingClarificationMeal(t, st, userID, familyID)

	fake := &vision.Fake{ClarifyErr: errClarifyBoom}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)
	w := httptest.NewRecorder()
	h.ClarifyMeal(w, withClaims(clarifyRequest(meal.ID.String(), []string{"a1"}), userID))

	var got database.FoodMeal
	json.NewDecoder(w.Body).Decode(&got) //nolint:errcheck
	if got.Status != database.MealStatusFailed {
		t.Errorf("expected failed, got %s", got.Status)
	}
}

// slowClarifyClient blocks for delay before returning, widening the race
// window between "load meal, see pending_clarification" and "claim the
// round" enough for two real goroutines to land inside it.
type slowClarifyClient struct{ delay time.Duration }

func (c slowClarifyClient) Recognize(context.Context, []byte, string, string, string) (*vision.RecognizeResult, error) {
	return &vision.RecognizeResult{}, nil
}
func (c slowClarifyClient) EstimateWeights(context.Context, []byte, string, []vision.WeightEstimateInput) (*vision.WeightEstimateResult, error) {
	return &vision.WeightEstimateResult{}, nil
}
func (c slowClarifyClient) Clarify(_ context.Context, _ string, _ []vision.Item, _ []vision.ClarifyTurn, _ string) (*vision.RecognizeResult, error) {
	time.Sleep(c.delay)
	return &vision.RecognizeResult{Items: []vision.Item{{Name: "Sauce", WeightGrams: 30}}}, nil
}
func (c slowClarifyClient) Select(context.Context, []vision.ItemCandidates) (*vision.SelectResult, error) {
	return &vision.SelectResult{}, nil
}
func (c slowClarifyClient) Translate(context.Context, string) (string, error) {
	return "", nil
}
func (c slowClarifyClient) Describe(context.Context, string, string) (*vision.RecognizeResult, error) {
	return &vision.RecognizeResult{}, nil
}

// Two genuinely concurrent submissions of the same clarify round (a
// double-click, or two tabs) must not both succeed: only one may claim the
// round and bill a real vision call, the other must be rejected outright
// rather than racing an unguarded write.
func TestClarifyMeal_ConcurrentDoubleSubmitOnlyOneWins(t *testing.T) {
	st := newFoodTestStorage(t)
	// :memory: SQLite gives each pooled connection its own separate
	// database unless using shared-cache mode; database.Open never limits
	// pool size. Pin to one connection so this test exercises the app's
	// actual concurrency guard instead of a spurious "which connection sees
	// which database" artifact — production uses a file-based DB, which is
	// genuinely shared across connections and doesn't have this problem.
	sqlDB, err := st.DB().DB()
	if err != nil {
		t.Fatalf("underlying sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	userID, familyID := seedFoodUser(t, st)
	meal := createPendingClarificationMeal(t, st, userID, familyID)

	client := slowClarifyClient{delay: 50 * time.Millisecond}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(client, 10<<20, time.Second)

	var wg sync.WaitGroup
	codes := make([]int, 2)
	for i := range 2 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			r := clarifyRequest(meal.ID.String(), []string{"answer"})
			w := httptest.NewRecorder()
			h.ClarifyMeal(w, withClaims(r, userID))
			codes[idx] = w.Code
		}(i)
	}
	wg.Wait()

	successes := 0
	for _, c := range codes {
		if c == http.StatusOK {
			successes++
		}
	}
	if successes != 1 {
		t.Errorf("expected exactly 1 of 2 concurrent submissions to succeed, got codes %v", codes)
	}
}

// seedClarifyMealWithCanonicalName creates a pending_clarification meal holding
// one item that has both a Display Name and a Canonical Name, as a non-English
// recognition round would have persisted it, and sets the user's stored
// display_language.
func seedClarifyMealWithCanonicalName(
	t *testing.T, st database.Storage, userID, familyID uuid.UUID, displayName, canonicalName, language string,
) database.FoodMeal {
	t.Helper()
	putH := server.PutUserSettingsHandler(st)
	w := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"display_language":"` + language + `"}`)
	putH.ServeHTTP(w, withClaims(httptest.NewRequest(http.MethodPut, "/api/users/me/settings", body), userID))
	if w.Code != http.StatusOK {
		t.Fatalf("PUT settings: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	log, err := json.Marshal([]database.ClarifyEntry{
		{Round: 1, Question: "Со сметаной или без?", Answer: ""},
	})
	if err != nil {
		t.Fatalf("marshal clarify log: %v", err)
	}
	meal := database.FoodMeal{
		UserID: userID, Status: database.MealStatusPendingClarification, LoggedAt: time.Now(),
		ClarifyRound: 0, ClarifyLog: string(log),
	}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}
	item := database.FoodItem{
		UserID: userID, MealID: meal.ID, Name: displayName, CanonicalName: canonicalName,
		WeightGrams: 200, MacroSource: database.MacroSourceNone,
	}
	item.ID = uuid.New()
	item.FamilyID = familyID
	if err := st.DB().Create(&item).Error; err != nil {
		t.Fatalf("create item: %v", err)
	}
	return meal
}

// Regression for a code-review finding: canonical_name is required by the
// clarify response schema, so the model must emit the key — but not a non-empty
// value, and an empty one used to be persisted as-is. Since canonical_name is
// deliberately not editable via PATCH, that permanently stripped the item's
// English identity: a Russian user's meal would show an Expert Mode gloss for
// every item except the one that went through clarification, with no way to
// restore it. The prior round's Canonical Name is carried forward instead,
// under the same identity guard the estimate carry-forward uses.
func TestClarifyMeal_EmptyCanonicalNameKeepsThePriorRounds(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := seedClarifyMealWithCanonicalName(t, st, userID, familyID, "вареники", "dumplings", "ru")

	fake := &vision.Fake{
		// Same item, narrowed name, and no canonical_name of its own.
		ClarifyResult: &vision.RecognizeResult{
			Items: []vision.Item{{Name: "вареники с картошкой", WeightGrams: 200, Confidence: 0.8}},
		},
	}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.ClarifyMeal(w, withClaims(clarifyRequest(meal.ID.String(), []string{"Без сметаны"}), userID))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var got database.FoodMeal
	json.NewDecoder(w.Body).Decode(&got) //nolint:errcheck
	if len(got.Items) != 1 {
		t.Fatalf("expected one item, got %+v", got.Items)
	}
	if got.Items[0].CanonicalName != "dumplings" {
		t.Errorf("CanonicalName = %q, want the prior round's %q", got.Items[0].CanonicalName, "dumplings")
	}
	if got.Items[0].Name != "вареники с картошкой" {
		t.Errorf("Name = %q, want the clarified display name", got.Items[0].Name)
	}
}

// The carry-forward above must not fire for an English Display Language, where
// a Canonical Name stays empty rather than duplicating the Display Name (see
// food-photo-recognition "Recognition in English Display Language does not
// duplicate the name"). The only way to reach that combination — a stored
// Canonical Name and an English Display Language — is switching language
// between rounds, which is what this seeds.
func TestClarifyMeal_EnglishDoesNotCarryForwardCanonicalName(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := seedClarifyMealWithCanonicalName(t, st, userID, familyID, "dumplings", "dumplings", "en")

	fake := &vision.Fake{
		ClarifyResult: &vision.RecognizeResult{
			Items: []vision.Item{{Name: "dumplings", WeightGrams: 200, Confidence: 0.8}},
		},
	}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.ClarifyMeal(w, withClaims(clarifyRequest(meal.ID.String(), []string{"No sour cream"}), userID))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var got database.FoodMeal
	json.NewDecoder(w.Body).Decode(&got) //nolint:errcheck
	if len(got.Items) != 1 {
		t.Fatalf("expected one item, got %+v", got.Items)
	}
	if got.Items[0].CanonicalName != "" {
		t.Errorf("CanonicalName = %q, want empty for an English Display Language", got.Items[0].CanonicalName)
	}
}
