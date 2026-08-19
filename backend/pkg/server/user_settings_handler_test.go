package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/ya-breeze/healthvault/pkg/server"
	kinmodels "github.com/ya-breeze/kin-core/models"
)

func TestUserSettings_GetBeforeAnyPutReturnsEmptyObjectWithoutCreatingRow(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	getH := server.GetUserSettingsHandler(st)
	w := httptest.NewRecorder()
	getH.ServeHTTP(w, withClaims(httptest.NewRequest(http.MethodGet, "/api/users/me/settings", nil), userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected {}, got %v", got)
	}

	var count int64
	st.DB().Table("user_settings").Where("user_id = ?", userID).Count(&count) //nolint:errcheck
	if count != 0 {
		t.Errorf("expected no row created by GET, found %d", count)
	}
}

func TestUserSettings_RoundTripsThroughPutAndGet(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	putH := server.PutUserSettingsHandler(st)
	body := `{"dashboard_order":["weight","steps"]}`
	w := httptest.NewRecorder()
	putH.ServeHTTP(w, withClaims(httptest.NewRequest(http.MethodPut, "/api/users/me/settings", bytes.NewBufferString(body)), userID))
	if w.Code != http.StatusOK {
		t.Fatalf("PUT: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	getH := server.GetUserSettingsHandler(st)
	w2 := httptest.NewRecorder()
	getH.ServeHTTP(w2, withClaims(httptest.NewRequest(http.MethodGet, "/api/users/me/settings", nil), userID))
	if w2.Code != http.StatusOK {
		t.Fatalf("GET: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(w2.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	order, ok := got["dashboard_order"].([]any)
	if !ok || len(order) != 2 || order[0] != "weight" || order[1] != "steps" {
		t.Errorf("dashboard_order = %v, want [weight steps]", got["dashboard_order"])
	}
}

func TestUserSettings_PutOverwritesPreviousValue(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	putH := server.PutUserSettingsHandler(st)
	for _, body := range []string{`{"dashboard_order":["weight"]}`, `{"dashboard_order":["steps"]}`} {
		w := httptest.NewRecorder()
		putH.ServeHTTP(w, withClaims(httptest.NewRequest(http.MethodPut, "/api/users/me/settings", bytes.NewBufferString(body)), userID))
		if w.Code != http.StatusOK {
			t.Fatalf("PUT %q: expected 200, got %d: %s", body, w.Code, w.Body.String())
		}
	}

	getH := server.GetUserSettingsHandler(st)
	w := httptest.NewRecorder()
	getH.ServeHTTP(w, withClaims(httptest.NewRequest(http.MethodGet, "/api/users/me/settings", nil), userID))
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	order, _ := got["dashboard_order"].([]any)
	if len(order) != 1 || order[0] != "steps" {
		t.Errorf("dashboard_order = %v, want [steps]", got["dashboard_order"])
	}
}

func TestUserSettings_IsolatedPerUser(t *testing.T) {
	st := newFoodTestStorage(t)
	userA, familyID := seedFoodUser(t, st)
	userB := uuid.New()
	if err := st.DB().Create(&kinmodels.User{ID: userB, Username: "userB", PasswordHash: "x", FamilyID: familyID}).Error; err != nil {
		t.Fatalf("create second user: %v", err)
	}

	putH := server.PutUserSettingsHandler(st)
	w := httptest.NewRecorder()
	putH.ServeHTTP(w, withClaims(httptest.NewRequest(http.MethodPut, "/api/users/me/settings", bytes.NewBufferString(`{"dashboard_order":["weight"]}`)), userA))
	if w.Code != http.StatusOK {
		t.Fatalf("PUT for userA: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	getH := server.GetUserSettingsHandler(st)
	w2 := httptest.NewRecorder()
	getH.ServeHTTP(w2, withClaims(httptest.NewRequest(http.MethodGet, "/api/users/me/settings", nil), userB))
	var got map[string]any
	if err := json.Unmarshal(w2.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("userB should see no settings (isolated from userA), got %v", got)
	}
}

func TestUserSettings_UnauthenticatedRejected(t *testing.T) {
	st := newFoodTestStorage(t)

	getH := server.GetUserSettingsHandler(st)
	w := httptest.NewRecorder()
	getH.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/users/me/settings", nil))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("GET without claims: expected 401, got %d", w.Code)
	}

	putH := server.PutUserSettingsHandler(st)
	w2 := httptest.NewRecorder()
	putH.ServeHTTP(w2, httptest.NewRequest(http.MethodPut, "/api/users/me/settings", bytes.NewBufferString(`{}`)))
	if w2.Code != http.StatusUnauthorized {
		t.Errorf("PUT without claims: expected 401, got %d", w2.Code)
	}
}

func TestUserSettings_MalformedPutBodyRejectedAndLeavesPriorSettingsUnchanged(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	putH := server.PutUserSettingsHandler(st)
	w := httptest.NewRecorder()
	putH.ServeHTTP(w, withClaims(httptest.NewRequest(http.MethodPut, "/api/users/me/settings", bytes.NewBufferString(`{"dashboard_order":["weight"]}`)), userID))
	if w.Code != http.StatusOK {
		t.Fatalf("initial PUT: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	for _, malformed := range []string{`not json`, `["an","array"]`, ``, `null`} {
		w := httptest.NewRecorder()
		putH.ServeHTTP(w, withClaims(httptest.NewRequest(http.MethodPut, "/api/users/me/settings", bytes.NewBufferString(malformed)), userID))
		if w.Code != http.StatusBadRequest {
			t.Errorf("PUT %q: expected 400, got %d: %s", malformed, w.Code, w.Body.String())
		}
	}

	getH := server.GetUserSettingsHandler(st)
	w2 := httptest.NewRecorder()
	getH.ServeHTTP(w2, withClaims(httptest.NewRequest(http.MethodGet, "/api/users/me/settings", nil), userID))
	var got map[string]any
	if err := json.Unmarshal(w2.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	order, _ := got["dashboard_order"].([]any)
	if len(order) != 1 || order[0] != "weight" {
		t.Errorf("settings should be unchanged after malformed PUTs, got %v", got)
	}
}

func TestUserSettings_OversizedPutBodyRejected(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	huge := `{"dashboard_order":["` + strings.Repeat("x", 70*1024) + `"]}`
	putH := server.PutUserSettingsHandler(st)
	w := httptest.NewRecorder()
	putH.ServeHTTP(w, withClaims(httptest.NewRequest(http.MethodPut, "/api/users/me/settings", bytes.NewBufferString(huge)), userID))
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413, got %d: %s", w.Code, w.Body.String())
	}
}
