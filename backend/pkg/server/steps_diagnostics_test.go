package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	kinmodels "github.com/ya-breeze/kin-core/models"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/server"
)

type stepsDiagnosticDayForTest struct {
	BucketStart    string `json:"bucket_start"`
	RawCount       int    `json:"raw_count"`
	RawSum         int    `json:"raw_sum"`
	CollapsedSum   int    `json:"collapsed_sum"`
	DroppedRecords int    `json:"dropped_records"`
	PayloadCount   int    `json:"payload_count"`
	LocalDaySum    int    `json:"local_day_sum"`
}

func newStepsDiagnosticsRequest(rawQuery string, userID uuid.UUID) *http.Request {
	url := "/api/data/steps/diagnostics"
	if rawQuery != "" {
		url += "?" + rawQuery
	}
	return withClaims(httptest.NewRequest(http.MethodGet, url, nil), userID)
}

func decodeStepsDiagnostics(t *testing.T, w *httptest.ResponseRecorder) []stepsDiagnosticDayForTest {
	t.Helper()
	var days []stepsDiagnosticDayForTest
	if err := json.Unmarshal(w.Body.Bytes(), &days); err != nil {
		t.Fatalf("unmarshal: %v (body: %s)", err, w.Body.String())
	}
	return days
}

func TestStepsDiagnosticsHandler_Unauthenticated(t *testing.T) {
	st := newFoodTestStorage(t)

	h := server.StepsDiagnosticsHandler(st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/data/steps/diagnostics", nil))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestStepsDiagnosticsHandler_OverlapReportsRawAboveCollapsedWithDrops(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	day1 := time.Date(2026, time.March, 15, 8, 0, 0, 0, time.UTC)
	for _, rec := range []database.Steps{
		{UserID: userID, SourcePayloadID: uuid.New(), StartTime: day1, EndTime: day1.Add(2 * time.Hour), Count: 3000},
		// Nested inside the first: a duplicate copy from a second sync source.
		{UserID: userID, SourcePayloadID: uuid.New(), StartTime: day1.Add(time.Minute), EndTime: day1.Add(time.Hour), Count: 2900},
	} {
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		if err := st.DB().Create(&rec).Error; err != nil {
			t.Fatalf("create steps: %v", err)
		}
	}

	h := server.StepsDiagnosticsHandler(st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, newStepsDiagnosticsRequest("from=2026-03-14T00:00:00Z&to=2026-03-16T00:00:00Z", userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	days := decodeStepsDiagnostics(t, w)
	if len(days) != 1 {
		t.Fatalf("expected 1 day, got %d: %+v", len(days), days)
	}
	d := days[0]
	if d.RawCount != 2 {
		t.Errorf("raw_count = %d, want 2", d.RawCount)
	}
	if d.RawSum <= d.CollapsedSum {
		t.Errorf("raw_sum (%d) should be greater than collapsed_sum (%d)", d.RawSum, d.CollapsedSum)
	}
	if d.DroppedRecords == 0 {
		t.Errorf("dropped_records = 0, want nonzero")
	}
}

func TestStepsDiagnosticsHandler_TwoPayloadsReportsPayloadCountTwo(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	day1 := time.Date(2026, time.March, 15, 8, 0, 0, 0, time.UTC)
	for _, rec := range []database.Steps{
		{UserID: userID, SourcePayloadID: uuid.New(), StartTime: day1, EndTime: day1.Add(time.Hour), Count: 1000},
		{UserID: userID, SourcePayloadID: uuid.New(), StartTime: day1.Add(2 * time.Hour), EndTime: day1.Add(3 * time.Hour), Count: 1200},
	} {
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		if err := st.DB().Create(&rec).Error; err != nil {
			t.Fatalf("create steps: %v", err)
		}
	}

	h := server.StepsDiagnosticsHandler(st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, newStepsDiagnosticsRequest("from=2026-03-14T00:00:00Z&to=2026-03-16T00:00:00Z", userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	days := decodeStepsDiagnostics(t, w)
	if len(days) != 1 {
		t.Fatalf("expected 1 day, got %d: %+v", len(days), days)
	}
	if days[0].PayloadCount != 2 {
		t.Errorf("payload_count = %d, want 2", days[0].PayloadCount)
	}
}

// A record close to UTC midnight lands in a different local calendar day
// under a non-UTC stored timezone, so local_day_sum for the UTC bucket
// diverges from collapsed_sum even though nothing was collapsed.
func TestStepsDiagnosticsHandler_NonUTCTimezoneShiftsLocalDaySum(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	setProfile(t, st, userID, `{"birthdate":"1990-01-01","sex":"male","timezone":"America/Los_Angeles"}`)

	// 2026-03-15T02:00:00Z is 2026-03-14T18:00:00-08:00 in America/Los_Angeles
	// — same UTC day, but the previous local day.
	ts := time.Date(2026, time.March, 15, 2, 0, 0, 0, time.UTC)
	rec := database.Steps{UserID: userID, SourcePayloadID: uuid.New(), StartTime: ts, EndTime: ts.Add(time.Hour), Count: 4000}
	rec.ID = uuid.New()
	rec.FamilyID = familyID
	if err := st.DB().Create(&rec).Error; err != nil {
		t.Fatalf("create steps: %v", err)
	}

	h := server.StepsDiagnosticsHandler(st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, newStepsDiagnosticsRequest("from=2026-03-14T00:00:00Z&to=2026-03-16T00:00:00Z", userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	days := decodeStepsDiagnostics(t, w)
	if len(days) != 1 {
		t.Fatalf("expected 1 day, got %d: %+v", len(days), days)
	}
	d := days[0]
	if d.CollapsedSum != 4000 {
		t.Fatalf("collapsed_sum = %d, want 4000", d.CollapsedSum)
	}
	if d.LocalDaySum == d.CollapsedSum {
		t.Errorf("local_day_sum (%d) should differ from collapsed_sum (%d) under a shifted local day boundary",
			d.LocalDaySum, d.CollapsedSum)
	}
}

// Without an explicit ?user=, resolveUser scopes to the caller only: a
// sibling family member's steps must never appear in the caller's own
// diagnostics response.
func TestStepsDiagnosticsHandler_AnotherFamilyMembersDataIsNeverReturned(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	memberID := uuid.New()
	member := kinmodels.User{ID: memberID, Username: "sibling", PasswordHash: "x", FamilyID: familyID}
	if err := st.DB().Create(&member).Error; err != nil {
		t.Fatalf("create family member: %v", err)
	}
	day1 := time.Date(2026, time.March, 15, 8, 0, 0, 0, time.UTC)
	rec := database.Steps{UserID: memberID, SourcePayloadID: uuid.New(), StartTime: day1, EndTime: day1.Add(time.Hour), Count: 5000}
	rec.ID = uuid.New()
	rec.FamilyID = familyID
	if err := st.DB().Create(&rec).Error; err != nil {
		t.Fatalf("create steps for sibling: %v", err)
	}

	h := server.StepsDiagnosticsHandler(st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, newStepsDiagnosticsRequest("from=2026-03-14T00:00:00Z&to=2026-03-16T00:00:00Z", userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	days := decodeStepsDiagnostics(t, w)
	if len(days) != 0 {
		t.Errorf("expected no days (the caller has no steps of their own), got %+v", days)
	}
}
