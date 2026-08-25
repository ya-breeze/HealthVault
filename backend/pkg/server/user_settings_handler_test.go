package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/server"
	kinmodels "github.com/ya-breeze/kin-core/models"
)

// seedFoodDayCompletion inserts a confirmed-day row directly, bypassing the
// (not-yet-implemented) confirm endpoint — task 1's tests only need a row to
// exist, not the endpoint that would normally create one.
func seedFoodDayCompletion(t *testing.T, st database.Storage, userID, familyID uuid.UUID, localDate string) {
	t.Helper()
	row := database.FoodDayCompletion{
		UserID:      userID,
		LocalDate:   localDate,
		ConfirmedAt: time.Now().UTC(),
	}
	row.ID = uuid.New()
	row.FamilyID = familyID
	if err := st.DB().Create(&row).Error; err != nil {
		t.Fatalf("seed FoodDayCompletion: %v", err)
	}
}

func countFoodDayCompletions(t *testing.T, st database.Storage, userID uuid.UUID) int64 {
	t.Helper()
	var count int64
	if err := st.DB().Model(&database.FoodDayCompletion{}).
		Where("user_id = ?", userID).Count(&count).Error; err != nil {
		t.Fatalf("count FoodDayCompletion: %v", err)
	}
	return count
}

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

// display_language is just another key in the opaque settings blob — no
// schema change needed for it to round-trip. See
// openspec/specs/display-language "Per-User Display Language Setting".
func TestUserSettings_DisplayLanguageRoundTrips(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	putH := server.PutUserSettingsHandler(st)
	body := `{"display_language":"ru"}`
	w := httptest.NewRecorder()
	putH.ServeHTTP(w, withClaims(httptest.NewRequest(http.MethodPut, "/api/users/me/settings", bytes.NewBufferString(body)), userID))
	if w.Code != http.StatusOK {
		t.Fatalf("PUT: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	getH := server.GetUserSettingsHandler(st)
	w2 := httptest.NewRecorder()
	getH.ServeHTTP(w2, withClaims(httptest.NewRequest(http.MethodGet, "/api/users/me/settings", nil), userID))
	var got map[string]any
	if err := json.Unmarshal(w2.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["display_language"] != "ru" {
		t.Errorf("display_language = %v, want ru", got["display_language"])
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

// The following tests cover task 1.6/1.7: PUT /api/users/me/settings clears
// a user's FoodDayCompletion rows when their stored "timezone" changes. See
// design.md §4 "Storage" under openspec/changes/food-day-completeness.

func TestUserSettings_UnchangedTimezoneLeavesConfirmationsIntact(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	putH := server.PutUserSettingsHandler(st)
	w := httptest.NewRecorder()
	putH.ServeHTTP(w, withClaims(httptest.NewRequest(http.MethodPut, "/api/users/me/settings",
		bytes.NewBufferString(`{"timezone":"Europe/Warsaw"}`)), userID))
	if w.Code != http.StatusOK {
		t.Fatalf("initial PUT: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	seedFoodDayCompletion(t, st, userID, familyID, "2026-08-20")

	w2 := httptest.NewRecorder()
	putH.ServeHTTP(w2, withClaims(httptest.NewRequest(http.MethodPut, "/api/users/me/settings",
		bytes.NewBufferString(`{"timezone":"Europe/Warsaw","dashboard_order":["weight"]}`)), userID))
	if w2.Code != http.StatusOK {
		t.Fatalf("second PUT: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	if got := countFoodDayCompletions(t, st, userID); got != 1 {
		t.Errorf("expected confirmation to survive an unchanged timezone, found %d rows", got)
	}
}

func TestUserSettings_ChangedTimezoneDeletesOnlyCallersConfirmations(t *testing.T) {
	st := newFoodTestStorage(t)
	userA, familyID := seedFoodUser(t, st)
	userB := uuid.New()
	if err := st.DB().Create(&kinmodels.User{ID: userB, Username: "userB", PasswordHash: "x", FamilyID: familyID}).Error; err != nil {
		t.Fatalf("create second user: %v", err)
	}

	putH := server.PutUserSettingsHandler(st)
	for _, u := range []uuid.UUID{userA, userB} {
		w := httptest.NewRecorder()
		putH.ServeHTTP(w, withClaims(httptest.NewRequest(http.MethodPut, "/api/users/me/settings",
			bytes.NewBufferString(`{"timezone":"Europe/Warsaw"}`)), u))
		if w.Code != http.StatusOK {
			t.Fatalf("initial PUT for %v: expected 200, got %d: %s", u, w.Code, w.Body.String())
		}
	}
	seedFoodDayCompletion(t, st, userA, familyID, "2026-08-20")
	seedFoodDayCompletion(t, st, userB, familyID, "2026-08-20")

	w := httptest.NewRecorder()
	putH.ServeHTTP(w, withClaims(httptest.NewRequest(http.MethodPut, "/api/users/me/settings",
		bytes.NewBufferString(`{"timezone":"America/Los_Angeles"}`)), userA))
	if w.Code != http.StatusOK {
		t.Fatalf("timezone-change PUT: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if got := countFoodDayCompletions(t, st, userA); got != 0 {
		t.Errorf("expected userA's confirmations cleared, found %d rows", got)
	}
	if got := countFoodDayCompletions(t, st, userB); got != 1 {
		t.Errorf("expected userB's confirmation untouched, found %d rows", got)
	}
}

func TestUserSettings_FirstPutWithNoTimezoneKeyLeavesConfirmationsIntact(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	// No settings row exists yet, so GetUserSettings returns
	// gorm.ErrRecordNotFound and the "previously stored" timezone is treated
	// as absent (""). A confirmation row existing despite no settings ever
	// having been saved isn't reachable in practice, but exercises that the
	// comparison against a never-saved document doesn't panic or
	// misclassify "no key present" as a change.
	seedFoodDayCompletion(t, st, userID, familyID, "2026-08-20")

	putH := server.PutUserSettingsHandler(st)
	w := httptest.NewRecorder()
	putH.ServeHTTP(w, withClaims(httptest.NewRequest(http.MethodPut, "/api/users/me/settings",
		bytes.NewBufferString(`{"dashboard_order":["weight"]}`)), userID))
	if w.Code != http.StatusOK {
		t.Fatalf("PUT: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if got := countFoodDayCompletions(t, st, userID); got != 1 {
		t.Errorf("expected confirmation to survive a PUT with no timezone key, found %d rows", got)
	}
}

// Guards against the CustomFood-style soft-delete trap: after a timezone
// change hard-deletes a confirmation, re-confirming the same date string
// (simulated here by directly inserting a row for it, since the confirm
// endpoint doesn't exist yet) must not hit the unique index on a
// soft-deleted row.
func TestUserSettings_ChangedTimezoneAllowsReconfirmingSameDateAfterwards(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	putH := server.PutUserSettingsHandler(st)
	w := httptest.NewRecorder()
	putH.ServeHTTP(w, withClaims(httptest.NewRequest(http.MethodPut, "/api/users/me/settings",
		bytes.NewBufferString(`{"timezone":"Europe/Warsaw"}`)), userID))
	if w.Code != http.StatusOK {
		t.Fatalf("initial PUT: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	seedFoodDayCompletion(t, st, userID, familyID, "2026-08-20")

	w2 := httptest.NewRecorder()
	putH.ServeHTTP(w2, withClaims(httptest.NewRequest(http.MethodPut, "/api/users/me/settings",
		bytes.NewBufferString(`{"timezone":"America/Los_Angeles"}`)), userID))
	if w2.Code != http.StatusOK {
		t.Fatalf("timezone-change PUT: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	// If the cascade delete had used a plain (soft) Delete, this insert
	// would violate the (user_id, local_date) unique index.
	seedFoodDayCompletion(t, st, userID, familyID, "2026-08-20")

	if got := countFoodDayCompletions(t, st, userID); got != 1 {
		t.Errorf("expected exactly one live confirmation after re-confirming, found %d rows", got)
	}
}
