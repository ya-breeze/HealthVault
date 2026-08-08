package server_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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
