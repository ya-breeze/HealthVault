package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/server"
	"github.com/ya-breeze/healthvault/pkg/vision"
	"gorm.io/gorm"
)

// Exercise the actual stateless HTTP protocol, including its structured output.
func phoneCall(t *testing.T, h http.Handler, tool string, args map[string]any) map[string]any {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": tool, "arguments": args}})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/phone-mcp", bytes.NewReader(payload))
	r.Header.Set("Authorization", "Bearer test-phone-token")
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	r.Header.Set("MCP-Protocol-Version", "2025-11-25")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("protocol HTTP %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(w.Header().Get("Content-Type"), "text/event-stream") {
		for _, line := range strings.Split(body, "\n") {
			if strings.HasPrefix(line, "data: ") {
				body = strings.TrimPrefix(line, "data: ")
				break
			}
		}
	}
	var reply struct {
		Result map[string]any `json:"result"`
		Error  any            `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &reply); err != nil {
		t.Fatalf("decode %q: %v", body, err)
	}
	if reply.Error != nil {
		t.Fatalf("RPC error: %+v", reply.Error)
	}
	return reply.Result
}

func phoneSuccess(t *testing.T, result map[string]any) map[string]any {
	t.Helper()
	if result["isError"] == true {
		t.Fatalf("tool failed: %+v", result)
	}
	out, ok := result["structuredContent"].(map[string]any)
	if !ok {
		t.Fatalf("missing structured output: %+v", result)
	}
	return out
}

func TestPhoneMCPWorkflowReplayAndIsolation(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	fake := &vision.Fake{DescribeResult: &vision.RecognizeResult{Items: []vision.Item{{Name: "Soup", WeightGrams: 300, EstimatedProfile: &database.NutrientProfile{CaloriesPer100g: 40}}}}}
	food := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)
	h := food.PhoneMCPHandler("test-phone-token", userID.String(), nil)
	args := map[string]any{"request_id": "meal-1", "description": "300 g soup"}
	draft := phoneSuccess(t, phoneCall(t, h, "describe_food_meal", args))
	id := draft["meal_id"].(string)
	if draft["saved"] != false || draft["status"] != database.MealStatusPendingReview {
		t.Fatalf("incorrect draft: %+v", draft)
	}
	if draft["totals"].(map[string]any)["calories"] != float64(120) {
		t.Fatal("draft preview lost item nutrition")
	}
	// A reconstructed handler proves recovery uses persisted state rather than memory.
	h = food.PhoneMCPHandler("test-phone-token", userID.String(), nil)
	replay := phoneSuccess(t, phoneCall(t, h, "describe_food_meal", args))
	if replay["meal_id"] != id || len(fake.DescribeCalls) != 1 {
		t.Fatal("description replay created or analyzed another meal")
	}
	conflict := phoneCall(t, h, "describe_food_meal", map[string]any{"request_id": "meal-1", "description": "different soup"})
	if conflict["isError"] != true {
		t.Fatal("changed request accepted")
	}
	if phoneCall(t, h, "confirm_food_meal", map[string]any{"meal_id": id, "confirm": false})["isError"] != true {
		t.Fatal("confirmation without consent accepted")
	}
	confirmed := phoneSuccess(t, phoneCall(t, h, "confirm_food_meal", map[string]any{"meal_id": id, "confirm": true}))
	if confirmed["saved"] != true || confirmed["totals"].(map[string]any)["calories"] != float64(120) {
		t.Fatalf("incorrect confirmation: %+v", confirmed)
	}
	phoneSuccess(t, phoneCall(t, h, "confirm_food_meal", map[string]any{"meal_id": id, "confirm": true}))
	phoneSuccess(t, phoneCall(t, h, "get_food_meal", map[string]any{"meal_id": id}))
	// Even a meal in the same family belongs to a different user.
	other := database.FoodMeal{UserID: uuid.New(), Status: database.MealStatusPendingReview, LoggedAt: time.Now()}
	other.ID, other.FamilyID = uuid.New(), familyID
	if err := st.DB().Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"get_food_meal", "confirm_food_meal", "retry_food_meal", "clarify_food_meal"} {
		input := map[string]any{"meal_id": other.ID.String()}
		if tool == "confirm_food_meal" {
			input["confirm"] = true
		}
		if tool == "clarify_food_meal" {
			input["expected_round"], input["answers"] = 1, []string{"yes"}
			input["expected_version"] = other.UpdatedAt.UTC().Format(time.RFC3339Nano)
		}
		if phoneCall(t, h, tool, input)["isError"] != true {
			t.Fatalf("%s allowed another user's meal", tool)
		}
	}
	var meal database.FoodMeal
	if err := st.DB().First(&meal, "id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	if meal.UserID != userID || meal.Calories != 120 {
		t.Fatalf("wrong identity or duplicate aggregates: %+v", meal)
	}
}

func TestPhoneMCPClarificationRoundReplay(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createPendingClarificationMeal(t, st, userID, familyID)
	fake := &vision.Fake{ClarifyResult: &vision.RecognizeResult{Items: []vision.Item{{Name: "Sauce", WeightGrams: 30}}, ClarificationQuestions: []string{"How much?"}}}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second).PhoneMCPHandler("test-phone-token", userID.String(), nil)
	args := map[string]any{"meal_id": meal.ID.String(), "expected_round": 1, "expected_version": meal.UpdatedAt.UTC().Format(time.RFC3339Nano), "answers": []string{"tomato"}}
	first := phoneSuccess(t, phoneCall(t, h, "clarify_food_meal", args))
	if first["expected_round"] != float64(2) || len(first["questions"].([]any)) != 1 {
		t.Fatalf("wrong new round: %+v", first)
	}
	phoneSuccess(t, phoneCall(t, h, "clarify_food_meal", args))
	if len(fake.ClarifyCalls) != 1 {
		t.Fatal("replay answered next round")
	}
	args["answers"] = []string{"cream"}
	if phoneCall(t, h, "clarify_food_meal", args)["isError"] != true || len(fake.ClarifyCalls) != 1 {
		t.Fatal("changed answers accepted for old round")
	}
	phoneSuccess(t, phoneCall(t, h, "clarify_food_meal", map[string]any{"meal_id": meal.ID.String(), "expected_round": first["expected_round"], "expected_version": first["expected_version"], "answers": []string{"30 grams"}}))
	if len(fake.ClarifyCalls) != 2 {
		t.Fatal("returned version did not allow next round")
	}
}

func TestPhoneMCPDescribeReturnsUsableQuestionVersion(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	fake := &vision.Fake{DescribeResult: &vision.RecognizeResult{Items: []vision.Item{{Name: "Soup", WeightGrams: 300}}, ClarificationQuestions: []string{"What soup?"}}, ClarifyResult: &vision.RecognizeResult{Items: []vision.Item{{Name: "Borscht", WeightGrams: 300}}}}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second).PhoneMCPHandler("test-phone-token", userID.String(), nil)
	draft := phoneSuccess(t, phoneCall(t, h, "describe_food_meal", map[string]any{"request_id": "question-version", "description": "soup"}))
	result := phoneSuccess(t, phoneCall(t, h, "clarify_food_meal", map[string]any{"meal_id": draft["meal_id"], "expected_round": draft["expected_round"], "expected_version": draft["expected_version"], "answers": []string{"borscht"}}))
	if result["status"] != database.MealStatusPendingReview {
		t.Fatalf("returned version failed: %+v", result)
	}
}

func TestPhoneMCPFailedAnalysisRecovery(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	fake := &vision.Fake{DescribeErr: context.DeadlineExceeded}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second).PhoneMCPHandler("test-phone-token", userID.String(), nil)
	args := map[string]any{"request_id": "failed-1", "description": "soup"}
	failed := phoneSuccess(t, phoneCall(t, h, "describe_food_meal", args))
	if failed["status"] != database.MealStatusFailed || failed["saved"] != false {
		t.Fatalf("failure misreported: %+v", failed)
	}
	phoneSuccess(t, phoneCall(t, h, "describe_food_meal", args))
	if len(fake.DescribeCalls) != 1 {
		t.Fatal("replay re-ran failed analysis")
	}
	fake.DescribeErr = nil
	retried := phoneSuccess(t, phoneCall(t, h, "retry_food_meal", map[string]any{"meal_id": failed["meal_id"]}))
	if retried["meal_id"] != failed["meal_id"] || len(fake.DescribeCalls) != 2 {
		t.Fatal("retry did not recover original row")
	}
}

func TestPhoneMCPRejectsOldAnswersAfterRecoveryResetsRound(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createPendingClarificationMeal(t, st, userID, familyID)
	fake := &vision.Fake{ClarifyErr: context.DeadlineExceeded, DescribeResult: &vision.RecognizeResult{Items: []vision.Item{{Name: "Sauce", WeightGrams: 30}}, ClarificationQuestions: []string{"How much sauce?"}}}
	food := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)
	h := food.PhoneMCPHandler("test-phone-token", userID.String(), nil)
	old := map[string]any{"meal_id": meal.ID.String(), "expected_round": 1, "expected_version": meal.UpdatedAt.UTC().Format(time.RFC3339Nano), "answers": []string{"tomato"}}
	failed := phoneSuccess(t, phoneCall(t, h, "clarify_food_meal", old))
	if failed["status"] != database.MealStatusFailed {
		t.Fatalf("expected failed: %+v", failed)
	}
	// RetryMeal recognizes text-only rows by their persisted description.
	if err := st.DB().Model(&database.FoodMeal{}).Where("id = ?", meal.ID).Update("description", "sauce").Error; err != nil {
		t.Fatal(err)
	}
	retried := phoneSuccess(t, phoneCall(t, h, "retry_food_meal", map[string]any{"meal_id": meal.ID.String()}))
	if retried["expected_round"] != float64(1) || retried["expected_version"] == old["expected_version"] {
		t.Fatalf("expected new question generation: %+v", retried)
	}
	if phoneCall(t, h, "clarify_food_meal", old)["isError"] != true || len(fake.ClarifyCalls) != 1 {
		t.Fatal("old answer applied to recovered round")
	}
}

func TestPhoneMCPAuthAndDisabledConfiguration(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	food := server.NewFoodHandlers(st, nil, t.TempDir())
	for _, tc := range []struct {
		token, user, auth string
		status            int
	}{
		{"", userID.String(), "", 503}, {"test-phone-token", "bad-id", "", 503},
		{"test-phone-token", userID.String(), "", 401}, {"test-phone-token", userID.String(), "Bearer wrong", 401},
	} {
		r := httptest.NewRequest(http.MethodPost, "/phone-mcp", nil)
		r.Header.Set("Authorization", tc.auth)
		w := httptest.NewRecorder()
		food.PhoneMCPHandler(tc.token, tc.user, nil).ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("got %d want %d", w.Code, tc.status)
		}
	}
	if phoneCall(t, food.PhoneMCPHandler("test-phone-token", uuid.NewString(), nil), "get_food_meal", map[string]any{"meal_id": uuid.NewString()})["isError"] != true {
		t.Fatal("missing configured user accepted")
	}
}

func TestPhoneMCPBackupBarrierPerTool(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	barrier := &sync.RWMutex{}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).PhoneMCPHandler("test-phone-token", userID.String(), barrier)
	barrier.Lock()
	// A stateless GET is rejected immediately, rather than waiting on capture.
	r := httptest.NewRequest(http.MethodGet, "/phone-mcp", nil)
	r.Header.Set("Authorization", "Bearer test-phone-token")
	r.Header.Set("Accept", "text/event-stream")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		barrier.Unlock()
		t.Fatalf("stateless GET returned %d", w.Code)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		phoneCall(t, h, "get_food_meal", map[string]any{"meal_id": uuid.NewString()})
	}()
	select {
	case <-done:
		barrier.Unlock()
		t.Fatal("tool bypassed capture barrier")
	case <-time.After(30 * time.Millisecond):
	}
	barrier.Unlock()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("tool stayed blocked after capture")
	}
}

func TestDescribedMealSimultaneousFirstInsert(t *testing.T) {
	db, err := database.Open(slog.Default(), filepath.Join(t.TempDir(), "food.db"))
	if err != nil {
		t.Fatal(err)
	}
	st := database.NewStorage(db)
	userID, _ := seedFoodUser(t, st)
	ready := make(chan struct{})
	var arrivals atomic.Int32
	// Both requests have seen no existing key before either insertion begins.
	if err := db.Callback().Create().Before("gorm:begin_transaction").Register("test:first-insert-race", func(tx *gorm.DB) {
		if tx.Statement.Table == "food_meals" {
			if arrivals.Add(1) == 2 {
				close(ready)
			}
			<-ready
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Create().Remove("test:first-insert-race")
	fake := &vision.Fake{}
	food := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)
	responses := make(chan *httptest.ResponseRecorder, 2)
	for range 2 {
		go func() {
			w := httptest.NewRecorder()
			food.CreateDescribedMeal(w, withClaims(describeMealHTTPRequest(map[string]any{"request_id": "simultaneous", "description": "soup"}), userID))
			responses <- w
		}()
	}
	var ids []uuid.UUID
	for range 2 {
		select {
		case w := <-responses:
			if w.Code != 200 && w.Code != 201 {
				t.Fatalf("race HTTP %d: %s", w.Code, w.Body.String())
			}
			var meal database.FoodMeal
			if err := json.Unmarshal(w.Body.Bytes(), &meal); err != nil {
				t.Fatal(err)
			}
			ids = append(ids, meal.ID)
		case <-time.After(5 * time.Second):
			t.Fatal("first insert race hung")
		}
	}
	if ids[0] != ids[1] || len(fake.DescribeCalls) != 1 {
		t.Fatal("simultaneous requests duplicated a meal")
	}
}

type blockedPhoneVision struct {
	vision.Fake
	entered chan struct{}
	resume  chan struct{}
}

func (f *blockedPhoneVision) Describe(ctx context.Context, description, language string) (*vision.RecognizeResult, error) {
	close(f.entered)
	select {
	case <-f.resume:
		return f.Fake.Describe(ctx, description, language)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestDescribedMealConcurrentReplayAndDeletedKey(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	fake := &blockedPhoneVision{entered: make(chan struct{}), resume: make(chan struct{})}
	food := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, 5*time.Second)
	body := map[string]any{"request_id": "overlap", "description": "soup"}
	first := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		food.CreateDescribedMeal(first, withClaims(describeMealHTTPRequest(body), userID))
	}()
	select {
	case <-fake.entered:
	case <-time.After(time.Second):
		t.Fatal("analysis did not start")
	}
	replay := httptest.NewRecorder()
	food.CreateDescribedMeal(replay, withClaims(describeMealHTTPRequest(body), userID))
	close(fake.resume)
	<-done
	if first.Code != 201 || replay.Code != 200 {
		t.Fatalf("first=%d replay=%d: %s", first.Code, replay.Code, replay.Body.String())
	}
	var a, b database.FoodMeal
	if err := json.Unmarshal(first.Body.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(replay.Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	if a.ID != b.ID || b.Status != database.MealStatusProcessing || len(fake.DescribeCalls) != 1 {
		t.Fatal("in-flight replay duplicated analysis")
	}
	if err := st.DB().Delete(&a).Error; err != nil {
		t.Fatal(err)
	}
	deleted := httptest.NewRecorder()
	food.CreateDescribedMeal(deleted, withClaims(describeMealHTTPRequest(body), userID))
	if deleted.Code != 409 {
		t.Fatal("deleted request ID recreated a meal")
	}
	// Ordinary callers without request IDs retain their non-idempotent semantics.
	regular := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(&vision.Fake{}, 10<<20, time.Second)
	for range 2 {
		w := httptest.NewRecorder()
		regular.CreateDescribedMeal(w, withClaims(describeMealHTTPRequest(map[string]any{"description": "soup"}), userID))
		if w.Code != 201 {
			t.Fatalf("no-key meal rejected: %s", w.Body.String())
		}
	}
	var count int64
	if err := st.DB().Unscoped().Model(&database.FoodMeal{}).Count(&count).Error; err != nil || count != 3 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}
