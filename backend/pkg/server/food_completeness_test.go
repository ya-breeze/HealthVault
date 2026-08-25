package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/server"
)

func completenessRequest(query string) *http.Request {
	url := "/api/food/completeness"
	if query != "" {
		url += "?" + query
	}
	return httptest.NewRequest(http.MethodGet, url, nil)
}

func TestGetCompleteness_HappyPathAcrossSeveralDays(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	// today - 3: 3 occasions (>= default threshold 3) -> complete
	base := time.Now().UTC().Truncate(24 * time.Hour)
	day3 := base.AddDate(0, 0, -3)
	createMealAt(t, st, userID, familyID, database.MealStatusConfirmed, day3.Add(8*time.Hour))
	createMealAt(t, st, userID, familyID, database.MealStatusConfirmed, day3.Add(13*time.Hour))
	createMealAt(t, st, userID, familyID, database.MealStatusConfirmed, day3.Add(19*time.Hour))

	// today - 2: 1 occasion, no confirmation -> unconfirmed
	day2 := base.AddDate(0, 0, -2)
	createMealAt(t, st, userID, familyID, database.MealStatusConfirmed, day2.Add(12*time.Hour))

	// today - 1: zero meals -> incomplete

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	from := base.AddDate(0, 0, -3).Format("2006-01-02")
	to := base.AddDate(0, 0, -1).Format("2006-01-02")

	w := httptest.NewRecorder()
	h.GetCompleteness(w, withClaims(completenessRequest(fmt.Sprintf("from=%s&to=%s", from, to)), userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got []database.DayCompleteness
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 days, got %d: %+v", len(got), got)
	}
	if got[0].State != database.DayStateComplete || got[0].OccasionCount != 3 {
		t.Errorf("day -3: expected complete/3, got %+v", got[0])
	}
	if got[1].State != database.DayStateUnconfirmed || got[1].OccasionCount != 1 {
		t.Errorf("day -2: expected unconfirmed/1, got %+v", got[1])
	}
	if got[2].State != database.DayStateIncomplete || got[2].OccasionCount != 0 {
		t.Errorf("day -1 (zero meals): expected incomplete/0, got %+v", got[2])
	}
}

func TestGetCompleteness_ToClampedToYesterday(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	h := server.NewFoodHandlers(st, nil, t.TempDir())

	today := time.Now().UTC().Format("2006-01-02")
	future := time.Now().UTC().AddDate(0, 0, 5).Format("2006-01-02")
	from := time.Now().UTC().AddDate(0, 0, -2).Format("2006-01-02")

	for _, to := range []string{today, future} {
		w := httptest.NewRecorder()
		h.GetCompleteness(w, withClaims(completenessRequest(fmt.Sprintf("from=%s&to=%s", from, to)), userID))
		if w.Code != http.StatusOK {
			t.Fatalf("to=%s: expected 200, got %d: %s", to, w.Code, w.Body.String())
		}
		var got []database.DayCompleteness
		if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		yesterday := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
		if len(got) == 0 || got[len(got)-1].Date != yesterday {
			t.Errorf("to=%s: expected range clamped so last day is yesterday (%s), got %+v", to, yesterday, got)
		}
	}
}

func TestGetCompleteness_FromTodayInvertsAfterClamp(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	h := server.NewFoodHandlers(st, nil, t.TempDir())

	today := time.Now().UTC().Format("2006-01-02")
	w := httptest.NewRecorder()
	h.GetCompleteness(w, withClaims(completenessRequest(fmt.Sprintf("from=%s&to=%s", today, today)), userID))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 (from inverts to after clamp), got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetCompleteness_BadRequests(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	h := server.NewFoodHandlers(st, nil, t.TempDir())

	yesterday := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	old := time.Now().UTC().AddDate(0, 0, -100).Format("2006-01-02")

	cases := []struct {
		name  string
		query string
	}{
		{"missing from", "to=" + yesterday},
		{"missing to", "from=" + old},
		{"malformed from", "from=not-a-date&to=" + yesterday},
		{"malformed to", "from=" + old + "&to=not-a-date"},
		{"from after to", fmt.Sprintf("from=%s&to=%s", yesterday, old)},
		{"span exceeds 92 days", fmt.Sprintf("from=%s&to=%s", old, yesterday)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			h.GetCompleteness(w, withClaims(completenessRequest(tc.query), userID))
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestGetCompleteness_Unauthorized(t *testing.T) {
	st := newFoodTestStorage(t)
	h := server.NewFoodHandlers(st, nil, t.TempDir())

	yesterday := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	w := httptest.NewRecorder()
	h.GetCompleteness(w, completenessRequest("from="+yesterday+"&to="+yesterday))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetCompleteness_NoUserOverride(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	otherUserID := uuid.New()

	day := time.Now().UTC().AddDate(0, 0, -2).Truncate(24 * time.Hour)
	createMealAt(t, st, otherUserID, familyID, database.MealStatusConfirmed, day.Add(8*time.Hour))
	createMealAt(t, st, otherUserID, familyID, database.MealStatusConfirmed, day.Add(9*time.Hour))
	createMealAt(t, st, otherUserID, familyID, database.MealStatusConfirmed, day.Add(10*time.Hour))

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	from := day.Format("2006-01-02")
	to := from

	w := httptest.NewRecorder()
	req := completenessRequest(fmt.Sprintf("from=%s&to=%s&user=%s", from, to, otherUserID))
	h.GetCompleteness(w, withClaims(req, userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got []database.DayCompleteness
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].State != database.DayStateIncomplete {
		t.Errorf("expected caller's own (empty) day, not the other user's meals: %+v", got)
	}
}

func TestGetCompleteness_UsesStoredTimezoneAndThreshold(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	if err := st.UpsertUserSettings(
		userID, familyID, `{"timezone":"America/Los_Angeles","usual_meals_per_day":2}`,
	); err != nil {
		t.Fatalf("UpsertUserSettings: %v", err)
	}

	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	// 2026-08-21T02:00:00Z is 2026-08-20 19:00 in America/Los_Angeles.
	loggedAt := time.Date(2026, 8, 21, 2, 0, 0, 0, time.UTC)
	createMealAt(t, st, userID, familyID, database.MealStatusConfirmed, loggedAt)

	// Give it a second occasion within the same LA-local day so 2 occasions
	// meets the threshold of 2.
	createMealAt(t, st, userID, familyID, database.MealStatusConfirmed, loggedAt.Add(2*time.Hour))

	localDate := loggedAt.In(loc).Format("2006-01-02")

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	req := completenessRequest(fmt.Sprintf("from=%s&to=%s", localDate, localDate))
	w := httptest.NewRecorder()
	h.GetCompleteness(w, withClaims(req, userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got []database.DayCompleteness
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].Date != localDate {
		t.Fatalf("expected single day %s, got %+v", localDate, got)
	}
	if got[0].State != database.DayStateComplete || got[0].OccasionCount != 2 {
		t.Errorf("expected complete/2 occasions grouped under the LA-local day, got %+v", got[0])
	}
}

func TestGetCompleteness_UnknownUserSettingsFallsBackToDefaults(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	// No settings row created for userID at all.

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	yesterday := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	w := httptest.NewRecorder()
	h.GetCompleteness(w, withClaims(completenessRequest("from="+yesterday+"&to="+yesterday), userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (fail-open with default settings), got %d: %s", w.Code, w.Body.String())
	}
}
