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

func createMealWithMacros(
	t *testing.T, st database.Storage, userID, familyID uuid.UUID, status string, loggedAt time.Time,
	calories, protein, carbs, fat, sugar, sodium float64,
) database.FoodMeal {
	t.Helper()
	meal := database.FoodMeal{
		UserID: userID, Status: status, LoggedAt: loggedAt, Name: "Meal",
		Calories: calories, ProteinGrams: protein, CarbsGrams: carbs, FatGrams: fat,
		SugarGrams: sugar, SodiumGrams: sodium,
	}
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
	// The three non-confirmed rows are reported as a count so a consumer can
	// tell this under-counted day apart from a genuinely 500 kcal one.
	if got[0].UnconfirmedMeals != 3 {
		t.Errorf("expected 3 unconfirmed meals, got %+v", got[0])
	}
}

// A day whose meals are all confirmed reports zero unconfirmed meals, and a
// day with no meals at all reports zero too — the two cases a consumer treats
// as "this total can be trusted" (the second trivially, having nothing to
// under-count).
func TestGetFoodDailyTotals_UnconfirmedMealCountIsZeroWhenNothingIsPending(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	base := time.Now().UTC().Truncate(24 * time.Hour)
	day2 := base.AddDate(0, 0, -2)
	createMealWithCalories(t, st, userID, familyID, database.MealStatusConfirmed, day2.Add(8*time.Hour), 400)
	createMealWithCalories(t, st, userID, familyID, database.MealStatusConfirmed, day2.Add(18*time.Hour), 600)
	// day -1 has no meals at all.

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	from := day2.Format("2006-01-02")
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
	if len(got) != 2 {
		t.Fatalf("expected 2 days, got %d: %+v", len(got), got)
	}
	if got[0].Calories != 1000 || got[0].UnconfirmedMeals != 0 {
		t.Errorf("day -2: expected 1000 calories and 0 unconfirmed, got %+v", got[0])
	}
	if got[1].Calories != 0 || got[1].UnconfirmedMeals != 0 {
		t.Errorf("day -1 (no meals): expected 0 calories and 0 unconfirmed, got %+v", got[1])
	}
}

// The count is scoped per Logged Day, not smeared across the range: a pending
// meal on one day must not mark a neighbouring day's total as under-counted.
func TestGetFoodDailyTotals_UnconfirmedMealCountIsPerDay(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	base := time.Now().UTC().Truncate(24 * time.Hour)
	day3 := base.AddDate(0, 0, -3)
	day2 := base.AddDate(0, 0, -2)
	createMealWithCalories(t, st, userID, familyID, database.MealStatusConfirmed, day3.Add(9*time.Hour), 800)
	createMealWithCalories(t, st, userID, familyID, database.MealStatusConfirmed, day2.Add(9*time.Hour), 700)
	createMealWithCalories(t, st, userID, familyID, database.MealStatusPendingClarification, day2.Add(20*time.Hour), 0)

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	from := day3.Format("2006-01-02")
	to := day2.Format("2006-01-02")

	w := httptest.NewRecorder()
	h.GetFoodDailyTotals(w, withClaims(dailyTotalsRequest(fmt.Sprintf("from=%s&to=%s", from, to)), userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got []database.DailyTotal
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 days, got %d: %+v", len(got), got)
	}
	if got[0].UnconfirmedMeals != 0 {
		t.Errorf("day -3: expected 0 unconfirmed, got %+v", got[0])
	}
	if got[1].UnconfirmedMeals != 1 {
		t.Errorf("day -2: expected 1 unconfirmed, got %+v", got[1])
	}
}

func TestGetFoodDailyTotals_MacroSugarSodiumSumsAreCorrect(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	day := time.Now().UTC().AddDate(0, 0, -2).Truncate(24 * time.Hour)
	createMealWithMacros(t, st, userID, familyID, database.MealStatusConfirmed, day.Add(8*time.Hour),
		500, 30, 60, 15, 10, 1.2)
	createMealWithMacros(t, st, userID, familyID, database.MealStatusConfirmed, day.Add(19*time.Hour),
		600, 40, 50, 20, 8, 1.5)

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
	if len(got) != 1 {
		t.Fatalf("expected 1 day, got %d: %+v", len(got), got)
	}
	d := got[0]
	if d.Calories != 1100 || d.ProteinGrams != 70 || d.CarbsGrams != 110 || d.FatGrams != 35 ||
		d.SugarGrams != 18 || d.SodiumGrams != 2.7 {
		t.Errorf("unexpected sums: %+v", d)
	}
}

// A non-confirmed meal contributes to UnconfirmedMeals and to none of the
// five sums, matching how it already behaves for Calories.
func TestGetFoodDailyTotals_UnconfirmedMealExcludedFromMacroSugarSodiumSums(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	day := time.Now().UTC().AddDate(0, 0, -2).Truncate(24 * time.Hour)
	createMealWithMacros(t, st, userID, familyID, database.MealStatusConfirmed, day.Add(8*time.Hour),
		500, 30, 60, 15, 10, 1.2)
	createMealWithMacros(t, st, userID, familyID, database.MealStatusPendingReview, day.Add(13*time.Hour),
		999, 99, 99, 99, 99, 9.9)

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
	if len(got) != 1 {
		t.Fatalf("expected 1 day, got %d: %+v", len(got), got)
	}
	d := got[0]
	if d.UnconfirmedMeals != 1 {
		t.Errorf("expected 1 unconfirmed meal, got %+v", d)
	}
	if d.Calories != 500 || d.ProteinGrams != 30 || d.CarbsGrams != 60 || d.FatGrams != 15 ||
		d.SugarGrams != 10 || d.SodiumGrams != 1.2 {
		t.Errorf("unconfirmed meal's macros must not be summed in: %+v", d)
	}
}

func TestGetFoodDailyTotals_NoMealsDayReturnsZeroForAllFiveSums(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	h := server.NewFoodHandlers(st, nil, t.TempDir())

	dateStr := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	w := httptest.NewRecorder()
	h.GetFoodDailyTotals(w, withClaims(dailyTotalsRequest(fmt.Sprintf("from=%s&to=%s", dateStr, dateStr)), userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got []database.DailyTotal
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 day, got %d: %+v", len(got), got)
	}
	d := got[0]
	if d.Calories != 0 || d.ProteinGrams != 0 || d.CarbsGrams != 0 || d.FatGrams != 0 ||
		d.SugarGrams != 0 || d.SodiumGrams != 0 {
		t.Errorf("expected all-zero sums for a day with no meals, got %+v", d)
	}
}

// Asserts against the raw JSON, not a decoded struct: decoding a missing key
// and a present zero-valued key both leave the Go field at its zero value, so
// only inspecting the raw map can tell "the key was omitted" apart from "the
// key was present and zero" — exactly the bug an `omitempty` tag would cause.
func TestGetFoodDailyTotals_AllFiveNewKeysArePresentInRawJSONEvenWhenZero(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	h := server.NewFoodHandlers(st, nil, t.TempDir())

	dateStr := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	w := httptest.NewRecorder()
	h.GetFoodDailyTotals(w, withClaims(dailyTotalsRequest(fmt.Sprintf("from=%s&to=%s", dateStr, dateStr)), userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var raw []map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(raw) != 1 {
		t.Fatalf("expected 1 day, got %d: %+v", len(raw), raw)
	}
	for _, key := range []string{"protein_grams", "carbs_grams", "fat_grams", "sugar_grams", "sodium_grams"} {
		v, ok := raw[0][key]
		if !ok {
			t.Errorf("expected key %q to be present even when zero", key)
			continue
		}
		if v != float64(0) {
			t.Errorf("expected key %q to be 0, got %v", key, v)
		}
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

func TestGetFoodDailyTotals_ExactlyMaxSpanSucceeds(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	h := server.NewFoodHandlers(st, nil, t.TempDir())

	yesterday := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	from := time.Now().UTC().AddDate(0, 0, -92).Format("2006-01-02")

	w := httptest.NewRecorder()
	h.GetFoodDailyTotals(w, withClaims(dailyTotalsRequest(fmt.Sprintf("from=%s&to=%s", from, yesterday)), userID))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for exactly a 92-day span, got %d: %s", w.Code, w.Body.String())
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
