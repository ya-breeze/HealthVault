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

func dashboardRequest(query string) *http.Request {
	path := "/api/dashboard"
	if query != "" {
		path += "?" + query
	}
	return httptest.NewRequest(http.MethodGet, path, nil)
}

func decodeDashboardBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode dashboard response: %v; body=%s", err, w.Body.String())
	}
	return body
}

func TestDashboardHandler_UnauthenticatedReturns401WithoutEnvelope(t *testing.T) {
	h := server.DashboardHandler(newFoodTestStorage(t))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, dashboardRequest(""))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	if len(w.Body.Bytes()) == 0 || w.Body.String()[0] == '{' {
		t.Fatalf("unauthenticated response looks like a read-model envelope: %s", w.Body.String())
	}
}

func TestDashboardHandler_IsSelfOnlyDespiteUserQueryParameter(t *testing.T) {
	storage := newFoodTestStorage(t)
	callerID, familyID := seedFoodUser(t, storage)
	otherID := uuid.New()
	createMealAt(t, storage, otherID, familyID, database.MealStatusPendingReview, time.Now().UTC())
	otherSteps := database.Steps{UserID: otherID, SourcePayloadID: uuid.New(), StartTime: time.Now().UTC(), EndTime: time.Now().UTC().Add(time.Minute), Count: 123}
	otherSteps.ID = uuid.New()
	otherSteps.FamilyID = familyID
	if err := storage.DB().Create(&otherSteps).Error; err != nil {
		t.Fatalf("create other user's steps: %v", err)
	}

	w := httptest.NewRecorder()
	server.DashboardHandler(storage).ServeHTTP(w, withClaims(dashboardRequest("user="+otherID.String()), callerID))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	body := decodeDashboardBody(t, w)
	presence := body["presence"].(map[string]any)
	if presence["status"] != "ok" || presence["value"].(map[string]any)["steps"] != false {
		t.Fatalf("dashboard used query user instead of claims user: %#v", presence)
	}
	needs := body["needs_attention"].(map[string]any)
	if needs["status"] != "ok" || needs["count"] != float64(0) {
		t.Fatalf("needs_attention was not scoped to caller: %#v", needs)
	}
}

func TestDashboardHandler_ContractIncludesOpaqueSettingsPresenceAllAggregatesAndCount(t *testing.T) {
	storage := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, storage)
	if err := storage.UpsertUserSettings(userID, familyID, `{"timezone":"UTC","future_key":{"enabled":true}}`); err != nil {
		t.Fatalf("settings: %v", err)
	}
	now := time.Now().UTC()
	steps := database.Steps{UserID: userID, SourcePayloadID: uuid.New(), StartTime: now.Add(-time.Hour), EndTime: now.Add(-time.Minute), Count: 100}
	steps.ID, steps.FamilyID = uuid.New(), familyID
	if err := storage.DB().Create(&steps).Error; err != nil {
		t.Fatalf("steps: %v", err)
	}
	weight := database.Weight{UserID: userID, SourcePayloadID: ptrUUIDForPresence(uuid.New()), Time: now.Add(-2 * time.Hour), Kilograms: 70}
	weight.ID, weight.FamilyID = uuid.New(), familyID
	if err := storage.DB().Create(&weight).Error; err != nil {
		t.Fatalf("weight: %v", err)
	}
	bloodPressure := database.BloodPressure{
		UserID: userID, SourcePayloadID: uuid.New(), Time: now.Add(-90 * time.Minute), Systolic: 120, Diastolic: 80,
	}
	bloodPressure.ID, bloodPressure.FamilyID = uuid.New(), familyID
	if err := storage.DB().Create(&bloodPressure).Error; err != nil {
		t.Fatalf("blood pressure: %v", err)
	}
	createMealAt(t, storage, userID, familyID, database.MealStatusPendingReview, now.Add(-3*time.Hour))

	w := httptest.NewRecorder()
	server.DashboardHandler(storage).ServeHTTP(w, withClaims(dashboardRequest(""), userID))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	body := decodeDashboardBody(t, w)
	settings := body["settings"].(map[string]any)
	if settings["status"] != "ok" || settings["value"].(map[string]any)["future_key"].(map[string]any)["enabled"] != true {
		t.Fatalf("opaque settings key did not round-trip: %#v", settings)
	}
	presence := body["presence"].(map[string]any)
	value := presence["value"].(map[string]any)
	if len(value) != len(presenceTypeNames) || value["steps"] != true || value["weight"] != true {
		t.Fatalf("presence = %#v, want complete map with steps and weight", value)
	}
	aggregates := body["aggregates"].(map[string]any)
	if len(aggregates) != 8 {
		t.Fatalf("aggregate keys = %d, want 8: %#v", len(aggregates), aggregates)
	}
	for _, metric := range []string{"steps", "heart_rate", "sleep", "heart_rate_variability", "distance", "weight", "blood_pressure", "oxygen_saturation"} {
		section, ok := aggregates[metric].(map[string]any)
		if !ok || section["status"] != "ok" {
			t.Errorf("aggregate %q = %#v, want success section", metric, aggregates[metric])
		}
		if rows, ok := section["rows"].([]any); !ok {
			t.Errorf("aggregate %q rows = %#v, want array", metric, section["rows"])
		} else if rows == nil {
			t.Errorf("aggregate %q rows serialized null, want []", metric)
		}
	}
	requireAggregateRow := func(metric string, fields ...string) map[string]any {
		t.Helper()
		rows := aggregates[metric].(map[string]any)["rows"].([]any)
		if len(rows) != 1 {
			t.Fatalf("aggregate %q rows = %#v, want one seeded row", metric, rows)
		}
		row, ok := rows[0].(map[string]any)
		if !ok {
			t.Fatalf("aggregate %q row = %#v, want object", metric, rows[0])
		}
		for _, field := range fields {
			if _, ok := row[field]; !ok {
				t.Errorf("aggregate %q row = %#v, missing field %q", metric, row, field)
			}
		}
		return row
	}
	stepsRow := requireAggregateRow("steps", "bucket_start", "count", "sum")
	if stepsRow["count"] != float64(1) || stepsRow["sum"] != float64(100) {
		t.Errorf("steps row = %#v, want count 1 and sum 100", stepsRow)
	}
	weightRow := requireAggregateRow("weight", "bucket_start", "count", "avg", "min", "max")
	if weightRow["avg"] != float64(70) || weightRow["min"] != float64(70) || weightRow["max"] != float64(70) {
		t.Errorf("weight row = %#v, want avg/min/max 70", weightRow)
	}
	bloodPressureRow := requireAggregateRow(
		"blood_pressure", "bucket_start", "count",
		"systolic_avg", "systolic_min", "systolic_max",
		"diastolic_avg", "diastolic_min", "diastolic_max",
	)
	if bloodPressureRow["systolic_avg"] != float64(120) || bloodPressureRow["diastolic_avg"] != float64(80) {
		t.Errorf("blood pressure row = %#v, want systolic 120 and diastolic 80", bloodPressureRow)
	}
	needs := body["needs_attention"].(map[string]any)
	if needs["status"] != "ok" || needs["count"] != float64(1) {
		t.Fatalf("needs_attention = %#v, want count 1", needs)
	}
}

func TestDashboardHandler_MissingSettingsNormalizesToEmptyObjectAndEmptyAggregatesToArrays(t *testing.T) {
	storage := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, storage)
	w := httptest.NewRecorder()
	server.DashboardHandler(storage).ServeHTTP(w, withClaims(dashboardRequest(""), userID))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	body := decodeDashboardBody(t, w)
	settings := body["settings"].(map[string]any)
	if settings["status"] != "ok" || len(settings["value"].(map[string]any)) != 0 {
		t.Fatalf("settings = %#v, want {status:ok,value:{}}", settings)
	}
	for metric, raw := range body["aggregates"].(map[string]any) {
		section := raw.(map[string]any)
		if section["status"] != "ok" {
			t.Fatalf("aggregate %q = %#v, want ok", metric, section)
		}
		if rows, ok := section["rows"].([]any); !ok || rows == nil {
			t.Fatalf("aggregate %q rows = %#v, want empty array", metric, section["rows"])
		}
	}
}

func TestDashboardHandler_SettingsFailureReturns200WithIndependentSections(t *testing.T) {
	storage := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, storage)
	if err := storage.DB().Exec("DROP TABLE user_settings").Error; err != nil {
		t.Fatalf("drop settings table: %v", err)
	}

	w := httptest.NewRecorder()
	server.DashboardHandler(storage).ServeHTTP(w, withClaims(dashboardRequest(""), userID))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 partial response: %s", w.Code, w.Body.String())
	}
	body := decodeDashboardBody(t, w)
	if body["settings"].(map[string]any)["status"] != "error" {
		t.Fatalf("settings = %#v, want error", body["settings"])
	}
	if body["presence"].(map[string]any)["status"] != "ok" || body["needs_attention"].(map[string]any)["status"] != "ok" {
		t.Fatalf("independent sections failed with settings: presence=%#v needs=%#v", body["presence"], body["needs_attention"])
	}
	for metric, raw := range body["aggregates"].(map[string]any) {
		if raw.(map[string]any)["status"] != "error" {
			t.Errorf("aggregate %q = %#v, want error when timezone prerequisite is unavailable", metric, raw)
		}
	}
}
