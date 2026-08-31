package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/server"
	"github.com/ya-breeze/healthvault/pkg/vision"
)

func describeMealHTTPRequest(body map[string]any) *http.Request {
	b, _ := json.Marshal(body) //nolint:errcheck
	return httptest.NewRequest(http.MethodPost, "/api/food/meals/describe", bytes.NewBuffer(b))
}

func describeMealRawRequest(body []byte) *http.Request {
	return httptest.NewRequest(http.MethodPost, "/api/food/meals/describe", bytes.NewBuffer(body))
}

func TestCreateDescribedMeal_SuccessCreatesPendingReviewWithEstimatedItems(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	fake := &vision.Fake{
		DescribeResult: &vision.RecognizeResult{
			Items: []vision.Item{{
				Name: "Borscht", WeightGrams: 350, Confidence: 0.8,
				EstimatedProfile: &database.NutrientProfile{CaloriesPer100g: 49, ProteinPer100g: 1.5},
			}},
		},
	}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.CreateDescribedMeal(w, withClaims(describeMealHTTPRequest(map[string]any{
		"description": "  a bowl of borscht with sour cream  ",
	}), userID))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var meal database.FoodMeal
	if err := json.NewDecoder(w.Body).Decode(&meal); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if meal.Status != database.MealStatusPendingReview {
		t.Fatalf("expected pending_review, got %s", meal.Status)
	}
	if meal.Description != "a bowl of borscht with sour cream" {
		t.Errorf("expected trimmed description persisted, got %q", meal.Description)
	}
	if meal.PhotoPath != "" {
		t.Errorf("expected no photo path, got %q", meal.PhotoPath)
	}
	if meal.Calories != 0 {
		t.Errorf("expected zero aggregate before confirm, got %v", meal.Calories)
	}
	if len(meal.Items) != 1 || meal.Items[0].Name != "Borscht" || meal.Items[0].WeightGrams != 350 {
		t.Fatalf("unexpected items: %+v", meal.Items)
	}
	if meal.Items[0].MacroSource != database.MacroSourceEstimated {
		t.Errorf("expected estimated macro source, got %s", meal.Items[0].MacroSource)
	}
	if len(fake.DescribeCalls) != 1 || fake.DescribeCalls[0].Description != "a bowl of borscht with sour cream" {
		t.Fatalf("expected Describe called with the trimmed description, got %+v", fake.DescribeCalls)
	}
}

// When name is omitted, the meal Name is derived from the description,
// truncated to 60 runes with a trailing ellipsis.
func TestCreateDescribedMeal_DerivesNameFromDescriptionWhenOmitted(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(&vision.Fake{}, 10<<20, time.Second)

	long := strings.Repeat("a", 80)
	w := httptest.NewRecorder()
	h.CreateDescribedMeal(w, withClaims(describeMealHTTPRequest(map[string]any{"description": long}), userID))
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var meal database.FoodMeal
	json.NewDecoder(w.Body).Decode(&meal) //nolint:errcheck
	want := strings.Repeat("a", 60) + "…"
	if meal.Name != want {
		t.Errorf("expected derived name %q, got %q", want, meal.Name)
	}
}

func TestCreateDescribedMeal_ValidationRejectsBeforeAnyDescribeCall(t *testing.T) {
	for _, tc := range []struct {
		name string
		req  *http.Request
		want int
	}{
		{
			name: "missing description",
			req:  describeMealHTTPRequest(map[string]any{}),
			want: http.StatusBadRequest,
		},
		{
			name: "empty description",
			req:  describeMealHTTPRequest(map[string]any{"description": "   "}),
			want: http.StatusBadRequest,
		},
		{
			name: "over-length description",
			req:  describeMealHTTPRequest(map[string]any{"description": strings.Repeat("a", 1001)}),
			want: http.StatusBadRequest,
		},
		{
			name: "oversized request body",
			req:  describeMealRawRequest(append([]byte(`{"description":"x","padding":"`), append(bytes.Repeat([]byte("x"), 9*1024), []byte(`"}`)...)...)),
			want: http.StatusRequestEntityTooLarge,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := newFoodTestStorage(t)
			userID, _ := seedFoodUser(t, st)
			fake := &vision.Fake{}
			h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)

			w := httptest.NewRecorder()
			h.CreateDescribedMeal(w, withClaims(tc.req, userID))
			if w.Code != tc.want {
				t.Fatalf("expected %d, got %d: %s", tc.want, w.Code, w.Body.String())
			}
			if len(fake.DescribeCalls) != 0 {
				t.Errorf("expected no Describe call, got %d", len(fake.DescribeCalls))
			}
			var count int64
			if err := st.DB().Model(&database.FoodMeal{}).Count(&count).Error; err != nil || count != 0 {
				t.Errorf("expected no meal row, count=%d err=%v", count, err)
			}
		})
	}
}

func TestCreateDescribedMeal_DescribeFailureLeavesMealFailedThenRetryReRunsDescribe(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	fake := &vision.Fake{DescribeErr: context.DeadlineExceeded}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.CreateDescribedMeal(w, withClaims(describeMealHTTPRequest(map[string]any{
		"description": "some soup",
	}), userID))
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 (meal created even though analysis failed), got %d: %s", w.Code, w.Body.String())
	}
	var meal database.FoodMeal
	json.NewDecoder(w.Body).Decode(&meal) //nolint:errcheck
	if meal.Status != database.MealStatusFailed {
		t.Fatalf("expected failed status, got %s", meal.Status)
	}
	if meal.Description != "some soup" {
		t.Errorf("expected description still stored, got %q", meal.Description)
	}
	if len(fake.DescribeCalls) != 1 {
		t.Fatalf("expected one Describe call, got %d", len(fake.DescribeCalls))
	}

	// Now retry, with Describe succeeding.
	fake.DescribeErr = nil
	fake.DescribeResult = &vision.RecognizeResult{
		Items: []vision.Item{{Name: "Soup", WeightGrams: 300, Confidence: 0.6}},
	}
	retryReq := httptest.NewRequest(http.MethodPost, "/api/food/meals/"+meal.ID.String()+"/retry", nil)
	retryReq = mux.SetURLVars(retryReq, map[string]string{"id": meal.ID.String()})
	w2 := httptest.NewRecorder()
	h.RetryMeal(w2, withClaims(retryReq, userID))
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 on retry, got %d: %s", w2.Code, w2.Body.String())
	}
	var retried database.FoodMeal
	json.NewDecoder(w2.Body).Decode(&retried) //nolint:errcheck
	if retried.Status != database.MealStatusPendingReview {
		t.Fatalf("expected pending_review after retry, got %s", retried.Status)
	}
	if len(fake.DescribeCalls) != 2 {
		t.Fatalf("expected RetryMeal to re-run Describe (not 409), got %d Describe calls", len(fake.DescribeCalls))
	}
}

// The central case this change fixes: a non-English Display Language must
// reach vision.Describe as-is, must not query USDA/OFF (English-vocabulary
// indexes), and must fall back to the model's own estimated_profile.
func TestCreateDescribedMeal_NonEnglishDisplayLanguageSkipsReferenceDatabases(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	idx := buildUSDAIndex(t, usdaFood(1, "Borscht", 49))

	putH := server.PutUserSettingsHandler(st)
	wSettings := httptest.NewRecorder()
	putH.ServeHTTP(wSettings, withClaims(httptest.NewRequest(http.MethodPut, "/api/users/me/settings", strings.NewReader(`{"display_language":"ru"}`)), userID))
	if wSettings.Code != http.StatusOK {
		t.Fatalf("PUT settings: expected 200, got %d", wSettings.Code)
	}

	fake := &vision.Fake{
		DescribeResult: &vision.RecognizeResult{
			Items: []vision.Item{{
				Name: "борщ", CanonicalName: "borscht", WeightGrams: 350, Confidence: 0.8,
				EstimatedProfile: &database.NutrientProfile{CaloriesPer100g: 49, ProteinPer100g: 1.5},
			}},
		},
	}
	h := server.NewFoodHandlers(st, idx, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.CreateDescribedMeal(w, withClaims(describeMealHTTPRequest(map[string]any{
		"description": "тарелка борща со сметаной",
	}), userID))
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if len(fake.DescribeCalls) != 1 || fake.DescribeCalls[0].DisplayLanguage != "ru" {
		t.Fatalf("expected Describe called with display language ru, got %+v", fake.DescribeCalls)
	}

	var meal database.FoodMeal
	json.NewDecoder(w.Body).Decode(&meal) //nolint:errcheck
	if len(meal.Items) != 1 {
		t.Fatalf("expected one item, got %+v", meal.Items)
	}
	item := meal.Items[0]
	if item.FdcID != nil {
		t.Errorf("expected no USDA candidate bound for a non-English item, got fdc_id %v", *item.FdcID)
	}
	if item.OffCode != nil {
		t.Errorf("expected no OFF candidate bound, got off_code %v", *item.OffCode)
	}
	if item.MacroSource != database.MacroSourceEstimated {
		t.Errorf("expected estimated macro source, got %s", item.MacroSource)
	}
}

// A Describe response carrying clarification_questions routes the meal into
// pending_clarification, exactly like Recognize does — and the existing
// ClarifyMeal endpoint, which is already text-only, carries it through to
// pending_review without ever touching a photo.
func TestCreateDescribedMeal_ClarificationQuestionsRouteThroughExistingClarifyFlow(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	fake := &vision.Fake{
		DescribeResult: &vision.RecognizeResult{
			ClarificationQuestions: []string{"How big was the portion?"},
		},
	}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.CreateDescribedMeal(w, withClaims(describeMealHTTPRequest(map[string]any{
		"description": "some soup",
	}), userID))
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var meal database.FoodMeal
	json.NewDecoder(w.Body).Decode(&meal) //nolint:errcheck
	if meal.Status != database.MealStatusPendingClarification {
		t.Fatalf("expected pending_clarification, got %s", meal.Status)
	}
	if meal.PhotoPath != "" {
		t.Errorf("expected no photo path, got %q", meal.PhotoPath)
	}

	fake.ClarifyResult = &vision.RecognizeResult{
		Items: []vision.Item{{Name: "Soup", WeightGrams: 300, Confidence: 0.7}},
	}
	clarifyReq := httptest.NewRequest(http.MethodPost, "/api/food/meals/"+meal.ID.String()+"/clarify",
		bytes.NewBufferString(`{"answers":["about 300 grams"]}`))
	clarifyReq = mux.SetURLVars(clarifyReq, map[string]string{"id": meal.ID.String()})
	w2 := httptest.NewRecorder()
	h.ClarifyMeal(w2, withClaims(clarifyReq, userID))
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
	var clarified database.FoodMeal
	json.NewDecoder(w2.Body).Decode(&clarified) //nolint:errcheck
	if clarified.Status != database.MealStatusPendingReview {
		t.Fatalf("expected pending_review after clarify, got %s", clarified.Status)
	}
	if len(fake.ClarifyCalls) != 1 {
		t.Fatalf("expected one Clarify call, got %d", len(fake.ClarifyCalls))
	}
}
