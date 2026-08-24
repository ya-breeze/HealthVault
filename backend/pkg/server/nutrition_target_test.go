package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/server"
)

func setProfile(t *testing.T, st database.Storage, userID uuid.UUID, body string) {
	t.Helper()
	putH := server.PutUserSettingsHandler(st)
	w := httptest.NewRecorder()
	putH.ServeHTTP(w, withClaims(httptest.NewRequest(http.MethodPut, "/api/users/me/settings", bytes.NewBufferString(body)), userID))
	if w.Code != http.StatusOK {
		t.Fatalf("setProfile PUT: expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func createRecord(t *testing.T, st database.Storage, userID uuid.UUID, typeName string, value float64) {
	t.Helper()
	h := server.CreateRecordHandler(st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, newCreateRequest(typeName, map[string]any{"value": value}, userID))
	if w.Code != http.StatusCreated {
		t.Fatalf("createRecord %s=%v: expected 201, got %d: %s", typeName, value, w.Code, w.Body.String())
	}
}

func newNutritionTargetRequest(userID uuid.UUID) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/users/me/nutrition-target", nil)
	return withClaims(r, userID)
}

func decodeUnprocessableReason(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return body["error"]
}

func TestNutritionTarget_Unauthenticated(t *testing.T) {
	st := newFoodTestStorage(t)
	h := server.NutritionTargetHandler(st)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/users/me/nutrition-target", nil))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// No profile at all: missing_profile must be reported first, ahead of the
// measurements/goal/activity reasons that are also unmet.
func TestNutritionTarget_MissingProfileReportedFirst(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	h := server.NutritionTargetHandler(st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, newNutritionTargetRequest(userID))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
	if reason := decodeUnprocessableReason(t, w); reason != "missing_profile" {
		t.Errorf("reason = %q, want missing_profile", reason)
	}
}

// Profile set, but no weight/height records: missing_measurements must be
// reported ahead of missing_goal_weight and insufficient_activity_data,
// which are also unmet.
func TestNutritionTarget_MissingMeasurementsReportedBeforeLaterReasons(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	setProfile(t, st, userID, `{"birthdate":"1990-01-01","sex":"male"}`)

	h := server.NutritionTargetHandler(st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, newNutritionTargetRequest(userID))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
	if reason := decodeUnprocessableReason(t, w); reason != "missing_measurements" {
		t.Errorf("reason = %q, want missing_measurements", reason)
	}
}

// Profile and measurements set, but no weight_goal record: missing_goal_weight
// must be reported ahead of insufficient_activity_data, which is also unmet.
func TestNutritionTarget_MissingGoalWeightReportedBeforeActivityReason(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	setProfile(t, st, userID, `{"birthdate":"1990-01-01","sex":"male"}`)
	createRecord(t, st, userID, "weight", 80)
	createRecord(t, st, userID, "height", 1.80)

	h := server.NutritionTargetHandler(st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, newNutritionTargetRequest(userID))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
	if reason := decodeUnprocessableReason(t, w); reason != "missing_goal_weight" {
		t.Errorf("reason = %q, want missing_goal_weight", reason)
	}
}

// Everything but activity data is present, and no activity_override is set,
// so the user's (empty) steps history leaves fewer than 7 valid days: the
// last of the four reasons, insufficient_activity_data, is reported.
func TestNutritionTarget_InsufficientActivityDataIsLastReason(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	setProfile(t, st, userID, `{"birthdate":"1990-01-01","sex":"male"}`)
	createRecord(t, st, userID, "weight", 80)
	createRecord(t, st, userID, "height", 1.80)
	createRecord(t, st, userID, "weight_goal", 75)

	h := server.NutritionTargetHandler(st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, newNutritionTargetRequest(userID))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
	if reason := decodeUnprocessableReason(t, w); reason != "insufficient_activity_data" {
		t.Errorf("reason = %q, want insufficient_activity_data", reason)
	}
}

// A valid activity_override bypasses the steps-history requirement
// entirely, so a complete profile+measurements+goal+override succeeds with
// 200 and echoes the override's tier/multiplier.
func TestNutritionTarget_SuccessWithActivityOverride(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	setProfile(t, st, userID, `{"birthdate":"1990-01-01","sex":"male","activity_override":"moderate"}`)
	createRecord(t, st, userID, "weight", 80)
	createRecord(t, st, userID, "height", 1.80)
	createRecord(t, st, userID, "weight_goal", 75)

	h := server.NutritionTargetHandler(st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, newNutritionTargetRequest(userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Calories           int     `json:"calories"`
		ProteinGrams       int     `json:"protein_grams"`
		CarbsGrams         int     `json:"carbs_grams"`
		FatGrams           int     `json:"fat_grams"`
		ActivityTier       string  `json:"activity_tier"`
		ActivityMultiplier float64 `json:"activity_multiplier"`
		Sex                string  `json:"sex"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.ActivityTier != "Moderately active" || resp.ActivityMultiplier != 1.55 {
		t.Errorf("activity tier/multiplier = %q/%v, want %q/%v", resp.ActivityTier, resp.ActivityMultiplier, "Moderately active", 1.55)
	}
	if resp.Sex != "male" {
		t.Errorf("sex = %q, want male", resp.Sex)
	}
	if resp.Calories <= 0 || resp.ProteinGrams <= 0 {
		t.Errorf("expected positive calories/protein, got calories=%d protein=%d", resp.Calories, resp.ProteinGrams)
	}
}
