package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/server"
	"github.com/ya-breeze/kin-core/auth"
	kinmodels "github.com/ya-breeze/kin-core/models"
	"gorm.io/gorm"
)

// presenceTypeNames mirrors typeRegistry's keys in api.go. Kept here (rather
// than exporting the registry) so this test documents, independently of the
// production code, exactly which types a presence response must cover.
var presenceTypeNames = []string{
	"steps", "heart_rate", "heart_rate_variability", "sleep", "distance",
	"active_calories", "total_calories", "weight", "height", "blood_pressure",
	"blood_glucose", "oxygen_saturation", "body_temperature", "skin_temperature",
	"respiratory_rate", "resting_heart_rate", "exercise", "hydration", "nutrition",
	"basal_metabolic_rate", "body_fat", "lean_body_mass", "vo2_max", "bone_mass",
	"speed", "food_meal", "weight_goal",
}

func newPresenceRequest(rawQuery string) *http.Request {
	url := "/api/data-types/presence"
	if rawQuery != "" {
		url += "?" + rawQuery
	}
	return httptest.NewRequest(http.MethodGet, url, nil)
}

// withClaimsFamily is withClaims plus a FamilyID, needed to exercise the
// ?user= family-member resolution path that resolveUser supports.
func withClaimsFamily(r *http.Request, userID, familyID uuid.UUID) *http.Request {
	claims := &auth.Claims{UserID: userID, FamilyID: &familyID}
	return r.WithContext(context.WithValue(r.Context(), server.ClaimsContextKey, claims))
}

func TestDataTypesPresenceHandler_UnauthenticatedReturns401(t *testing.T) {
	st := newFoodTestStorage(t)

	h := server.DataTypesPresenceHandler(st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, newPresenceRequest(""))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDataTypesPresenceHandler_ReportsPresenceAndAbsence(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	ts := time.Now()
	rec := database.Steps{UserID: userID, SourcePayloadID: uuid.New(), StartTime: ts, EndTime: ts, Count: 100}
	rec.ID = uuid.New()
	rec.FamilyID = familyID
	if err := st.DB().Create(&rec).Error; err != nil {
		t.Fatalf("create steps: %v", err)
	}

	h := server.DataTypesPresenceHandler(st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, withClaims(newPresenceRequest(""), userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var presence map[string]bool
	if err := json.Unmarshal(w.Body.Bytes(), &presence); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, name := range presenceTypeNames {
		want := name == "steps"
		if presence[name] != want {
			t.Errorf("presence[%q] = %v, want %v", name, presence[name], want)
		}
	}
}

func TestDataTypesPresenceHandler_OneEntryPerRegisteredType(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	h := server.DataTypesPresenceHandler(st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, withClaims(newPresenceRequest(""), userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var presence map[string]bool
	if err := json.Unmarshal(w.Body.Bytes(), &presence); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(presence) != len(presenceTypeNames) {
		t.Fatalf("expected %d entries, got %d: %+v", len(presenceTypeNames), len(presence), presence)
	}
	for _, name := range presenceTypeNames {
		if _, ok := presence[name]; !ok {
			t.Errorf("missing entry for registered type %q", name)
		}
	}
}

func TestDataTypesPresenceHandler_EmptyAccountReportsEveryTypeAbsent(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	h := server.DataTypesPresenceHandler(st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, withClaims(newPresenceRequest(""), userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var presence map[string]bool
	if err := json.Unmarshal(w.Body.Bytes(), &presence); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(presence) != len(presenceTypeNames) {
		t.Fatalf("expected %d entries, got %d: %+v", len(presenceTypeNames), len(presence), presence)
	}
	for _, name := range presenceTypeNames {
		if presence[name] {
			t.Errorf("empty account reported presence[%q] = true", name)
		}
	}
}

func TestDataTypesPresenceHandler_MultipleRowsAcrossTypesRemainPresent(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	for i := range 3 {
		ts := time.Date(2026, time.January, 1+i, 8, 0, 0, 0, time.UTC)
		steps := database.Steps{
			UserID: userID, SourcePayloadID: uuid.New(), StartTime: ts,
			EndTime: ts.Add(time.Hour), Count: 100 + i,
		}
		steps.ID, steps.FamilyID = uuid.New(), familyID
		if err := st.DB().Create(&steps).Error; err != nil {
			t.Fatalf("create steps row %d: %v", i, err)
		}

		weight := database.Weight{
			UserID: userID, SourcePayloadID: ptrUUIDForPresence(uuid.New()),
			Time: ts, Kilograms: 80 + float64(i),
		}
		weight.ID, weight.FamilyID = uuid.New(), familyID
		if err := st.DB().Create(&weight).Error; err != nil {
			t.Fatalf("create weight row %d: %v", i, err)
		}
	}

	h := server.DataTypesPresenceHandler(st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, withClaims(newPresenceRequest(""), userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var presence map[string]bool
	if err := json.Unmarshal(w.Body.Bytes(), &presence); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(presence) != len(presenceTypeNames) {
		t.Fatalf("expected %d entries, got %d: %+v", len(presenceTypeNames), len(presence), presence)
	}
	for _, name := range presenceTypeNames {
		want := name == "steps" || name == "weight"
		if presence[name] != want {
			t.Errorf("presence[%q] = %v, want %v", name, presence[name], want)
		}
	}
}

func ptrUUIDForPresence(value uuid.UUID) *uuid.UUID { return &value }

func TestDataTypesPresenceHandler_FamilyMemberResolutionViaUserParam(t *testing.T) {
	st := newFoodTestStorage(t)
	callerID, familyID := seedFoodUser(t, st)

	memberID := uuid.New()
	member := kinmodels.User{ID: memberID, Username: "member", PasswordHash: "x", FamilyID: familyID}
	if err := st.DB().Create(&member).Error; err != nil {
		t.Fatalf("create family member: %v", err)
	}
	ts := time.Now()
	rec := database.Steps{UserID: memberID, SourcePayloadID: uuid.New(), StartTime: ts, EndTime: ts, Count: 100}
	rec.ID = uuid.New()
	rec.FamilyID = familyID
	if err := st.DB().Create(&rec).Error; err != nil {
		t.Fatalf("create steps for member: %v", err)
	}

	h := server.DataTypesPresenceHandler(st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, withClaimsFamily(newPresenceRequest("user=member"), callerID, familyID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var presence map[string]bool
	if err := json.Unmarshal(w.Body.Bytes(), &presence); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !presence["steps"] {
		t.Errorf("expected steps = true for family member, got %v", presence["steps"])
	}
}

func TestDataTypesPresenceHandler_UserParamOutsideFamilyForbidden(t *testing.T) {
	st := newFoodTestStorage(t)
	callerID, familyID := seedFoodUser(t, st)

	otherFamilyID := uuid.New()
	if err := st.DB().Create(&kinmodels.Family{ID: otherFamilyID, Name: "OtherFamily"}).Error; err != nil {
		t.Fatalf("create other family: %v", err)
	}
	outsiderID := uuid.New()
	outsider := kinmodels.User{ID: outsiderID, Username: "outsider", PasswordHash: "x", FamilyID: otherFamilyID}
	if err := st.DB().Create(&outsider).Error; err != nil {
		t.Fatalf("create outsider: %v", err)
	}

	h := server.DataTypesPresenceHandler(st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, withClaimsFamily(newPresenceRequest("user=outsider"), callerID, familyID))

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

// Regression guard: a combined presence query error must surface as a 500,
// not be swallowed into a partial/incorrect 200 presence map.
func TestDataTypesPresenceHandler_QueryErrorReturns500(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	const hookName = "test:presence-query-error"
	st.DB().Callback().Row().Before("gorm:row").Register(hookName, func(tx *gorm.DB) {
		// Raw(...).Scan uses GORM's row callback. The SQL is already built at
		// this point, so matching a registry-owned arm proves this hook poisons
		// the combined Presence statement, not resolveUser's user lookup.
		if strings.Contains(tx.Statement.SQL.String(), "FROM steps") {
			tx.Error = errors.New("simulated combined presence query failure")
		}
	})
	t.Cleanup(func() { st.DB().Callback().Row().Remove(hookName) })

	h := server.DataTypesPresenceHandler(st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, withClaims(newPresenceRequest(""), userID))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when the combined presence query errors, got %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "{") || strings.Contains(w.Body.String(), "steps") {
		t.Fatalf("expected no partial JSON response, got %q", w.Body.String())
	}
}
