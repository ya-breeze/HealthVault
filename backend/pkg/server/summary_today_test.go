package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/server"
)

func newSummaryTodayRequest(userID uuid.UUID, query string) *http.Request {
	url := "/api/summary/today"
	if query != "" {
		url += "?" + query
	}
	return withClaims(httptest.NewRequest(http.MethodGet, url, nil), userID)
}

type summaryTodayTestResponse struct {
	Date                 string     `json:"date"`
	CaloriesConsumed     float64    `json:"calories_consumed"`
	ProteinGramsConsumed float64    `json:"protein_grams_consumed"`
	CarbsGramsConsumed   float64    `json:"carbs_grams_consumed"`
	FatGramsConsumed     float64    `json:"fat_grams_consumed"`
	MealCount            int        `json:"meal_count"`
	LastLoggedAt         *time.Time `json:"last_logged_at"`
	DisplayLanguage      string     `json:"display_language"`
	Target               struct {
		Available    bool   `json:"available"`
		Reason       string `json:"reason"`
		Calories     int    `json:"calories"`
		ProteinGrams int    `json:"protein_grams"`
		CarbsGrams   int    `json:"carbs_grams"`
		FatGrams     int    `json:"fat_grams"`
		BMR          int    `json:"bmr"`
	} `json:"target"`
	Recommendation any `json:"recommendation"`
}

func decodeSummaryToday(t *testing.T, w *httptest.ResponseRecorder) summaryTodayTestResponse {
	t.Helper()
	var body summaryTodayTestResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return body
}

func TestSummaryToday_Unauthenticated(t *testing.T) {
	st := newFoodTestStorage(t)
	h := server.SummaryTodayHandler(st)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/summary/today", nil))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// TestSummaryToday_TargetUnavailableReasons covers 4.3: every one of
// nutrition-target's four unmet-precondition reasons must still surface as
// HTTP 200 here (never 422, unlike GET /api/users/me/nutrition-target
// itself), with target.available=false and the same reason code.
func TestSummaryToday_TargetUnavailableReasons(t *testing.T) {
	cases := []struct {
		name       string
		setup      func(t *testing.T, st database.Storage, userID, familyID uuid.UUID)
		wantReason string
	}{
		{
			name:       "missing_profile",
			setup:      func(t *testing.T, st database.Storage, userID, familyID uuid.UUID) {},
			wantReason: "missing_profile",
		},
		{
			name: "missing_measurements",
			setup: func(t *testing.T, st database.Storage, userID, familyID uuid.UUID) {
				setProfile(t, st, userID, `{"birthdate":"1990-01-01","sex":"male"}`)
			},
			wantReason: "missing_measurements",
		},
		{
			name: "missing_goal_weight",
			setup: func(t *testing.T, st database.Storage, userID, familyID uuid.UUID) {
				setProfile(t, st, userID, `{"birthdate":"1990-01-01","sex":"male"}`)
				createRecord(t, st, userID, "weight", 80)
				createRecord(t, st, userID, "height", 1.80)
			},
			wantReason: "missing_goal_weight",
		},
		{
			name: "insufficient_activity_data",
			setup: func(t *testing.T, st database.Storage, userID, familyID uuid.UUID) {
				setProfile(t, st, userID, `{"birthdate":"1990-01-01","sex":"male"}`)
				createRecord(t, st, userID, "weight", 80)
				createRecord(t, st, userID, "height", 1.80)
				createRecord(t, st, userID, "weight_goal", 75)
			},
			wantReason: "insufficient_activity_data",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := newFoodTestStorage(t)
			userID, familyID := seedFoodUser(t, st)
			tc.setup(t, st, userID, familyID)

			h := server.SummaryTodayHandler(st)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, newSummaryTodayRequest(userID, ""))

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
			}
			resp := decodeSummaryToday(t, w)
			if resp.Target.Available {
				t.Errorf("target.available = true, want false")
			}
			if resp.Target.Reason != tc.wantReason {
				t.Errorf("target.reason = %q, want %q", resp.Target.Reason, tc.wantReason)
			}
		})
	}
}

// TestSummaryToday_TargetAvailable covers 4.3's success case: with every
// nutrition-target precondition met, the endpoint embeds the computed target
// with available=true and no reason.
func TestSummaryToday_TargetAvailable(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	setProfile(t, st, userID, `{"birthdate":"1990-01-01","sex":"male","activity_override":"moderate"}`)
	createRecord(t, st, userID, "weight", 80)
	createRecord(t, st, userID, "height", 1.80)
	createRecord(t, st, userID, "weight_goal", 75)

	h := server.SummaryTodayHandler(st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, newSummaryTodayRequest(userID, ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	resp := decodeSummaryToday(t, w)
	if !resp.Target.Available {
		t.Fatalf("target.available = false, want true: %+v", resp.Target)
	}
	if resp.Target.Reason != "" {
		t.Errorf("target.reason = %q, want empty when available", resp.Target.Reason)
	}
	if resp.Target.Calories <= 0 || resp.Target.ProteinGrams <= 0 {
		t.Errorf("expected positive target calories/protein, got %+v", resp.Target)
	}
	if resp.Recommendation != nil {
		t.Errorf("recommendation = %v, want null", resp.Recommendation)
	}

}

// TestSummaryToday_ZeroTargetFieldIsStillPresent pins the absence of
// `omitempty` on summaryTargetPayload's numeric fields.
//
// The scenario is the one that makes the tag bite, not a hypothetical: a goal
// weight typed in pounds (165) against an ordinary sedentary TDEE (~2100 kcal
// for this profile). Protein at 1.6 g/kg of goal plus the 0.8 g/kg fat floor
// together exceed the calorie budget, so computeNutritionTarget clamps carbs
// to zero — a complete, available target that happens to contain a zero.
//
// With `omitempty` that key vanishes, and a client whose type declares it
// required (the frontend's TodaySummaryTarget) reads undefined and renders
// "Carbs 130/NaN g" on the dashboard. A missing key must mean "no target",
// which `available` already reports; it must never also mean "this number
// happened to be zero".
//
// Asserted against the raw JSON deliberately: decoding into a struct cannot
// tell a missing key from a zero, so a struct-based assertion would pass with
// the tag restored.
func TestSummaryToday_ZeroTargetFieldIsStillPresent(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	setProfile(t, st, userID, `{"birthdate":"1990-01-01","sex":"male","activity_override":"sedentary"}`)
	createRecord(t, st, userID, "weight", 80)
	createRecord(t, st, userID, "height", 1.80)
	createRecord(t, st, userID, "weight_goal", 165)

	h := server.SummaryTodayHandler(st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, newSummaryTodayRequest(userID, ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	resp := decodeSummaryToday(t, w)
	if !resp.Target.Available {
		t.Fatalf("target.available = false, want true: %+v", resp.Target)
	}
	if resp.Target.CarbsGrams != 0 {
		t.Fatalf("this fixture must produce a zero carbs target for the assertion below to mean "+
			"anything, got %d — recheck computeNutritionTarget's clamp", resp.Target.CarbsGrams)
	}

	var raw struct {
		Target map[string]json.RawMessage `json:"target"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	for _, key := range []string{"calories", "protein_grams", "carbs_grams", "fat_grams", "bmr"} {
		if _, ok := raw.Target[key]; !ok {
			t.Errorf("target.%s missing from the response body; every numeric field must be present "+
				"whenever available=true, zero included", key)
		}
	}
}

// TestSummaryToday_ReflectsCallersOwnMeals covers the endpoint's core happy
// path: with confirmed meals logged today, the handler's JSON response
// carries the caller's aggregated macros, meal_count, and a non-nil
// last_logged_at through to serialization — exercising the
// TodaySummaryRow-to-summaryTodayResponse wiring (including the
// HasLastLoggedAt pointer branch in summary_today.go) that
// TestSummaryToday_IgnoresUserQueryParam and the zero-meal cases above never
// touch.
func TestSummaryToday_ReflectsCallersOwnMeals(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	loggedAt := time.Now().UTC().Add(-1 * time.Hour)
	confirmed := database.FoodMeal{
		UserID: userID, Status: database.MealStatusConfirmed, LoggedAt: loggedAt,
		Name: "Lunch", Calories: 500, ProteinGrams: 30, CarbsGrams: 40, FatGrams: 15,
	}
	confirmed.ID = uuid.New()
	confirmed.FamilyID = familyID
	if err := st.DB().Create(&confirmed).Error; err != nil {
		t.Fatalf("create confirmed meal: %v", err)
	}
	// A non-confirmed meal logged later: counts toward meal_count and
	// last_logged_at, but must not contribute to the macro sums.
	pending := database.FoodMeal{
		UserID: userID, Status: database.MealStatusPendingReview, LoggedAt: loggedAt.Add(time.Minute),
		Name: "Snack", Calories: 999, ProteinGrams: 99, CarbsGrams: 99, FatGrams: 99,
	}
	pending.ID = uuid.New()
	pending.FamilyID = familyID
	if err := st.DB().Create(&pending).Error; err != nil {
		t.Fatalf("create pending meal: %v", err)
	}

	h := server.SummaryTodayHandler(st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, newSummaryTodayRequest(userID, ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	resp := decodeSummaryToday(t, w)
	if resp.MealCount != 2 {
		t.Errorf("meal_count = %d, want 2 (both statuses count)", resp.MealCount)
	}
	if resp.CaloriesConsumed != 500 || resp.ProteinGramsConsumed != 30 ||
		resp.CarbsGramsConsumed != 40 || resp.FatGramsConsumed != 15 {
		t.Errorf("macro sums = %+v, want only the confirmed meal's macros", resp)
	}
	if resp.LastLoggedAt == nil {
		t.Fatalf("last_logged_at = nil, want non-nil")
	}
	wantLastLoggedAt := pending.LoggedAt
	if !resp.LastLoggedAt.Equal(wantLastLoggedAt) {
		t.Errorf("last_logged_at = %v, want %v (the later, pending meal)", resp.LastLoggedAt, wantLastLoggedAt)
	}
}

// TestSummaryToday_IgnoresUserQueryParam covers 4.4's self-only half: a
// ?user= override naming a different user with data of its own must not
// change the caller's own (empty) result.
func TestSummaryToday_IgnoresUserQueryParam(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	otherUserID := uuid.New()

	createMealAt(t, st, otherUserID, familyID, database.MealStatusConfirmed, time.Now().UTC())

	h := server.SummaryTodayHandler(st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, newSummaryTodayRequest(userID, "user="+otherUserID.String()))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	resp := decodeSummaryToday(t, w)
	if resp.MealCount != 0 {
		t.Errorf("meal_count = %d, want 0 (caller's own data, not the other user's)", resp.MealCount)
	}
}
