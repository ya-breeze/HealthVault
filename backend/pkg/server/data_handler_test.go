package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/server"
)

func newDataRequest(typeName, rawQuery string, userID uuid.UUID) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/data/"+typeName+"?"+rawQuery, nil)
	r = mux.SetURLVars(r, map[string]string{"type": typeName})
	return withClaims(r, userID)
}

func TestDataHandler_BucketDayReturnsAggregatedRows(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	ts := time.Date(2026, time.March, 15, 8, 0, 0, 0, time.UTC)
	rec := database.Steps{UserID: userID, SourcePayloadID: uuid.New(), StartTime: ts, EndTime: ts, Count: 1000}
	rec.ID = uuid.New()
	rec.FamilyID = familyID
	if err := st.DB().Create(&rec).Error; err != nil {
		t.Fatalf("create steps: %v", err)
	}

	h := server.DataHandler(st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, newDataRequest("steps", "bucket=day&from=2026-03-14T00:00:00Z&to=2026-03-16T00:00:00Z", userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 bucket, got %d: %+v", len(rows), rows)
	}
	if rows[0]["bucket_start"] != "2026-03-15T00:00:00Z" {
		t.Errorf("bucket_start = %v, want 2026-03-15T00:00:00Z", rows[0]["bucket_start"])
	}
	if _, ok := rows[0]["sum"]; !ok {
		t.Errorf("expected a sum field in the aggregated response, got %+v", rows[0])
	}
}

func TestDataHandler_OmittingBucketReturnsRawRecords(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	ts := time.Now()
	rec := database.Steps{UserID: userID, SourcePayloadID: uuid.New(), StartTime: ts, EndTime: ts, Count: 1000}
	rec.ID = uuid.New()
	rec.FamilyID = familyID
	if err := st.DB().Create(&rec).Error; err != nil {
		t.Fatalf("create steps: %v", err)
	}

	h := server.DataHandler(st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, newDataRequest("steps", "", userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 raw record, got %d: %+v", len(rows), rows)
	}
	if _, ok := rows[0]["bucket_start"]; ok {
		t.Errorf("raw response should not contain bucket_start, got %+v", rows[0])
	}
	if _, ok := rows[0]["count"]; !ok {
		t.Errorf("expected the raw steps column 'count', got %+v", rows[0])
	}
}

func TestDataHandler_InvalidBucketValueReturns400(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	for _, bad := range []string{"week", "hour", "banana"} {
		h := server.DataHandler(st)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, newDataRequest("steps", "bucket="+bad, userID))
		if w.Code != http.StatusBadRequest {
			t.Errorf("bucket=%s: expected 400, got %d: %s", bad, w.Code, w.Body.String())
		}
	}
}

func TestDataHandler_FoodMealWithBucketReturns400(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	h := server.DataHandler(st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, newDataRequest("food_meal", "bucket=day", userID))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDataHandler_BucketedBloodPressureReturnsDualColumns(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	ts := time.Date(2026, time.March, 15, 8, 0, 0, 0, time.UTC)
	rec := database.BloodPressure{UserID: userID, SourcePayloadID: uuid.New(), Time: ts, Systolic: 120, Diastolic: 80}
	rec.ID = uuid.New()
	rec.FamilyID = familyID
	if err := st.DB().Create(&rec).Error; err != nil {
		t.Fatalf("create blood_pressure: %v", err)
	}

	h := server.DataHandler(st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, newDataRequest("blood_pressure", "bucket=day&from=2026-03-14T00:00:00Z&to=2026-03-16T00:00:00Z", userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 bucket, got %d: %+v", len(rows), rows)
	}
	for _, field := range []string{"systolic_avg", "systolic_min", "systolic_max", "diastolic_avg", "diastolic_min", "diastolic_max"} {
		if _, ok := rows[0][field]; !ok {
			t.Errorf("expected field %q in blood_pressure bucket, got %+v", field, rows[0])
		}
	}
}

func TestDataHandler_UnknownTypeReturns404RegardlessOfBucket(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	h := server.DataHandler(st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, newDataRequest("not_a_real_type", "bucket=day", userID))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}
