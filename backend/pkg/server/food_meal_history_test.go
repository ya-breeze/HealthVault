package server_test

import (
	"bytes"
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

func listMealsRequest(query string) *http.Request {
	url := "/api/food/meals"
	if query != "" {
		url += "?" + query
	}
	return httptest.NewRequest(http.MethodGet, url, nil)
}

func createMealAt(t *testing.T, st database.Storage, userID, familyID uuid.UUID, status string, loggedAt time.Time) database.FoodMeal {
	t.Helper()
	meal := database.FoodMeal{UserID: userID, Status: status, LoggedAt: loggedAt, Name: "Meal"}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}
	return meal
}

func TestListMeals_OwnMealsOnlyAndOrderedMostRecentFirst(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	otherUserID := uuid.New()

	now := time.Now().UTC()
	createMealAt(t, st, userID, familyID, database.MealStatusConfirmed, now.Add(-2*time.Hour))
	createMealAt(t, st, userID, familyID, database.MealStatusPendingReview, now.Add(-1*time.Hour))
	createMealAt(t, st, userID, familyID, database.MealStatusProcessing, now)
	createMealAt(t, st, otherUserID, familyID, database.MealStatusConfirmed, now) // different user, same family

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.ListMeals(w, withClaims(listMealsRequest(""), userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got []server.MealSummary
	json.NewDecoder(w.Body).Decode(&got) //nolint:errcheck
	if len(got) != 3 {
		t.Fatalf("expected 3 of the caller's own meals (not the other user's), got %d", len(got))
	}
	if got[0].Status != database.MealStatusProcessing || got[2].Status != database.MealStatusConfirmed {
		t.Errorf("expected most-recent-first ordering, got %+v", got)
	}
}

func TestListMeals_DeterministicTieBreak(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	same := time.Now().UTC().Truncate(time.Second)
	createMealAt(t, st, userID, familyID, database.MealStatusConfirmed, same)
	createMealAt(t, st, userID, familyID, database.MealStatusConfirmed, same)

	h := server.NewFoodHandlers(st, nil, t.TempDir())

	var first []server.MealSummary
	w1 := httptest.NewRecorder()
	h.ListMeals(w1, withClaims(listMealsRequest(""), userID))
	json.NewDecoder(w1.Body).Decode(&first) //nolint:errcheck

	var second []server.MealSummary
	w2 := httptest.NewRecorder()
	h.ListMeals(w2, withClaims(listMealsRequest(""), userID))
	json.NewDecoder(w2.Body).Decode(&second) //nolint:errcheck

	if len(first) != 2 || len(second) != 2 {
		t.Fatalf("expected 2 tied meals in both responses, got %d and %d", len(first), len(second))
	}
	if first[0].ID != second[0].ID || first[1].ID != second[1].ID {
		t.Errorf("expected identical ordering across requests for tied logged_at, got %+v then %+v", first, second)
	}
}

// Regression: a naive `before`-cursor page that splits a tied-logged_at
// group at the cutoff would silently drop whichever half of the group
// wasn't included, since the next page's `logged_at < before` filter can't
// distinguish "already returned" from "shares this exact timestamp but
// wasn't returned yet". ListMeals must defer the whole tied group to the
// next page instead of splitting it.
// Regression: a timestamp-only `before` cursor can split a tied-logged_at
// group at a page boundary and then silently drop whichever part of it
// wasn't returned, since the next page's `logged_at < before` filter can't
// distinguish "already returned" from "shares this exact timestamp but
// wasn't returned yet". This is true even when the tie group is *larger*
// than the page limit (a plain "defer the whole group" fallback still loses
// rows in that case). The exact (logged_at, id) keyset cursor must page
// through an oversized tie group correctly, with no meal lost or repeated,
// regardless of how the group's size relates to the page limit.
func TestListMeals_LargeTieGroupPagesWithoutLoss(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	tied := time.Now().UTC()

	const tieGroupSize = 5
	const limit = 2 // smaller than the tie group, on purpose
	want := make(map[uuid.UUID]bool, tieGroupSize)
	for i := 0; i < tieGroupSize; i++ {
		m := createMealAt(t, st, userID, familyID, database.MealStatusConfirmed, tied)
		want[m.ID] = true
	}

	h := server.NewFoodHandlers(st, nil, t.TempDir())

	got := map[uuid.UUID]bool{}
	before, beforeID := "", ""
	for page := 0; page < tieGroupSize+1; page++ { // +1 guards against an infinite loop on a bug
		q := fmt.Sprintf("limit=%d", limit)
		if before != "" {
			q += "&before=" + before + "&before_id=" + beforeID
		}
		w := httptest.NewRecorder()
		h.ListMeals(w, withClaims(listMealsRequest(q), userID))
		var rows []server.MealSummary
		json.NewDecoder(w.Body).Decode(&rows) //nolint:errcheck
		if len(rows) == 0 {
			break
		}
		for _, row := range rows {
			if got[row.ID] {
				t.Fatalf("meal %s returned on more than one page — cursor repeated a row", row.ID)
			}
			got[row.ID] = true
		}
		last := rows[len(rows)-1]
		before, beforeID = last.LoggedAt.Format(time.RFC3339Nano), last.ID.String()
	}

	if len(got) != tieGroupSize {
		t.Fatalf("expected all %d tied meals reachable via paging, got %d: %+v", tieGroupSize, len(got), got)
	}
	for id := range want {
		if !got[id] {
			t.Errorf("meal %s in the tie group was never returned by any page", id)
		}
	}
}

func TestListMeals_SummaryOmitsInternalFields(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createMealAt(t, st, userID, familyID, database.MealStatusConfirmed, time.Now())
	if err := st.DB().Model(&database.FoodMeal{}).Where("id = ?", meal.ID).
		Updates(map[string]any{"photo_path": "secret/path.jpg", "raw_response": `{"leak":true}`}).Error; err != nil {
		t.Fatalf("seed internal fields: %v", err)
	}

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.ListMeals(w, withClaims(listMealsRequest(""), userID))

	var rows []map[string]any
	json.NewDecoder(w.Body).Decode(&rows) //nolint:errcheck
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	wantKeys := map[string]bool{"id": true, "name": true, "logged_at": true, "status": true, "calories": true}
	for k := range rows[0] {
		if !wantKeys[k] {
			t.Errorf("unexpected field %q in list response — internal/detail-only fields must not leak", k)
		}
	}
	for k := range wantKeys {
		if _, ok := rows[0][k]; !ok {
			t.Errorf("expected field %q in summary, missing", k)
		}
	}
}

func TestListMeals_DefaultAndMaxLimit(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	base := time.Now().UTC()
	for i := 0; i < 60; i++ {
		createMealAt(t, st, userID, familyID, database.MealStatusConfirmed, base.Add(-time.Duration(i)*time.Minute))
	}
	h := server.NewFoodHandlers(st, nil, t.TempDir())

	w1 := httptest.NewRecorder()
	h.ListMeals(w1, withClaims(listMealsRequest(""), userID))
	var defaultPage []server.MealSummary
	json.NewDecoder(w1.Body).Decode(&defaultPage) //nolint:errcheck
	if len(defaultPage) != 50 {
		t.Errorf("expected default limit of 50, got %d", len(defaultPage))
	}

	w2 := httptest.NewRecorder()
	h.ListMeals(w2, withClaims(listMealsRequest("limit=5"), userID))
	var small []server.MealSummary
	json.NewDecoder(w2.Body).Decode(&small) //nolint:errcheck
	if len(small) != 5 {
		t.Errorf("expected limit=5 to return 5, got %d", len(small))
	}
}

func TestListMeals_InvalidLimitRejected(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	createMealAt(t, st, userID, familyID, database.MealStatusConfirmed, time.Now())
	h := server.NewFoodHandlers(st, nil, t.TempDir())

	for _, limit := range []string{"-1", "0", "201", "abc"} {
		w := httptest.NewRecorder()
		h.ListMeals(w, withClaims(listMealsRequest("limit="+limit), userID))
		if w.Code != http.StatusBadRequest {
			t.Errorf("limit=%s: expected 400, got %d: %s", limit, w.Code, w.Body.String())
		}
	}
}

func TestListMeals_BeforeCursorPagesToOlderMeals(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	base := time.Now().UTC()
	oldest := createMealAt(t, st, userID, familyID, database.MealStatusConfirmed, base.Add(-2*time.Hour))
	createMealAt(t, st, userID, familyID, database.MealStatusConfirmed, base.Add(-1*time.Hour))
	createMealAt(t, st, userID, familyID, database.MealStatusConfirmed, base)

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.ListMeals(w, withClaims(listMealsRequest("limit=1&before="+base.Format(time.RFC3339)), userID))

	var got []server.MealSummary
	json.NewDecoder(w.Body).Decode(&got) //nolint:errcheck
	if len(got) != 1 || got[0].LoggedAt.Sub(base.Add(-1*time.Hour)).Abs() > time.Second {
		t.Fatalf("expected the middle meal before the most recent, got %+v", got)
	}

	w2 := httptest.NewRecorder()
	h.ListMeals(w2, withClaims(listMealsRequest("before="+base.Add(-1*time.Hour).Format(time.RFC3339)), userID))
	var got2 []server.MealSummary
	json.NewDecoder(w2.Body).Decode(&got2) //nolint:errcheck
	if len(got2) != 1 || got2[0].ID != oldest.ID {
		t.Fatalf("expected paging to reach the oldest meal, got %+v", got2)
	}
}

func TestListMeals_InvalidBeforeRejected(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.ListMeals(w, withClaims(listMealsRequest("before=not-a-timestamp"), userID))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListMeals_Unauthenticated(t *testing.T) {
	st := newFoodTestStorage(t)
	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.ListMeals(w, listMealsRequest(""))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// --- PatchMeal ---

func patchMealNameRequest(mealID string, body map[string]any) *http.Request {
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(http.MethodPatch, "/api/food/meals/"+mealID, bytes.NewBuffer(b))
	return mux.SetURLVars(r, map[string]string{"id": mealID})
}

func TestPatchMeal_RenameConfirmedMeal(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createMealAt(t, st, userID, familyID, database.MealStatusConfirmed, time.Now())

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.PatchMeal(w, withClaims(patchMealNameRequest(meal.ID.String(), map[string]any{"name": "Dinner"}), userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got database.FoodMeal
	json.NewDecoder(w.Body).Decode(&got) //nolint:errcheck
	if got.Name != "Dinner" {
		t.Errorf("expected name updated, got %q", got.Name)
	}
}

func TestPatchMeal_CorrectLoggedAt(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createMealAt(t, st, userID, familyID, database.MealStatusConfirmed, time.Now())
	newTime := time.Now().Add(-24 * time.Hour).UTC().Truncate(time.Second)

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.PatchMeal(w, withClaims(patchMealNameRequest(meal.ID.String(), map[string]any{"logged_at": newTime.Format(time.RFC3339)}), userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got database.FoodMeal
	json.NewDecoder(w.Body).Decode(&got) //nolint:errcheck
	if !got.LoggedAt.Equal(newTime) {
		t.Errorf("expected logged_at = %v, got %v", newTime, got.LoggedAt)
	}
}

func TestPatchMeal_EmptyBodyReturns400(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createMealAt(t, st, userID, familyID, database.MealStatusConfirmed, time.Now())

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.PatchMeal(w, withClaims(patchMealNameRequest(meal.ID.String(), map[string]any{}), userID))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPatchMeal_ZeroLoggedAtReturns400(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createMealAt(t, st, userID, familyID, database.MealStatusConfirmed, time.Now())

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.PatchMeal(w, withClaims(patchMealNameRequest(meal.ID.String(), map[string]any{"logged_at": "0001-01-01T00:00:00Z"}), userID))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for zero-value logged_at, got %d: %s", w.Code, w.Body.String())
	}

	var reloaded database.FoodMeal
	st.DB().Where("id = ?", meal.ID).First(&reloaded) //nolint:errcheck
	if reloaded.LoggedAt.IsZero() {
		t.Error("logged_at must not have been overwritten with the zero value")
	}
}

func TestPatchMeal_NonEditableStatusReturns409(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createMealAt(t, st, userID, familyID, database.MealStatusProcessing, time.Now())

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.PatchMeal(w, withClaims(patchMealNameRequest(meal.ID.String(), map[string]any{"name": "x"}), userID))
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPatchMeal_CrossUserReturns404(t *testing.T) {
	st := newFoodTestStorage(t)
	_, familyID := seedFoodUser(t, st)
	otherUserID := uuid.New()
	meal := createMealAt(t, st, otherUserID, familyID, database.MealStatusConfirmed, time.Now())

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.PatchMeal(w, withClaims(patchMealNameRequest(meal.ID.String(), map[string]any{"name": "x"}), uuid.New()))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
