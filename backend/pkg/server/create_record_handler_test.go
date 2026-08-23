package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/ya-breeze/healthvault/pkg/server"
	"github.com/ya-breeze/kin-core/auth"
	kinmodels "github.com/ya-breeze/kin-core/models"
)

func newCreateRequest(typeName string, body map[string]any, userID uuid.UUID) *http.Request {
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(http.MethodPost, "/api/data/"+typeName, bytes.NewReader(b))
	r = mux.SetURLVars(r, map[string]string{"type": typeName})
	return withClaims(r, userID)
}

func TestCreateRecordHandler_WeightGoalSuccess(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	h := server.CreateRecordHandler(st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, newCreateRequest("weight_goal", map[string]any{"value": 72.5}, userID))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var row map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &row); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if row["kilograms"] != 72.5 {
		t.Errorf("kilograms = %v, want 72.5", row["kilograms"])
	}
	if row["id"] == nil || row["id"] == "" {
		t.Errorf("expected an id in the created row, got %+v", row)
	}
	if row["time"] == nil {
		t.Errorf("expected a defaulted time in the created row, got %+v", row)
	}
}

func TestCreateRecordHandler_ExplicitTime(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	h := server.CreateRecordHandler(st)
	w := httptest.NewRecorder()
	ts := "2026-01-15T08:00:00Z"
	h.ServeHTTP(w, newCreateRequest("weight", map[string]any{"value": 80, "time": ts}, userID))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var row map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &row); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got, err := time.Parse(time.RFC3339, row["time"].(string))
	if err != nil {
		t.Fatalf("parse time: %v", err)
	}
	if !got.Equal(time.Date(2026, time.January, 15, 8, 0, 0, 0, time.UTC)) {
		t.Errorf("time = %v, want %s", got, ts)
	}
}

func TestCreateRecordHandler_ExplicitTimeNonUTCOffsetNormalized(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	h := server.CreateRecordHandler(st)
	w := httptest.NewRecorder()
	// 08:00:00+05:00 is the same instant as 03:00:00Z.
	h.ServeHTTP(w, newCreateRequest("weight", map[string]any{"value": 80, "time": "2026-01-15T08:00:00+05:00"}, userID))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var row map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &row); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got, err := time.Parse(time.RFC3339, row["time"].(string))
	if err != nil {
		t.Fatalf("parse time: %v", err)
	}
	want := time.Date(2026, time.January, 15, 3, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("time = %v, want %v (normalized to UTC)", got, want)
	}
	if got.Location() != time.UTC {
		t.Errorf("stored time location = %v, want UTC", got.Location())
	}
}

func TestCreateRecordHandler_WritesCallerFamilyID(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	h := server.CreateRecordHandler(st)
	w := httptest.NewRecorder()
	b, _ := json.Marshal(map[string]any{"value": 80})
	r := httptest.NewRequest(http.MethodPost, "/api/data/weight", bytes.NewReader(b))
	r = mux.SetURLVars(r, map[string]string{"type": "weight"})
	claims := &auth.Claims{UserID: userID, FamilyID: &familyID}
	r = r.WithContext(context.WithValue(r.Context(), server.ClaimsContextKey, claims))
	h.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var row map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &row); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if row["family_id"] != familyID.String() {
		t.Errorf("family_id = %v, want %s", row["family_id"], familyID)
	}
}

func TestCreateRecordHandler_DuplicateTimeReturns409(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	h := server.CreateRecordHandler(st)
	ts := "2026-01-15T08:00:00Z"

	w1 := httptest.NewRecorder()
	h.ServeHTTP(w1, newCreateRequest("weight", map[string]any{"value": 80, "time": ts}, userID))
	if w1.Code != http.StatusCreated {
		t.Fatalf("first insert: expected 201, got %d: %s", w1.Code, w1.Body.String())
	}

	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, newCreateRequest("weight", map[string]any{"value": 81, "time": ts}, userID))
	if w2.Code != http.StatusConflict {
		t.Fatalf("second insert: expected 409, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestCreateRecordHandler_NonAllowlistedTypeReturns403(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	h := server.CreateRecordHandler(st)
	for _, typeName := range []string{"steps", "blood_pressure"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, newCreateRequest(typeName, map[string]any{"value": 10}, userID))
		if w.Code != http.StatusForbidden {
			t.Errorf("type=%s: expected 403, got %d: %s", typeName, w.Code, w.Body.String())
		}
	}
}

func TestCreateRecordHandler_UnknownTypeReturns404(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	h := server.CreateRecordHandler(st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, newCreateRequest("not_a_real_type", map[string]any{"value": 10}, userID))
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateRecordHandler_MissingValueReturns400(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	h := server.CreateRecordHandler(st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, newCreateRequest("weight", map[string]any{}, userID))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateRecordHandler_NonPositiveValueReturns400(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	h := server.CreateRecordHandler(st)
	for _, v := range []float64{0, -5} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, newCreateRequest("weight", map[string]any{"value": v}, userID))
		if w.Code != http.StatusBadRequest {
			t.Errorf("value=%v: expected 400, got %d: %s", v, w.Code, w.Body.String())
		}
	}
}

func TestCreateRecordHandler_NonNumericValueReturns400(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	h := server.CreateRecordHandler(st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, newCreateRequest("weight", map[string]any{"value": "abc"}, userID))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateRecordHandler_Unauthenticated(t *testing.T) {
	st := newFoodTestStorage(t)
	h := server.CreateRecordHandler(st)
	w := httptest.NewRecorder()
	b, _ := json.Marshal(map[string]any{"value": 80})
	r := httptest.NewRequest(http.MethodPost, "/api/data/weight", bytes.NewReader(b))
	r = mux.SetURLVars(r, map[string]string{"type": "weight"})
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateRecordHandler_DoesNotHonorUserQueryParam(t *testing.T) {
	// A family member cannot write into another member's account via ?user=:
	// the handler must resolve the target user as claims.UserID directly.
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	otherUserID := uuid.New()
	other := kinmodels.User{ID: otherUserID, Username: "other", PasswordHash: "x", FamilyID: familyID}
	if err := st.DB().Create(&other).Error; err != nil {
		t.Fatalf("create other user: %v", err)
	}

	h := server.CreateRecordHandler(st)
	w := httptest.NewRecorder()
	b, _ := json.Marshal(map[string]any{"value": 80})
	r := httptest.NewRequest(http.MethodPost, "/api/data/weight?user=other", bytes.NewReader(b))
	r = mux.SetURLVars(r, map[string]string{"type": "weight"})
	r = withClaims(r, userID)
	h.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var row map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &row); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if row["user_id"] != userID.String() {
		t.Errorf("expected the record to be scoped to the caller %s despite ?user=other, got user_id=%v", userID, row["user_id"])
	}
}

// A bare "> 0" check is not enough when one generic form serves kilograms and
// metres alike: 178 is a natural thing to type for a height and used to be
// stored as 178 metres, which then rendered BMI as 0.0 with category bands
// drawn around 586,000 kg. Bounds are enforced at the API so no client can
// write the bad value.
func TestCreateRecordHandler_ImplausibleValueReturns400(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	h := server.CreateRecordHandler(st)
	cases := []struct {
		typeName string
		value    float64
	}{
		{"height", 178},       // metres typed as centimetres
		{"height", 0.2},       // implausibly short
		{"weight", 0.5},       // kilograms typed as something else
		{"weight", 900},       // implausibly heavy
		{"weight_goal", 5000}, // fat-fingered goal
	}
	for _, c := range cases {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, newCreateRequest(c.typeName, map[string]any{"value": c.value}, userID))
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s=%v: expected 400, got %d: %s", c.typeName, c.value, w.Code, w.Body.String())
		}
	}
}

func TestCreateRecordHandler_PlausibleBoundaryValuesAccepted(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	h := server.CreateRecordHandler(st)
	cases := []struct {
		typeName string
		value    float64
	}{
		{"height", 1.78},
		{"weight", 91.1},
		{"weight_goal", 80},
	}
	for _, c := range cases {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, newCreateRequest(c.typeName, map[string]any{"value": c.value}, userID))
		if w.Code != http.StatusCreated {
			t.Errorf("%s=%v: expected 201, got %d: %s", c.typeName, c.value, w.Code, w.Body.String())
		}
	}
}
