package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ya-breeze/kin-core/auth"
	kinmodels "github.com/ya-breeze/kin-core/models"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/server"
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
	"speed", "food_meal",
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

	if !presence["steps"] {
		t.Errorf("expected steps = true, got %v", presence["steps"])
	}
	if presence["weight"] {
		t.Errorf("expected weight = false, got %v", presence["weight"])
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
