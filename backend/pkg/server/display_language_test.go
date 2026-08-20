package server_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ya-breeze/healthvault/pkg/server"
)

func TestDisplayLanguage_DefaultsToEnglishWhenNoSettingsSaved(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	if got := server.DisplayLanguage(st, userID); got != "en" {
		t.Errorf("DisplayLanguage = %q, want en", got)
	}
}

func TestDisplayLanguage_DefaultsToEnglishWhenKeyAbsent(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	putH := server.PutUserSettingsHandler(st)
	w := httptest.NewRecorder()
	putH.ServeHTTP(w, withClaims(httptest.NewRequest(http.MethodPut, "/api/users/me/settings", bytes.NewBufferString(`{"dashboard_order":["weight"]}`)), userID))
	if w.Code != http.StatusOK {
		t.Fatalf("PUT: expected 200, got %d", w.Code)
	}

	if got := server.DisplayLanguage(st, userID); got != "en" {
		t.Errorf("DisplayLanguage = %q, want en", got)
	}
}

func TestDisplayLanguage_ReadsSavedValue(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	putH := server.PutUserSettingsHandler(st)
	w := httptest.NewRecorder()
	putH.ServeHTTP(w, withClaims(httptest.NewRequest(http.MethodPut, "/api/users/me/settings", bytes.NewBufferString(`{"display_language":"ru"}`)), userID))
	if w.Code != http.StatusOK {
		t.Fatalf("PUT: expected 200, got %d", w.Code)
	}

	if got := server.DisplayLanguage(st, userID); got != "ru" {
		t.Errorf("DisplayLanguage = %q, want ru", got)
	}
}
