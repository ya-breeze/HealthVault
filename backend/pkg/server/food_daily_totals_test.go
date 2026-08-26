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

func dailyTotalsRequest(query string) *http.Request {
	url := "/api/food/daily-totals"
	if query != "" {
		url += "?" + query
	}
	return httptest.NewRequest(http.MethodGet, url, nil)
}

func createMealWithCalories(
	t *testing.T, st database.Storage, userID, familyID uuid.UUID, status string, loggedAt time.Time, calories float64,
) database.FoodMeal {
	t.Helper()
	meal := database.FoodMeal{UserID: userID, Status: status, LoggedAt: loggedAt, Name: "Meal", Calories: calories}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}
	return meal
}

func TestGetFoodDailyTotals_HappyPathAcrossSeveralDaysIncludingZeroMealDay(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	base := time.Now().UTC().Truncate(24 * time.Hour)
	day5 := base.AddDate(0, 0, -5)
	createMealWithCalories(t, st, userID, familyID, database.MealStatusConfirmed, day5.Add(8*time.Hour), 600)
	createMealWithCalories(t, st, userID, familyID, database.MealStatusConfirmed, day5.Add(19*time.Hour), 500)

	day3 := base.AddDate(0, 0, -3)
	createMealWithCalories(t, st, userID, familyID, database.MealStatusConfirmed, day3.Add(12*time.Hour), 700)

	// day4 and day1 have no meals at all.

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	from := base.AddDate(0, 0, -5).Format("2006-01-02")
	to := base.AddDate(0, 0, -1).Format("2006-01-02")

	w := httptest.NewRecorder()
	h.GetFoodDailyTotals(w, withClaims(dailyTotalsRequest(fmt.Sprintf("from=%s&to=%s", from, to)), userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got []database.DailyTotal
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("expected 5 days, got %d: %+v", len(got), got)
	}
	if got[0].Calories != 1100 {
		t.Errorf("day -5: expected 1100 calories, got %+v", got[0])
	}
	if got[1].Calories != 0 {
		t.Errorf("day -4 (zero meals): expected 0 calories, got %+v", got[1])
	}
	if got[2].Calories != 700 {
		t.Errorf("day -3: expected 700 calories, got %+v", got[2])
	}
	if got[4].Calories != 0 {
		t.Errorf("day -1 (zero meals): expected 0 calories, got %+v", got[4])
	}
}

func TestGetFoodDailyTotals_UnconfirmedMealsExcludedFromSum(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	day := time.Now().UTC().AddDate(0, 0, -2).Truncate(24 * time.Hour)
	createMealWithCalories(t, st, userID, familyID, database.MealStatusConfirmed, day.Add(8*time.Hour), 500)
	createMealWithCalories(t, st, userID, familyID, database.MealStatusPendingReview, day.Add(13*time.Hour), 300)
	createMealWithCalories(t, st, userID, familyID, database.MealStatusFailed, day.Add(18*time.Hour), 999)
	createMealWithCalories(t, st, userID, familyID, database.MealStatusProcessing, day.Add(20*time.Hour), 999)

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	dateStr := day.Format("2006-01-02")

	w := httptest.NewRecorder()
	h.GetFoodDailyTotals(w, withClaims(dailyTotalsRequest(fmt.Sprintf("from=%s&to=%s", dateStr, dateStr)), userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got []database.DailyTotal
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].Calories != 500 {
		t.Fatalf("expected only the confirmed meal's 500 calories, got %+v", got)
	}
}

func TestGetFoodDailyTotals_ToClampedToYesterday(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	h := server.NewFoodHandlers(st, nil, t.TempDir())

	today := time.Now().UTC().Format("2006-01-02")
	future := time.Now().UTC().AddDate(0, 0, 5).Format("2006-01-02")
	from := time.Now().UTC().AddDate(0, 0, -2).Format("2006-01-02")

	for _, to := range []string{today, future} {
		w := httptest.NewRecorder()
		h.GetFoodDailyTotals(w, withClaims(dailyTotalsRequest(fmt.Sprintf("from=%s&to=%s", from, to)), userID))
		if w.Code != http.StatusOK {
			t.Fatalf("to=%s: expected 200, got %d: %s", to, w.Code, w.Body.String())
		}
		var got []database.DailyTotal
		if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		yesterday := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
		if len(got) == 0 || got[len(got)-1].Date != yesterday {
			t.Errorf("to=%s: expected range clamped so last day is yesterday (%s), got %+v", to, yesterday, got)
		}
	}
}

func TestGetFoodDailyTotals_FromTodayInvertsAfterClamp(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	h := server.NewFoodHandlers(st, nil, t.TempDir())

	today := time.Now().UTC().Format("2006-01-02")
	w := httptest.NewRecorder()
	h.GetFoodDailyTotals(w, withClaims(dailyTotalsRequest(fmt.Sprintf("from=%s&to=%s", today, today)), userID))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 (from inverts to after clamp), got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetFoodDailyTotals_BadRequests(t *testing.T) {
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
			h.GetFoodDailyTotals(w, withClaims(dailyTotalsRequest(tc.query), userID))
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestGetFoodDailyTotals_Unauthorized(t *testing.T) {
	st := newFoodTestStorage(t)
	h := server.NewFoodHandlers(st, nil, t.TempDir())

	yesterday := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	w := httptest.NewRecorder()
	h.GetFoodDailyTotals(w, dailyTotalsRequest("from="+yesterday+"&to="+yesterday))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetFoodDailyTotals_NoUserOverride(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	otherUserID := uuid.New()

	day := time.Now().UTC().AddDate(0, 0, -2).Truncate(24 * time.Hour)
	createMealWithCalories(t, st, otherUserID, familyID, database.MealStatusConfirmed, day.Add(8*time.Hour), 900)

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	dateStr := day.Format("2006-01-02")

	w := httptest.NewRecorder()
	req := dailyTotalsRequest(fmt.Sprintf("from=%s&to=%s&user=%s", dateStr, dateStr, otherUserID))
	h.GetFoodDailyTotals(w, withClaims(req, userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got []database.DailyTotal
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].Calories != 0 {
		t.Errorf("expected caller's own (empty) day, not the other user's meals: %+v", got)
	}
}
