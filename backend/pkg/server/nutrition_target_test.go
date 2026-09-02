package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
		MeasuredWeightKg   float64 `json:"measured_weight_kg"`
		GoalWeightKg       float64 `json:"goal_weight_kg"`
		HeightM            float64 `json:"height_m"`
		AgeYears           int     `json:"age_years"`
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
	// Echoed inputs must be the values actually seeded, not e.g. a
	// weight/goal-weight field swap in the handler's wiring.
	if resp.MeasuredWeightKg != 80 {
		t.Errorf("measured_weight_kg = %v, want 80", resp.MeasuredWeightKg)
	}
	if resp.GoalWeightKg != 75 {
		t.Errorf("goal_weight_kg = %v, want 75", resp.GoalWeightKg)
	}
	if resp.HeightM != 1.80 {
		t.Errorf("height_m = %v, want 1.80", resp.HeightM)
	}
	wantAge := calendarAgeForTest(time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC), time.Now().UTC())
	if resp.AgeYears != wantAge {
		t.Errorf("age_years = %v, want %v", resp.AgeYears, wantAge)
	}
	if resp.CarbsGrams <= 0 || resp.FatGrams <= 0 {
		t.Errorf("expected positive carbs/fat, got carbs=%d fat=%d", resp.CarbsGrams, resp.FatGrams)
	}
}

// insertFutureRecord seeds a row via storage.InsertRecord directly (bypassing
// CreateRecordHandler's future-date rejection), simulating the Health
// Connect/Libra import and webhook ingest paths, none of which validate
// against future timestamps the way manual entry does.
func insertFutureRecord(t *testing.T, st database.Storage, userID, familyID uuid.UUID, table, timeCol, valueCol string, ts time.Time, value float64) {
	t.Helper()
	if _, err := st.InsertRecord(table, timeCol, valueCol, familyID, userID, ts, value); err != nil {
		t.Fatalf("insertFutureRecord %s: %v", table, err)
	}
}

// A future-dated weight/height/weight_goal row (as can arrive via import or
// webhook ingest, which don't reject future timestamps) must not count as
// the "latest" reading: with only future rows present, the endpoint reports
// the same unmet-precondition reasons as if there were no rows at all.
func TestNutritionTarget_FutureOnlyMeasurementsAreNotUsed(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	setProfile(t, st, userID, `{"birthdate":"1990-01-01","sex":"male"}`)

	future := time.Now().UTC().AddDate(0, 1, 0)
	insertFutureRecord(t, st, userID, familyID, "weights", "time", "kilograms", future, 80)
	insertFutureRecord(t, st, userID, familyID, "heights", "time", "meters", future, 1.80)

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

// With both a legitimate past-dated row and a bogus future-dated row present,
// the endpoint must use the past value, not the future one, even though the
// future row's time sorts later.
func TestNutritionTarget_IgnoresFutureDatedRowsPreferringPast(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	setProfile(t, st, userID, `{"birthdate":"1990-01-01","sex":"male","activity_override":"moderate"}`)

	createRecord(t, st, userID, "weight", 80)
	createRecord(t, st, userID, "height", 1.80)
	createRecord(t, st, userID, "weight_goal", 75)

	future := time.Now().UTC().AddDate(0, 1, 0)
	insertFutureRecord(t, st, userID, familyID, "weights", "time", "kilograms", future, 999)
	insertFutureRecord(t, st, userID, familyID, "heights", "time", "meters", future, 2.50)
	insertFutureRecord(t, st, userID, familyID, "weight_goals", "time", "kilograms", future, 111)

	h := server.NutritionTargetHandler(st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, newNutritionTargetRequest(userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		MeasuredWeightKg float64 `json:"measured_weight_kg"`
		GoalWeightKg     float64 `json:"goal_weight_kg"`
		HeightM          float64 `json:"height_m"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.MeasuredWeightKg != 80 {
		t.Errorf("measured_weight_kg = %v, want 80 (past row), not the future row's 999", resp.MeasuredWeightKg)
	}
	if resp.GoalWeightKg != 75 {
		t.Errorf("goal_weight_kg = %v, want 75 (past row), not the future row's 111", resp.GoalWeightKg)
	}
	if resp.HeightM != 1.80 {
		t.Errorf("height_m = %v, want 1.80 (past row), not the future row's 2.50", resp.HeightM)
	}
}

// calendarAgeForTest duplicates calendarAge's "completed years" rule for use
// from server_test, which cannot import the unexported function directly.
func calendarAgeForTest(birthdate, now time.Time) int {
	age := now.Year() - birthdate.Year()
	if now.Month() < birthdate.Month() ||
		(now.Month() == birthdate.Month() && now.Day() < birthdate.Day()) {
		age--
	}
	return age
}

// The primary, non-override path: a complete profile+measurements+goal plus
// a full trailing-window of real steps history infers a tier from the data
// itself, rather than only being exercised via activity_override.
func TestNutritionTarget_SuccessWithInferredActivityFromSteps(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	setProfile(t, st, userID, `{"birthdate":"1990-01-01","sex":"male"}`)
	createRecord(t, st, userID, "weight", 80)
	createRecord(t, st, userID, "height", 1.80)
	createRecord(t, st, userID, "weight_goal", 75)

	// steps is not in the manual-write allowlist (POST /api/data/steps is
	// 403) and its table requires a source_payload_id InsertRecord doesn't
	// set, so seed it directly the way pkg/database's own aggregate tests
	// do: a raw database.Steps row per day.
	now := time.Now().UTC()
	for i := 1; i <= 7; i++ {
		ts := now.AddDate(0, 0, -i)
		rec := database.Steps{
			UserID:          userID,
			SourcePayloadID: uuid.New(),
			StartTime:       ts,
			EndTime:         ts,
			Count:           3000,
		}
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		if err := st.DB().Create(&rec).Error; err != nil {
			t.Fatalf("create steps day -%d: %v", i, err)
		}
	}

	h := server.NutritionTargetHandler(st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, newNutritionTargetRequest(userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		ActivityTier       string  `json:"activity_tier"`
		ActivityMultiplier float64 `json:"activity_multiplier"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.ActivityTier != "Sedentary" || resp.ActivityMultiplier != 1.2 {
		t.Errorf("activity tier/multiplier = %q/%v, want %q/%v", resp.ActivityTier, resp.ActivityMultiplier, "Sedentary", 1.2)
	}
}

// Two overlapping step records per trailing day (as two sync sources writing
// the same walk would produce) must not push the inferred Activity Level
// tier up: fetchDailySteps collapses the overlap before
// trailingStepsAverage ever sees the numbers. Each day's real, uncollapsed
// total is 6000 (-> "Lightly active", 5000-7499/day); if the duplicate
// record were summed instead of collapsed, the same days would read as
// 12000/day and cross into "Very active" (10000-12499/day).
func TestNutritionTarget_DuplicatedStepsDoNotInflateActivityTier(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	setProfile(t, st, userID, `{"birthdate":"1990-01-01","sex":"male"}`)
	createRecord(t, st, userID, "weight", 80)
	createRecord(t, st, userID, "height", 1.80)
	createRecord(t, st, userID, "weight_goal", 75)

	now := time.Now().UTC()
	for i := 1; i <= 7; i++ {
		ts := now.AddDate(0, 0, -i)
		// Two overlapping intervals for the same walk: idx_steps_user_time is
		// unique on (user_id, start_time), so they can't share a start_time —
		// offset the second by a minute and nest it fully inside the first, so
		// the collapse drops it whole rather than trimming it.
		for _, rec := range []database.Steps{
			{UserID: userID, SourcePayloadID: uuid.New(), StartTime: ts, EndTime: ts.Add(2 * time.Hour), Count: 6000},
			{UserID: userID, SourcePayloadID: uuid.New(), StartTime: ts.Add(time.Minute), EndTime: ts.Add(time.Hour), Count: 6000},
		} {
			rec.ID = uuid.New()
			rec.FamilyID = familyID
			if err := st.DB().Create(&rec).Error; err != nil {
				t.Fatalf("create steps day -%d: %v", i, err)
			}
		}
	}

	h := server.NutritionTargetHandler(st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, newNutritionTargetRequest(userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		ActivityTier       string  `json:"activity_tier"`
		ActivityMultiplier float64 `json:"activity_multiplier"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.ActivityTier != "Lightly active" || resp.ActivityMultiplier != 1.375 {
		t.Errorf("activity tier/multiplier = %q/%v, want %q/%v (duplicate steps must not inflate the tier)",
			resp.ActivityTier, resp.ActivityMultiplier, "Lightly active", 1.375)
	}
}

// A genuine storage failure while resolving inputs must surface as a 500,
// never be folded into one of the endpoint's 422 "unmet precondition"
// reasons, which are reserved for "the user hasn't set this up yet".
func TestNutritionTarget_StorageFailureReturns500(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	setProfile(t, st, userID, `{"birthdate":"1990-01-01","sex":"male"}`)

	sqlDB, err := st.DB().DB()
	if err != nil {
		t.Fatalf("DB(): %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	h := server.NutritionTargetHandler(st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, newNutritionTargetRequest(userID))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for a genuine storage failure, got %d: %s", w.Code, w.Body.String())
	}
}
