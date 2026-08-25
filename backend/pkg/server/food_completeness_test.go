package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/server"
)

func confirmRequest(method, date string) *http.Request {
	url := "/api/food/completeness/" + date + "/confirm"
	r := httptest.NewRequest(method, url, nil)
	return mux.SetURLVars(r, map[string]string{"date": date})
}

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

func TestConfirmDay_EligibleUnconfirmedDay(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	yesterday := time.Now().UTC().AddDate(0, 0, -1).Truncate(24 * time.Hour)
	createMealAt(t, st, userID, familyID, database.MealStatusConfirmed, yesterday.Add(12*time.Hour))
	dateStr := yesterday.Format("2006-01-02")

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.ConfirmDay(w, withClaims(confirmRequest(http.MethodPost, dateStr), userID))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var row database.FoodDayCompletion
	if err := json.NewDecoder(w.Body).Decode(&row); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if row.LocalDate != dateStr || row.UserID != userID {
		t.Errorf("unexpected confirmation row: %+v", row)
	}
}

func TestConfirmDay_ReconfirmAlreadyConfirmedIsIdempotent(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	yesterday := time.Now().UTC().AddDate(0, 0, -1).Truncate(24 * time.Hour)
	createMealAt(t, st, userID, familyID, database.MealStatusConfirmed, yesterday.Add(12*time.Hour))
	dateStr := yesterday.Format("2006-01-02")

	h := server.NewFoodHandlers(st, nil, t.TempDir())

	w1 := httptest.NewRecorder()
	h.ConfirmDay(w1, withClaims(confirmRequest(http.MethodPost, dateStr), userID))
	if w1.Code != http.StatusCreated {
		t.Fatalf("first confirm: expected 201, got %d: %s", w1.Code, w1.Body.String())
	}

	w2 := httptest.NewRecorder()
	h.ConfirmDay(w2, withClaims(confirmRequest(http.MethodPost, dateStr), userID))
	if w2.Code != http.StatusOK {
		t.Fatalf("re-confirm: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var count int64
	if err := st.DB().Model(&database.FoodDayCompletion{}).
		Where("user_id = ? AND local_date = ?", userID, dateStr).
		Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 confirmation row, got %d", count)
	}
}

func TestConfirmDay_ZeroOccasionDayRejected(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	dateStr := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.ConfirmDay(w, withClaims(confirmRequest(http.MethodPost, dateStr), userID))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 (zero occasions), got %d: %s", w.Code, w.Body.String())
	}
}

func TestConfirmDay_AlreadyCompleteDayRejected(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	yesterday := time.Now().UTC().AddDate(0, 0, -1).Truncate(24 * time.Hour)
	createMealAt(t, st, userID, familyID, database.MealStatusConfirmed, yesterday.Add(8*time.Hour))
	createMealAt(t, st, userID, familyID, database.MealStatusConfirmed, yesterday.Add(13*time.Hour))
	createMealAt(t, st, userID, familyID, database.MealStatusConfirmed, yesterday.Add(19*time.Hour))
	dateStr := yesterday.Format("2006-01-02")

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.ConfirmDay(w, withClaims(confirmRequest(http.MethodPost, dateStr), userID))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 (already complete), got %d: %s", w.Code, w.Body.String())
	}
}

func TestConfirmUnconfirmDay_TodayOrFutureRejected(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	h := server.NewFoodHandlers(st, nil, t.TempDir())

	today := time.Now().UTC().Format("2006-01-02")
	future := time.Now().UTC().AddDate(0, 0, 5).Format("2006-01-02")

	for _, dateStr := range []string{today, future, "not-a-date"} {
		w := httptest.NewRecorder()
		h.ConfirmDay(w, withClaims(confirmRequest(http.MethodPost, dateStr), userID))
		if w.Code != http.StatusBadRequest {
			t.Errorf("confirm %s: expected 400, got %d: %s", dateStr, w.Code, w.Body.String())
		}

		w2 := httptest.NewRecorder()
		h.UnconfirmDay(w2, withClaims(confirmRequest(http.MethodDelete, dateStr), userID))
		if w2.Code != http.StatusBadRequest {
			t.Errorf("unconfirm %s: expected 400, got %d: %s", dateStr, w2.Code, w2.Body.String())
		}
	}
}

func TestUnconfirmDay_DeletesExistingConfirmation(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	yesterday := time.Now().UTC().AddDate(0, 0, -1).Truncate(24 * time.Hour)
	createMealAt(t, st, userID, familyID, database.MealStatusConfirmed, yesterday.Add(12*time.Hour))
	dateStr := yesterday.Format("2006-01-02")

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	wc := httptest.NewRecorder()
	h.ConfirmDay(wc, withClaims(confirmRequest(http.MethodPost, dateStr), userID))
	if wc.Code != http.StatusCreated {
		t.Fatalf("confirm: expected 201, got %d: %s", wc.Code, wc.Body.String())
	}

	wd := httptest.NewRecorder()
	h.UnconfirmDay(wd, withClaims(confirmRequest(http.MethodDelete, dateStr), userID))
	if wd.Code != http.StatusNoContent {
		t.Fatalf("unconfirm: expected 204, got %d: %s", wd.Code, wd.Body.String())
	}

	var count int64
	if err := st.DB().Model(&database.FoodDayCompletion{}).
		Where("user_id = ? AND local_date = ?", userID, dateStr).
		Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected confirmation row gone, got count %d", count)
	}

	// State reverts to unconfirmed now that the confirmation is gone.
	wg := httptest.NewRecorder()
	h.GetCompleteness(wg, withClaims(completenessRequest(fmt.Sprintf("from=%s&to=%s", dateStr, dateStr)), userID))
	var got []database.DayCompleteness
	if err := json.NewDecoder(wg.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].State != database.DayStateUnconfirmed {
		t.Errorf("expected reverted state unconfirmed, got %+v", got)
	}
}

func TestUnconfirmDay_NonExistentConfirmationNoError(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	dateStr := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.UnconfirmDay(w, withClaims(confirmRequest(http.MethodDelete, dateStr), userID))

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestConfirmUnconfirmDay_Unauthorized(t *testing.T) {
	st := newFoodTestStorage(t)
	h := server.NewFoodHandlers(st, nil, t.TempDir())
	dateStr := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")

	wc := httptest.NewRecorder()
	h.ConfirmDay(wc, confirmRequest(http.MethodPost, dateStr))
	if wc.Code != http.StatusUnauthorized {
		t.Errorf("confirm: expected 401, got %d: %s", wc.Code, wc.Body.String())
	}

	wd := httptest.NewRecorder()
	h.UnconfirmDay(wd, confirmRequest(http.MethodDelete, dateStr))
	if wd.Code != http.StatusUnauthorized {
		t.Errorf("unconfirm: expected 401, got %d: %s", wd.Code, wd.Body.String())
	}
}

func TestConfirmDay_NoUserOverride(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	otherUserID := uuid.New()

	yesterday := time.Now().UTC().AddDate(0, 0, -1).Truncate(24 * time.Hour)
	// Give the *other* user 3 occasions (would be Complete, i.e. rejected)
	// so that if the handler ever honored a ?user= override, confirming
	// would fail with 400 instead of the caller's own eligible 201.
	createMealAt(t, st, otherUserID, familyID, database.MealStatusConfirmed, yesterday.Add(8*time.Hour))
	createMealAt(t, st, otherUserID, familyID, database.MealStatusConfirmed, yesterday.Add(13*time.Hour))
	createMealAt(t, st, otherUserID, familyID, database.MealStatusConfirmed, yesterday.Add(19*time.Hour))
	// The caller has exactly 1 occasion — eligible for confirmation.
	createMealAt(t, st, userID, familyID, database.MealStatusConfirmed, yesterday.Add(12*time.Hour))
	dateStr := yesterday.Format("2006-01-02")

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	r := confirmRequest(http.MethodPost, dateStr)
	r.URL.RawQuery = "user=" + otherUserID.String()
	w := httptest.NewRecorder()
	h.ConfirmDay(w, withClaims(r, userID))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 using caller's own data, got %d: %s", w.Code, w.Body.String())
	}

	var count int64
	if err := st.DB().Model(&database.FoodDayCompletion{}).
		Where("user_id = ? AND local_date = ?", otherUserID, dateStr).
		Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no confirmation row created for the other user, got count %d", count)
	}
}

func TestConfirmDay_ConfirmRetractConfirmAgainSucceeds(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	yesterday := time.Now().UTC().AddDate(0, 0, -1).Truncate(24 * time.Hour)
	createMealAt(t, st, userID, familyID, database.MealStatusConfirmed, yesterday.Add(12*time.Hour))
	dateStr := yesterday.Format("2006-01-02")

	h := server.NewFoodHandlers(st, nil, t.TempDir())

	w1 := httptest.NewRecorder()
	h.ConfirmDay(w1, withClaims(confirmRequest(http.MethodPost, dateStr), userID))
	if w1.Code != http.StatusCreated {
		t.Fatalf("confirm: expected 201, got %d: %s", w1.Code, w1.Body.String())
	}

	w2 := httptest.NewRecorder()
	h.UnconfirmDay(w2, withClaims(confirmRequest(http.MethodDelete, dateStr), userID))
	if w2.Code != http.StatusNoContent {
		t.Fatalf("retract: expected 204, got %d: %s", w2.Code, w2.Body.String())
	}

	w3 := httptest.NewRecorder()
	h.ConfirmDay(w3, withClaims(confirmRequest(http.MethodPost, dateStr), userID))
	if w3.Code != http.StatusCreated {
		t.Fatalf("re-confirm after retract: expected 201 (not a unique-constraint violation), got %d: %s",
			w3.Code, w3.Body.String())
	}
}

// TestConfirmDay_DistinctDatesGetDistinctIDs guards against a regression
// where ConfirmDay built the FoodDayCompletion row without ever setting its
// primary key: GORM then inserted the zero UUID for every confirmation, so
// only the first confirmation in the whole table ever succeeded and every
// later distinct confirmation 500'd on the primary-key unique constraint.
func TestConfirmDay_DistinctDatesGetDistinctIDs(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	day1 := time.Now().UTC().AddDate(0, 0, -2).Truncate(24 * time.Hour)
	day2 := time.Now().UTC().AddDate(0, 0, -1).Truncate(24 * time.Hour)
	createMealAt(t, st, userID, familyID, database.MealStatusConfirmed, day1.Add(12*time.Hour))
	createMealAt(t, st, userID, familyID, database.MealStatusConfirmed, day2.Add(12*time.Hour))
	date1Str := day1.Format("2006-01-02")
	date2Str := day2.Format("2006-01-02")

	h := server.NewFoodHandlers(st, nil, t.TempDir())

	w1 := httptest.NewRecorder()
	h.ConfirmDay(w1, withClaims(confirmRequest(http.MethodPost, date1Str), userID))
	if w1.Code != http.StatusCreated {
		t.Fatalf("confirm day1: expected 201, got %d: %s", w1.Code, w1.Body.String())
	}
	var row1 database.FoodDayCompletion
	if err := json.NewDecoder(w1.Body).Decode(&row1); err != nil {
		t.Fatalf("decode row1: %v", err)
	}

	w2 := httptest.NewRecorder()
	h.ConfirmDay(w2, withClaims(confirmRequest(http.MethodPost, date2Str), userID))
	if w2.Code != http.StatusCreated {
		t.Fatalf("confirm day2: expected 201, got %d: %s", w2.Code, w2.Body.String())
	}
	var row2 database.FoodDayCompletion
	if err := json.NewDecoder(w2.Body).Decode(&row2); err != nil {
		t.Fatalf("decode row2: %v", err)
	}

	if row1.ID == uuid.Nil || row2.ID == uuid.Nil {
		t.Fatalf("expected non-nil IDs, got row1=%s row2=%s", row1.ID, row2.ID)
	}
	if row1.ID == row2.ID {
		t.Fatalf("expected distinct IDs for distinct confirmations, both got %s", row1.ID)
	}
}
