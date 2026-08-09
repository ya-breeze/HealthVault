package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"gorm.io/gorm"

	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/server"
	photostorage "github.com/ya-breeze/healthvault/pkg/storage"
	"github.com/ya-breeze/healthvault/pkg/vision"
)

func reanalyzeHTTPRequest(mealID, hint string) *http.Request {
	b, _ := json.Marshal(map[string]any{"hint": hint})
	r := httptest.NewRequest(http.MethodPost, "/api/food/meals/"+mealID+"/reanalyze", bytes.NewBuffer(b))
	return mux.SetURLVars(r, map[string]string{"id": mealID})
}

// createReanalyzeMeal creates a meal with a stored photo, one existing item,
// and the given status/clarify state — everything Reanalyze needs to be
// eligible, and enough pre-existing content to assert it's left untouched on
// a failed attempt.
func createReanalyzeMeal(
	t *testing.T, st database.Storage, dir string, userID, familyID uuid.UUID,
	status string, clarifyRound int, clarifyLog string,
) database.FoodMeal {
	t.Helper()
	photos := photostorage.New(dir)
	mealID := uuid.New()
	relPath, err := photos.Save(bytes.NewReader(fakeJPEGBytes), 1<<20, userID, photostorage.OwnerMeal, mealID)
	if err != nil {
		t.Fatalf("save photo: %v", err)
	}
	meal := database.FoodMeal{
		UserID: userID, Status: status, LoggedAt: time.Now(), PhotoPath: relPath,
		ClarifyRound: clarifyRound, ClarifyLog: clarifyLog,
	}
	meal.ID = mealID
	meal.FamilyID = familyID
	if status == database.MealStatusConfirmed {
		meal.Calories = 400
	}
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}
	item := database.FoodItem{
		UserID: userID, MealID: meal.ID, Name: "Original item", WeightGrams: 100,
		MacroSource: database.MacroSourceManual, Calories: 400,
	}
	item.ID = uuid.New()
	item.FamilyID = familyID
	if err := st.DB().Create(&item).Error; err != nil {
		t.Fatalf("create item: %v", err)
	}
	meal.Items = []database.FoodItem{item}
	return meal
}

func TestReanalyze_PendingReviewSucceedsAndReplacesItems(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	dir := t.TempDir()
	meal := createReanalyzeMeal(t, st, dir, userID, familyID, database.MealStatusPendingReview, 0, "")

	fake := &vision.Fake{RecognizeResult: &vision.RecognizeResult{
		Items: []vision.Item{{Name: "Rice", WeightGrams: 150}},
	}}
	h := server.NewFoodHandlers(st, nil, dir).WithVision(fake, 10<<20, time.Minute)

	w := httptest.NewRecorder()
	h.Reanalyze(w, withClaims(reanalyzeHTTPRequest(meal.ID.String(), "this is rice, not the original guess"), userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got database.FoodMeal
	json.NewDecoder(w.Body).Decode(&got) //nolint:errcheck
	if got.Status != database.MealStatusPendingReview {
		t.Errorf("expected pending_review, got %s", got.Status)
	}
	if len(got.Items) != 1 || got.Items[0].Name != "Rice" {
		t.Errorf("expected items replaced with the new recognition result, got %+v", got.Items)
	}
	if len(fake.RecognizeCalls) != 1 || fake.RecognizeCalls[0].Hint != "this is rice, not the original guess" {
		t.Errorf("expected the hint passed through to Recognize, got %+v", fake.RecognizeCalls)
	}
}

func TestReanalyze_ConfirmedMealRevertsToReviewAndZeroesAggregate(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	dir := t.TempDir()
	meal := createReanalyzeMeal(t, st, dir, userID, familyID, database.MealStatusConfirmed, 0, "")

	fake := &vision.Fake{RecognizeResult: &vision.RecognizeResult{
		Items: []vision.Item{{Name: "Chicken", WeightGrams: 120}},
	}}
	h := server.NewFoodHandlers(st, nil, dir).WithVision(fake, 10<<20, time.Minute)

	w := httptest.NewRecorder()
	h.Reanalyze(w, withClaims(reanalyzeHTTPRequest(meal.ID.String(), "this is chicken, not the original guess"), userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got database.FoodMeal
	json.NewDecoder(w.Body).Decode(&got) //nolint:errcheck
	if got.Status == database.MealStatusConfirmed {
		t.Error("expected the meal to no longer be confirmed after a successful reanalysis")
	}
	if got.Calories != 0 {
		t.Errorf("expected the stored aggregate to be zeroed, got %v", got.Calories)
	}
	if len(got.Items) != 1 || got.Items[0].Name != "Chicken" {
		t.Errorf("expected items replaced, got %+v", got.Items)
	}
}

func TestReanalyze_ClarifyRoundResetOnReanalysis(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	dir := t.TempDir()
	meal := createReanalyzeMeal(t, st, dir, userID, familyID, database.MealStatusConfirmed, 3, `[{"round":1,"question":"spicy?","answer":"yes"}]`)

	fake := &vision.Fake{RecognizeResult: &vision.RecognizeResult{
		Items: []vision.Item{{Name: "Soup", WeightGrams: 200}},
	}}
	h := server.NewFoodHandlers(st, nil, dir).WithVision(fake, 10<<20, time.Minute)

	w := httptest.NewRecorder()
	h.Reanalyze(w, withClaims(reanalyzeHTTPRequest(meal.ID.String(), "it's soup"), userID))

	var got database.FoodMeal
	json.NewDecoder(w.Body).Decode(&got) //nolint:errcheck
	if got.ClarifyRound != 0 || got.ClarifyLog != "" {
		t.Errorf("expected clarify state reset by reanalysis, got round=%d log=%q", got.ClarifyRound, got.ClarifyLog)
	}
}

func TestReanalyze_BlankHintReturns400(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	dir := t.TempDir()
	meal := createReanalyzeMeal(t, st, dir, userID, familyID, database.MealStatusPendingReview, 0, "")
	fake := &vision.Fake{}
	h := server.NewFoodHandlers(st, nil, dir).WithVision(fake, 10<<20, time.Minute)

	for _, hint := range []string{"", "   "} {
		w := httptest.NewRecorder()
		h.Reanalyze(w, withClaims(reanalyzeHTTPRequest(meal.ID.String(), hint), userID))
		if w.Code != http.StatusBadRequest {
			t.Errorf("hint=%q: expected 400, got %d: %s", hint, w.Code, w.Body.String())
		}
	}
	if len(fake.RecognizeCalls) != 0 {
		t.Errorf("expected no vision call for a blank hint, got %d", len(fake.RecognizeCalls))
	}
}

func TestReanalyze_OversizedHintReturns400(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	dir := t.TempDir()
	meal := createReanalyzeMeal(t, st, dir, userID, familyID, database.MealStatusPendingReview, 0, "")
	fake := &vision.Fake{}
	h := server.NewFoodHandlers(st, nil, dir).WithVision(fake, 10<<20, time.Minute)

	w := httptest.NewRecorder()
	h.Reanalyze(w, withClaims(reanalyzeHTTPRequest(meal.ID.String(), strings.Repeat("x", 501)), userID))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for an over-length hint, got %d: %s", w.Code, w.Body.String())
	}
	if len(fake.RecognizeCalls) != 0 {
		t.Errorf("expected no vision call for an oversized hint, got %d", len(fake.RecognizeCalls))
	}
}

// Regression: the hint limit is documented as 500 characters, but len() on a
// Go string counts UTF-8 bytes. A non-ASCII hint (each Cyrillic character is
// ~2 bytes) can be well within 500 characters while exceeding 500 bytes —
// counting bytes would reject it even though it satisfies the documented
// limit.
func TestReanalyze_HintLengthCountsCharactersNotBytes(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	dir := t.TempDir()
	meal := createReanalyzeMeal(t, st, dir, userID, familyID, database.MealStatusPendingReview, 0, "")
	fake := &vision.Fake{RecognizeResult: &vision.RecognizeResult{Items: []vision.Item{{Name: "Rice", WeightGrams: 100}}}}
	h := server.NewFoodHandlers(st, nil, dir).WithVision(fake, 10<<20, time.Minute)

	// 500 Cyrillic characters, ~1000 UTF-8 bytes — must be accepted.
	hint := strings.Repeat("щ", 500)
	w := httptest.NewRecorder()
	h.Reanalyze(w, withClaims(reanalyzeHTTPRequest(meal.ID.String(), hint), userID))
	if w.Code != http.StatusOK {
		t.Errorf("expected a 500-character non-ASCII hint to be accepted, got %d: %s", w.Code, w.Body.String())
	}
}

func TestReanalyze_OversizedBodyReturns413(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	dir := t.TempDir()
	meal := createReanalyzeMeal(t, st, dir, userID, familyID, database.MealStatusPendingReview, 0, "")
	h := server.NewFoodHandlers(st, nil, dir).WithVision(&vision.Fake{}, 10<<20, time.Minute)

	huge := map[string]any{"hint": strings.Repeat("x", 10*1024)}
	b, _ := json.Marshal(huge)
	r := httptest.NewRequest(http.MethodPost, "/api/food/meals/"+meal.ID.String()+"/reanalyze", bytes.NewBuffer(b))
	r = mux.SetURLVars(r, map[string]string{"id": meal.ID.String()})

	w := httptest.NewRecorder()
	h.Reanalyze(w, withClaims(r, userID))
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413, got %d: %s", w.Code, w.Body.String())
	}
}

func TestReanalyze_NoPhotoReturns409(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := database.FoodMeal{UserID: userID, Status: database.MealStatusPendingReview, LoggedAt: time.Now()}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}

	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(&vision.Fake{}, 10<<20, time.Minute)
	w := httptest.NewRecorder()
	h.Reanalyze(w, withClaims(reanalyzeHTTPRequest(meal.ID.String(), "a hint"), userID))
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestReanalyze_IneligibleStatusesReturn409(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	dir := t.TempDir()

	for _, status := range []string{database.MealStatusProcessing, database.MealStatusPendingClarification} {
		meal := createReanalyzeMeal(t, st, dir, userID, familyID, status, 0, "")
		h := server.NewFoodHandlers(st, nil, dir).WithVision(&vision.Fake{}, 10<<20, time.Minute)
		w := httptest.NewRecorder()
		h.Reanalyze(w, withClaims(reanalyzeHTTPRequest(meal.ID.String(), "a hint"), userID))
		if w.Code != http.StatusConflict {
			t.Errorf("status=%s: expected 409, got %d: %s", status, w.Code, w.Body.String())
		}
	}
}

func TestReanalyze_FailedVisionCallLeavesMealUnchanged(t *testing.T) {
	for _, status := range []string{database.MealStatusPendingReview, database.MealStatusConfirmed} {
		st := newFoodTestStorage(t)
		userID, familyID := seedFoodUser(t, st)
		dir := t.TempDir()
		meal := createReanalyzeMeal(t, st, dir, userID, familyID, status, 0, "")

		fake := &vision.Fake{RecognizeErr: context.DeadlineExceeded}
		h := server.NewFoodHandlers(st, nil, dir).WithVision(fake, 10<<20, time.Minute)

		w := httptest.NewRecorder()
		h.Reanalyze(w, withClaims(reanalyzeHTTPRequest(meal.ID.String(), "a hint"), userID))
		if w.Code != http.StatusBadGateway {
			t.Errorf("status=%s: expected 502, got %d: %s", status, w.Code, w.Body.String())
		}

		var reloaded database.FoodMeal
		if err := st.DB().Preload("Items").Where("id = ?", meal.ID).First(&reloaded).Error; err != nil {
			t.Fatalf("reload meal: %v", err)
		}
		if reloaded.Status != status {
			t.Errorf("status=%s: expected meal status unchanged, got %s", status, reloaded.Status)
		}
		if len(reloaded.Items) != 1 || reloaded.Items[0].Name != "Original item" {
			t.Errorf("status=%s: expected original item untouched, got %+v", status, reloaded.Items)
		}
		if status == database.MealStatusConfirmed && reloaded.Calories != 400 {
			t.Errorf("expected confirmed meal's aggregate untouched, got %v", reloaded.Calories)
		}
	}
}

// Regression: when the vision result includes clarification questions,
// processRecognition used to persist the item replacement/status/aggregate
// (persistAnalysis) and the clarify_log (appendPendingQuestions) as two
// separate statements. If only the second failed, the error still reached
// Reanalyze and it reverted — but revertReanalyze only restores
// status/clarify_round/clarify_log; the items, raw_response, and zeroed
// aggregate from the first (already-committed) statement stayed applied,
// contradicting the 502 response's "the meal is unchanged" claim. Both
// writes are now one transaction inside persistAnalysis, so a failure in
// either rolls back both. This forces the (now single) meal-level write to
// fail and checks the meal is left with its ORIGINAL items and status, not
// a mix of old status and new items.
func TestReanalyze_ClarificationPersistenceFailureLeavesMealFullyUnchanged(t *testing.T) {
	st := newFileFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	dir := t.TempDir()
	meal := createReanalyzeMeal(t, st, dir, userID, familyID, database.MealStatusPendingReview, 0, "")

	fake := &vision.Fake{RecognizeResult: &vision.RecognizeResult{
		Items:                  []vision.Item{{Name: "Mystery dish", WeightGrams: 100}},
		ClarificationQuestions: []string{"Is this spicy?"},
	}}
	h := server.NewFoodHandlers(st, nil, dir).WithVision(fake, 10<<20, time.Minute)

	// Update call #1 is Reanalyze's own atomic claim (outside
	// persistAnalysis's transaction); #2 is persistAnalysis's single
	// combined status/raw_response/aggregate/clarify_log write, reached via
	// the pending_clarification branch this Fake result triggers — that's
	// the one to fail.
	updateCalls := 0
	const hookName = "test:fail-clarify-persist"
	st.DB().Callback().Update().Before("gorm:update").Register(hookName, func(tx *gorm.DB) {
		updateCalls++
		if updateCalls == 2 {
			tx.Error = errors.New("simulated clarify persistence failure")
		}
	})
	t.Cleanup(func() { st.DB().Callback().Update().Remove(hookName) })

	w := httptest.NewRecorder()
	h.Reanalyze(w, withClaims(reanalyzeHTTPRequest(meal.ID.String(), "a hint"), userID))

	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", w.Code, w.Body.String())
	}

	var reloaded database.FoodMeal
	if err := st.DB().Preload("Items").Where("id = ?", meal.ID).First(&reloaded).Error; err != nil {
		t.Fatalf("reload meal: %v", err)
	}
	if reloaded.Status != database.MealStatusPendingReview {
		t.Errorf("expected status reverted to pending_review, got %s", reloaded.Status)
	}
	if len(reloaded.Items) != 1 || reloaded.Items[0].Name != "Original item" {
		t.Errorf("expected the original item untouched — not replaced by the failed persist — got %+v", reloaded.Items)
	}
	if reloaded.ClarifyLog != "" {
		t.Errorf("expected no clarify_log written on a failed persist, got %q", reloaded.ClarifyLog)
	}
}

// Regression: RetryMeal treats `processing` as retryable once updated_at is
// older than the same vision timeout this handler's own call runs against.
// If Reanalyze's call is slow enough to cross that threshold right as it
// fails, a concurrent RetryMeal can legitimately claim the meal (fresh
// status=processing, fresh updated_at — a "newer attempt") before this
// handler's revert runs. The old unconditional revert (`WHERE id = ?`)
// would stomp that claim back to the stale pre-reanalyze status, letting
// edits resume on a meal a retry may still be actively replacing the items
// of. The lease (this attempt's own updated_at, captured at its claim) makes
// the revert conditional: it must no-op once a newer claim has landed.
func TestReanalyze_FailedAttemptDoesNotStompNewerConcurrentClaim(t *testing.T) {
	st := newFileFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	dir := t.TempDir()
	meal := createReanalyzeMeal(t, st, dir, userID, familyID, database.MealStatusPendingReview, 0, "")

	fake := &vision.Fake{RecognizeErr: context.DeadlineExceeded}
	h := server.NewFoodHandlers(st, nil, dir).WithVision(fake, 10<<20, time.Minute)

	// Update call #1 is this handler's own claim; #2 would normally be its
	// revert (no persistAnalysis update happens on a failed vision call) —
	// inject the "concurrent newer claim" right before that revert fires.
	updateCalls := 0
	const hookName = "test:concurrent-reclaim"
	st.DB().Callback().Update().Before("gorm:update").Register(hookName, func(*gorm.DB) {
		updateCalls++
		if updateCalls == 2 {
			sqlDB, err := st.DB().DB()
			if err != nil {
				t.Fatalf("get sql.DB: %v", err)
			}
			if _, err := sqlDB.Exec(
				"UPDATE food_meals SET status = ?, updated_at = ? WHERE id = ?",
				database.MealStatusProcessing, time.Now().UTC().Add(time.Second), meal.ID,
			); err != nil {
				t.Fatalf("simulate concurrent reclaim: %v", err)
			}
		}
	})
	t.Cleanup(func() { st.DB().Callback().Update().Remove(hookName) })

	w := httptest.NewRecorder()
	h.Reanalyze(w, withClaims(reanalyzeHTTPRequest(meal.ID.String(), "a hint"), userID))

	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", w.Code, w.Body.String())
	}

	var reloaded database.FoodMeal
	if err := st.DB().Where("id = ?", meal.ID).First(&reloaded).Error; err != nil {
		t.Fatalf("reload meal: %v", err)
	}
	if reloaded.Status != database.MealStatusProcessing {
		t.Errorf("expected the newer concurrent claim's processing status to survive this attempt's revert, got %s", reloaded.Status)
	}
}

func TestReanalyze_CrossUserReturns404(t *testing.T) {
	st := newFoodTestStorage(t)
	_, familyID := seedFoodUser(t, st)
	otherUserID := uuid.New()
	dir := t.TempDir()
	meal := createReanalyzeMeal(t, st, dir, otherUserID, familyID, database.MealStatusPendingReview, 0, "")

	h := server.NewFoodHandlers(st, nil, dir).WithVision(&vision.Fake{}, 10<<20, time.Minute)
	w := httptest.NewRecorder()
	h.Reanalyze(w, withClaims(reanalyzeHTTPRequest(meal.ID.String(), "a hint"), uuid.New()))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// gatedRecognizeClient blocks inside Recognize until proceed is closed,
// signaling entered the moment it's called — after the atomic claim has
// already run — so a test can deterministically fire a second concurrent
// request while the first is still "in flight".
type gatedRecognizeClient struct {
	entered chan struct{}
	proceed chan struct{}
	result  *vision.RecognizeResult
}

func (c *gatedRecognizeClient) Recognize(context.Context, []byte, string, string) (*vision.RecognizeResult, error) {
	close(c.entered)
	<-c.proceed
	return c.result, nil
}

func (c *gatedRecognizeClient) Clarify(context.Context, []vision.Item, []vision.ClarifyTurn) (*vision.RecognizeResult, error) {
	return &vision.RecognizeResult{}, nil
}

func (c *gatedRecognizeClient) Select(context.Context, []vision.ItemCandidates) (*vision.SelectResult, error) {
	return &vision.SelectResult{}, nil
}

// Regression: the claim used to be `WHERE status IN (eligible...)`, which
// matches *any* eligible status, not specifically the one this request
// actually observed. If a concurrent ConfirmMeal committed pending_review ->
// confirmed in the gap between Reanalyze's initial read and its claim, the
// old claim would still match (confirmed is also eligible), succeed, and —
// on a later failure — revert to the *stale* captured pending_review,
// discarding the confirm that happened concurrently. Claiming the exact
// captured status closes this: the claim must fail here, and Reanalyze must
// not touch the meal at all (not even attempt the vision call).
func TestReanalyze_ClaimFailsIfStatusChangedConcurrently(t *testing.T) {
	st := newFileFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	dir := t.TempDir()
	meal := createReanalyzeMeal(t, st, dir, userID, familyID, database.MealStatusPendingReview, 0, "")

	registerRaceSimulation(t, st.DB(), func() {
		sqlDB, err := st.DB().DB()
		if err != nil {
			t.Fatalf("get sql.DB: %v", err)
		}
		if _, err := sqlDB.Exec(
			"UPDATE food_meals SET status = ? WHERE id = ?", database.MealStatusConfirmed, meal.ID,
		); err != nil {
			t.Fatalf("simulate concurrent confirm: %v", err)
		}
	})

	fake := &vision.Fake{RecognizeResult: &vision.RecognizeResult{Items: []vision.Item{{Name: "Rice", WeightGrams: 100}}}}
	h := server.NewFoodHandlers(st, nil, dir).WithVision(fake, 10<<20, time.Minute)

	w := httptest.NewRecorder()
	h.Reanalyze(w, withClaims(reanalyzeHTTPRequest(meal.ID.String(), "a hint"), userID))

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 when status changed concurrently, got %d: %s", w.Code, w.Body.String())
	}
	if len(fake.RecognizeCalls) != 0 {
		t.Errorf("expected no vision call once the claim failed, got %d", len(fake.RecognizeCalls))
	}

	var reloaded database.FoodMeal
	if err := st.DB().Where("id = ?", meal.ID).First(&reloaded).Error; err != nil {
		t.Fatalf("reload meal: %v", err)
	}
	if reloaded.Status != database.MealStatusConfirmed {
		t.Errorf("expected the concurrent confirm to stick (not be reverted to the stale pending_review), got status=%s", reloaded.Status)
	}
}

func TestReanalyze_ConcurrentCallsOnlyOneProceeds(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	dir := t.TempDir()
	meal := createReanalyzeMeal(t, st, dir, userID, familyID, database.MealStatusPendingReview, 0, "")

	client := &gatedRecognizeClient{
		entered: make(chan struct{}),
		proceed: make(chan struct{}),
		result:  &vision.RecognizeResult{Items: []vision.Item{{Name: "Rice", WeightGrams: 100}}},
	}
	h := server.NewFoodHandlers(st, nil, dir).WithVision(client, 10<<20, time.Minute)

	done1 := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		w := httptest.NewRecorder()
		h.Reanalyze(w, withClaims(reanalyzeHTTPRequest(meal.ID.String(), "hint one"), userID))
		done1 <- w
	}()

	<-client.entered // first request has already claimed the meal (status=processing) and reached vision

	w2 := httptest.NewRecorder()
	h.Reanalyze(w2, withClaims(reanalyzeHTTPRequest(meal.ID.String(), "hint two"), userID))
	if w2.Code != http.StatusConflict {
		t.Errorf("expected the second concurrent call to get 409, got %d: %s", w2.Code, w2.Body.String())
	}

	close(client.proceed)
	w1 := <-done1
	if w1.Code != http.StatusOK {
		t.Errorf("expected the first call to succeed, got %d: %s", w1.Code, w1.Body.String())
	}
}
