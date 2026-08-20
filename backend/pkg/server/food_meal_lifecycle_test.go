package server_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/server"
	photostorage "github.com/ya-breeze/healthvault/pkg/storage"
	"github.com/ya-breeze/healthvault/pkg/vision"
	"gorm.io/gorm"
)

// newFileFoodTestStorage is like newFoodTestStorage but file-backed rather
// than :memory:. Only needed by registerRaceSimulation: a :memory: SQLite
// database is private to whichever connection created it, so a second
// connection checked out from the pool (as happens for a nested query issued
// from inside a GORM callback) gets its own empty database instead of
// sharing the same one. A temp file has no such isolation — every
// connection, however many the pool opens, sees the same physical file.
func newFileFoodTestStorage(t *testing.T) database.Storage {
	t.Helper()
	path := filepath.Join(t.TempDir(), "race-sim.db")
	db, err := database.Open(slog.New(slog.NewTextHandler(os.Stderr, nil)), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return database.NewStorage(db)
}

func mealDetailRequest(method, id string) *http.Request {
	r := httptest.NewRequest(method, "/api/food/meals/"+id, nil)
	return mux.SetURLVars(r, map[string]string{"id": id})
}

func createUnresolvedMeal(t *testing.T, st database.Storage, userID, familyID uuid.UUID) database.FoodMeal {
	t.Helper()
	meal := database.FoodMeal{UserID: userID, Status: database.MealStatusPendingReview, LoggedAt: time.Now()}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}
	item := database.FoodItem{
		UserID: userID, MealID: meal.ID, Name: "Chicken", WeightGrams: 100,
		MacroSource: database.MacroSourceNone,
	}
	item.ID = uuid.New()
	item.FamilyID = familyID
	if err := st.DB().Create(&item).Error; err != nil {
		t.Fatalf("create item: %v", err)
	}
	meal.Items = []database.FoodItem{item}
	return meal
}

// createEstimatedItemMeal is like createUnresolvedMeal but the item already
// carries a persisted estimate and macro_source=estimated, mirroring what
// resolveItems' fallback produces when no candidate matches.
func createEstimatedItemMeal(t *testing.T, st database.Storage, userID, familyID uuid.UUID) database.FoodMeal {
	t.Helper()
	meal := database.FoodMeal{UserID: userID, Status: database.MealStatusPendingReview, LoggedAt: time.Now()}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}
	item := database.FoodItem{
		UserID: userID, MealID: meal.ID, Name: "Mexican vegetable mix", WeightGrams: 100,
	}
	item.ID = uuid.New()
	item.FamilyID = familyID
	item.SetEstimatedProfile(&database.NutrientProfile{CaloriesPer100g: 90, ProteinPer100g: 3, CarbsPer100g: 15})
	item.ApplyEstimatedProfile()
	if err := st.DB().Create(&item).Error; err != nil {
		t.Fatalf("create item: %v", err)
	}
	meal.Items = []database.FoodItem{item}
	return meal
}

// --- GetMeal ---

func TestGetMeal_ReturnsMealWithItems(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createUnresolvedMeal(t, st, userID, familyID)

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.GetMeal(w, withClaims(mealDetailRequest(http.MethodGet, meal.ID.String()), userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got database.FoodMeal
	json.NewDecoder(w.Body).Decode(&got) //nolint:errcheck
	if len(got.Items) != 1 {
		t.Errorf("expected 1 item, got %d", len(got.Items))
	}
}

// Regression: GetMeal must not hand-roll a response shape that silently
// drops canonical_name — see openspec/specs/food-nutrition-logging "Food
// Item and Custom Food Carry a Canonical Name".
func TestGetMeal_ResponseIncludesCanonicalName(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := database.FoodMeal{UserID: userID, Status: database.MealStatusPendingReview, LoggedAt: time.Now()}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}
	item := database.FoodItem{
		UserID: userID, MealID: meal.ID, Name: "вареники", CanonicalName: "dumplings", WeightGrams: 100,
		MacroSource: database.MacroSourceNone,
	}
	item.ID = uuid.New()
	item.FamilyID = familyID
	if err := st.DB().Create(&item).Error; err != nil {
		t.Fatalf("create item: %v", err)
	}

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.GetMeal(w, withClaims(mealDetailRequest(http.MethodGet, meal.ID.String()), userID))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"canonical_name":"dumplings"`) {
		t.Errorf("expected canonical_name in the response body, got %s", w.Body.String())
	}
}

func TestGetMeal_CrossUserReturns404(t *testing.T) {
	st := newFoodTestStorage(t)
	_, familyID := seedFoodUser(t, st)
	otherUserID := uuid.New()
	meal := createUnresolvedMeal(t, st, otherUserID, familyID)

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.GetMeal(w, withClaims(mealDetailRequest(http.MethodGet, meal.ID.String()), uuid.New()))

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// --- ConfirmMeal ---

func TestConfirmMeal_AggregatesReferenceAndManualExcludesNone(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	meal := database.FoodMeal{UserID: userID, Status: database.MealStatusPendingReview, LoggedAt: time.Now()}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}

	ref := database.FoodItem{UserID: userID, MealID: meal.ID, Name: "Rice", WeightGrams: 100, MacroSource: database.MacroSourceReference, Calories: 130}
	ref.ID = uuid.New()
	ref.FamilyID = familyID
	manual := database.FoodItem{UserID: userID, MealID: meal.ID, Name: "Sauce", MacroSource: database.MacroSourceManual, Calories: 50}
	manual.ID = uuid.New()
	manual.FamilyID = familyID
	none := database.FoodItem{UserID: userID, MealID: meal.ID, Name: "Mystery", MacroSource: database.MacroSourceNone}
	none.ID = uuid.New()
	none.FamilyID = familyID
	for _, it := range []database.FoodItem{ref, manual, none} {
		if err := st.DB().Create(&it).Error; err != nil {
			t.Fatalf("create item: %v", err)
		}
	}

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.ConfirmMeal(w, withClaims(mealDetailRequest(http.MethodPut, meal.ID.String()), userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got database.FoodMeal
	json.NewDecoder(w.Body).Decode(&got) //nolint:errcheck
	if got.Status != database.MealStatusConfirmed {
		t.Errorf("expected confirmed, got %s", got.Status)
	}
	if got.Calories != 180 {
		t.Errorf("expected aggregate 180 (130+50, excluding none), got %v", got.Calories)
	}

	var persisted database.FoodMeal
	st.DB().First(&persisted, "id = ?", meal.ID) //nolint:errcheck
	if persisted.Calories != 180 || persisted.Status != database.MealStatusConfirmed {
		t.Errorf("expected persisted aggregate 180 and confirmed status, got %+v", persisted)
	}
}

func TestConfirmMeal_CorrectsLoggedAt(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := database.FoodMeal{UserID: userID, Status: database.MealStatusPendingReview, LoggedAt: time.Now()}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}

	yesterday := time.Now().UTC().AddDate(0, 0, -1).Truncate(time.Second)
	body, _ := json.Marshal(map[string]any{"logged_at": yesterday.Format(time.RFC3339)})
	r := mux.SetURLVars(httptest.NewRequest(http.MethodPut, "/api/food/meals/"+meal.ID.String()+"/confirm", bytes.NewBuffer(body)),
		map[string]string{"id": meal.ID.String()})

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.ConfirmMeal(w, withClaims(r, userID))

	var got database.FoodMeal
	json.NewDecoder(w.Body).Decode(&got) //nolint:errcheck
	if !got.LoggedAt.Equal(yesterday) {
		t.Errorf("expected logged_at %v, got %v", yesterday, got.LoggedAt)
	}
}

func TestConfirmMeal_NoNutritionRowWritten(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := database.FoodMeal{UserID: userID, Status: database.MealStatusPendingReview, LoggedAt: time.Now()}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}

	var before int64
	st.DB().Table("nutritions").Count(&before)

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.ConfirmMeal(w, withClaims(mealDetailRequest(http.MethodPut, meal.ID.String()), userID))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var after int64
	st.DB().Table("nutritions").Count(&after)
	if after != before {
		t.Errorf("expected no nutrition rows written, before=%d after=%d", before, after)
	}
}

func TestConfirmMeal_CrossUserReturns404(t *testing.T) {
	st := newFoodTestStorage(t)
	_, familyID := seedFoodUser(t, st)
	otherUserID := uuid.New()
	meal := database.FoodMeal{UserID: otherUserID, Status: database.MealStatusPendingReview, LoggedAt: time.Now()}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.ConfirmMeal(w, withClaims(mealDetailRequest(http.MethodPut, meal.ID.String()), uuid.New()))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// Only pending_review is a valid state to confirm from — not processing (a
// live or stale analysis racing its own status write), not failed (no
// usable items), not pending_clarification (items not yet resolved), and
// not an already-confirmed meal (items are frozen past that point, so a
// second confirm has nothing meaningful left to do).
func TestConfirmMeal_RejectsNonPendingReviewStatuses(t *testing.T) {
	for _, status := range []string{
		database.MealStatusProcessing,
		database.MealStatusFailed,
		database.MealStatusPendingClarification,
		database.MealStatusConfirmed,
	} {
		t.Run(status, func(t *testing.T) {
			st := newFoodTestStorage(t)
			userID, familyID := seedFoodUser(t, st)
			meal := database.FoodMeal{UserID: userID, Status: status, LoggedAt: time.Now()}
			meal.ID = uuid.New()
			meal.FamilyID = familyID
			if err := st.DB().Create(&meal).Error; err != nil {
				t.Fatalf("create meal: %v", err)
			}

			h := server.NewFoodHandlers(st, nil, t.TempDir())
			w := httptest.NewRecorder()
			h.ConfirmMeal(w, withClaims(mealDetailRequest(http.MethodPut, meal.ID.String()), userID))
			if w.Code != http.StatusConflict {
				t.Errorf("status=%s: expected 409, got %d: %s", status, w.Code, w.Body.String())
			}

			var persisted database.FoodMeal
			st.DB().First(&persisted, "id = ?", meal.ID) //nolint:errcheck
			if persisted.Status != status {
				t.Errorf("status=%s: expected status to stay unchanged, got %s", status, persisted.Status)
			}
		})
	}
}

// Regression: ConfirmMeal's final write used to be an unconditional
// `UPDATE ... WHERE id = ?`, not conditioned on status. If a concurrent
// Reanalyze claimed the meal (pending_review -> processing) in the window
// between ConfirmMeal's initial read and that write, the blind write would
// still land, silently re-confirming a meal that reanalysis was actively
// replacing the items of. A GORM Before-update hook simulates that race
// deterministically — it fires synchronously, right before ConfirmMeal's own
// UPDATE statement runs, without needing real goroutines.
func TestConfirmMeal_ConcurrentStatusChangeBeforeWriteIsRejected(t *testing.T) {
	st := newFileFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createUnresolvedMeal(t, st, userID, familyID) // status: pending_review

	registerRaceSimulation(t, st.DB(), func() {
		sqlDB, err := st.DB().DB()
		if err != nil {
			t.Fatalf("get sql.DB: %v", err)
		}
		if _, err := sqlDB.Exec(
			"UPDATE food_meals SET status = ? WHERE id = ?", database.MealStatusProcessing, meal.ID,
		); err != nil {
			t.Fatalf("simulate concurrent claim: %v", err)
		}
	})

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.ConfirmMeal(w, withClaims(mealDetailRequest(http.MethodPut, meal.ID.String()), userID))

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 when status changed concurrently, got %d: %s", w.Code, w.Body.String())
	}
	var reloaded database.FoodMeal
	if err := st.DB().Where("id = ?", meal.ID).First(&reloaded).Error; err != nil {
		t.Fatalf("reload meal: %v", err)
	}
	if reloaded.Status != database.MealStatusProcessing {
		t.Errorf("expected the concurrent change to stick (not be overwritten by confirm), got status=%s", reloaded.Status)
	}
}

// Regression: ConfirmMeal used to aggregate from the item snapshot loaded at
// the very start of the handler, before any write. A concurrent item edit
// (permitted while pending_review, and pending_review edits intentionally
// don't recompute the meal's aggregate) could commit after that snapshot but
// before confirm's write — and since the item edit never touches the meal's
// own status, confirm's status-only conditional write would still match,
// confirming with an aggregate computed from stale items. ConfirmMeal now
// claims (a write) before reading items, which — per SQLite's single-writer
// model — means the item read that follows can't be racing a concurrent
// item-edit transaction: none can acquire the writer lock until this one
// commits. This test injects the "concurrent" edit via a hook that fires
// right before the claim's own UPDATE, using a nested connection (hence the
// file-backed storage — see newFileFoodTestStorage), and checks the
// confirmed aggregate reflects the injected value, not the pre-handler one.
func TestConfirmMeal_AggregatesFreshItemsDespiteConcurrentEditBeforeTransaction(t *testing.T) {
	st := newFileFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createUnresolvedMeal(t, st, userID, familyID)
	if err := st.DB().Model(&database.FoodItem{}).Where("id = ?", meal.Items[0].ID).
		Updates(map[string]any{"macro_source": database.MacroSourceManual, "calories": 100}).Error; err != nil {
		t.Fatalf("seed item macros: %v", err)
	}

	registerRaceSimulation(t, st.DB(), func() {
		sqlDB, err := st.DB().DB()
		if err != nil {
			t.Fatalf("get sql.DB: %v", err)
		}
		if _, err := sqlDB.Exec(
			"UPDATE food_items SET calories = ? WHERE id = ?", 500.0, meal.Items[0].ID,
		); err != nil {
			t.Fatalf("simulate concurrent item edit: %v", err)
		}
	})

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.ConfirmMeal(w, withClaims(mealDetailRequest(http.MethodPut, meal.ID.String()), userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got database.FoodMeal
	json.NewDecoder(w.Body).Decode(&got) //nolint:errcheck
	if got.Calories != 500 {
		t.Errorf("expected the confirmed aggregate to reflect the item's fresh value (500), not the stale pre-transaction snapshot (100), got %v", got.Calories)
	}

	var reloaded database.FoodMeal
	if err := st.DB().Where("id = ?", meal.ID).First(&reloaded).Error; err != nil {
		t.Fatalf("reload meal: %v", err)
	}
	if reloaded.Calories != 500 {
		t.Errorf("expected the persisted aggregate to be 500, got %v", reloaded.Calories)
	}
}

// registerRaceSimulation registers a one-shot GORM Before-update hook that
// runs fn immediately before the next Model().Updates() call executes its
// SQL, then unregisters itself — simulating a concurrent write landing in
// the gap between a handler's read and its own conditional write, without
// real concurrency. fn typically issues a raw Exec (bypassing GORM's own
// update callback chain, so it doesn't re-trigger this same hook).
func registerRaceSimulation(t *testing.T, db *gorm.DB, fn func()) {
	t.Helper()
	const name = "test:simulate-race"
	fired := false
	db.Callback().Update().Before("gorm:update").Register(name, func(*gorm.DB) {
		if fired {
			return
		}
		fired = true
		fn()
	})
	t.Cleanup(func() { db.Callback().Update().Remove(name) })
}

// --- PatchMealItem ---

func itemPatchRequest(mealID, itemID string, body map[string]any) *http.Request {
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(http.MethodPatch, "/api/food/meals/"+mealID+"/items/"+itemID, bytes.NewBuffer(b))
	return mux.SetURLVars(r, map[string]string{"id": mealID, "item_id": itemID})
}

func TestPatchMealItem_BindToFdcID(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createUnresolvedMeal(t, st, userID, familyID)
	idx := buildUSDAIndex(t, usdaFood(7, "Chicken breast", 165))

	h := server.NewFoodHandlers(st, idx, t.TempDir())
	w := httptest.NewRecorder()
	r := itemPatchRequest(meal.ID.String(), meal.Items[0].ID.String(), map[string]any{"fdc_id": 7})
	h.PatchMealItem(w, withClaims(r, userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var gotMeal database.FoodMeal
	json.NewDecoder(w.Body).Decode(&gotMeal) //nolint:errcheck
	got := gotMeal.Items[0]
	if got.MacroSource != database.MacroSourceReference {
		t.Errorf("expected reference, got %s", got.MacroSource)
	}
	// 165 kcal/100g * 1.0 (100g) = 165
	if got.Calories < 164.9 || got.Calories > 165.1 {
		t.Errorf("expected calories ~165, got %v", got.Calories)
	}
}

func TestPatchMealItem_BindToOffCode(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createUnresolvedMeal(t, st, userID, familyID)
	offIdx := buildOFFIndex(t, offFood("8594001222227", "Bílý jogurt", "Olma", 65))

	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithOFF(offIdx)
	w := httptest.NewRecorder()
	r := itemPatchRequest(meal.ID.String(), meal.Items[0].ID.String(), map[string]any{"off_code": "8594001222227"})
	h.PatchMealItem(w, withClaims(r, userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var gotMeal database.FoodMeal
	json.NewDecoder(w.Body).Decode(&gotMeal) //nolint:errcheck
	got := gotMeal.Items[0]
	if got.MacroSource != database.MacroSourceReference {
		t.Errorf("expected reference, got %s", got.MacroSource)
	}
	if got.OffCode == nil || *got.OffCode != "8594001222227" {
		t.Errorf("expected off_code bound on the item, got %+v", got.OffCode)
	}
	// 65 kcal/100g * 1.0 (100g) = 65
	if got.Calories < 64.9 || got.Calories > 65.1 {
		t.Errorf("expected calories ~65, got %v", got.Calories)
	}
}

// Regression: binding an item to a food match previously left item.Name as
// whatever the vision model guessed, forever, even when that guess was
// wrong (e.g. "dark berries" bound to a "Cherries, sweet, raw" search
// result would still display as "dark berries"). The frontend now sends the
// matched food's real name alongside fdc_id; the backend must apply it.
func TestPatchMealItem_BindUpdatesNameWhenProvided(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createUnresolvedMeal(t, st, userID, familyID)
	idx := buildUSDAIndex(t, usdaFood(7, "Chicken breast", 165))

	h := server.NewFoodHandlers(st, idx, t.TempDir())
	w := httptest.NewRecorder()
	r := itemPatchRequest(meal.ID.String(), meal.Items[0].ID.String(), map[string]any{
		"fdc_id": 7, "name": "Chicken breast",
	})
	h.PatchMealItem(w, withClaims(r, userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var gotMeal database.FoodMeal
	json.NewDecoder(w.Body).Decode(&gotMeal) //nolint:errcheck
	if gotMeal.Items[0].Name != "Chicken breast" {
		t.Errorf("expected name to update to the bound match, got %q", gotMeal.Items[0].Name)
	}
}

// Regression: an item that's already matched (macro_source=reference or
// manual) previously had no way to be corrected in the UI — the backend
// itself never restricted this, but confirm it still isn't restricted now
// that the frontend relies on it: re-binding a matched item to a different
// food changes both its macros and its name.
func TestPatchMealItem_RebindAlreadyMatchedItemChangesNameAndMacros(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createUnresolvedMeal(t, st, userID, familyID)
	idx := buildUSDAIndex(t, usdaFood(7, "Dark berries", 43), usdaFood(8, "Cherries, sweet, raw", 63))

	h := server.NewFoodHandlers(st, idx, t.TempDir())

	first := itemPatchRequest(meal.ID.String(), meal.Items[0].ID.String(), map[string]any{
		"fdc_id": 7, "name": "Dark berries",
	})
	w1 := httptest.NewRecorder()
	h.PatchMealItem(w1, withClaims(first, userID))
	if w1.Code != http.StatusOK {
		t.Fatalf("expected 200 on first bind, got %d: %s", w1.Code, w1.Body.String())
	}

	second := itemPatchRequest(meal.ID.String(), meal.Items[0].ID.String(), map[string]any{
		"fdc_id": 8, "name": "Cherries, sweet, raw",
	})
	w2 := httptest.NewRecorder()
	h.PatchMealItem(w2, withClaims(second, userID))
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 on rebind of an already-matched item, got %d: %s", w2.Code, w2.Body.String())
	}
	var gotMeal database.FoodMeal
	json.NewDecoder(w2.Body).Decode(&gotMeal) //nolint:errcheck
	got := gotMeal.Items[0]
	if got.Name != "Cherries, sweet, raw" {
		t.Errorf("expected name to reflect the corrected match, got %q", got.Name)
	}
	// 63 kcal/100g * 1.0 (100g) = 63
	if got.Calories < 62.9 || got.Calories > 63.1 {
		t.Errorf("expected calories from the new match (~63), got %v", got.Calories)
	}
}

// Regression: renaming an item with no other field set previously hit the
// "nothing to update" 400 — name-only edits (manual mode without changing
// macros) must be accepted.
func TestPatchMealItem_NameAloneIsAcceptedAndDoesNotTouchMacros(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createUnresolvedMeal(t, st, userID, familyID)
	idx := buildUSDAIndex(t, usdaFood(7, "Chicken breast", 165))
	h := server.NewFoodHandlers(st, idx, t.TempDir())

	bind := itemPatchRequest(meal.ID.String(), meal.Items[0].ID.String(), map[string]any{"fdc_id": 7})
	w1 := httptest.NewRecorder()
	h.PatchMealItem(w1, withClaims(bind, userID))
	if w1.Code != http.StatusOK {
		t.Fatalf("expected 200 on bind, got %d: %s", w1.Code, w1.Body.String())
	}

	rename := itemPatchRequest(meal.ID.String(), meal.Items[0].ID.String(), map[string]any{"name": "Grilled chicken"})
	w2 := httptest.NewRecorder()
	h.PatchMealItem(w2, withClaims(rename, userID))
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 on name-only patch, got %d: %s", w2.Code, w2.Body.String())
	}
	var gotMeal database.FoodMeal
	json.NewDecoder(w2.Body).Decode(&gotMeal) //nolint:errcheck
	got := gotMeal.Items[0]
	if got.Name != "Grilled chicken" {
		t.Errorf("expected name to update, got %q", got.Name)
	}
	if got.MacroSource != database.MacroSourceReference {
		t.Errorf("expected macro_source to stay reference, got %s", got.MacroSource)
	}
	// 165 kcal/100g * 1.0 (100g) = 165 — must not have been touched by the rename.
	if got.Calories < 164.9 || got.Calories > 165.1 {
		t.Errorf("expected calories to stay ~165 from the earlier bind, got %v", got.Calories)
	}
}

// Regression: an empty or whitespace-only name has nothing to apply, so it
// must not alone satisfy "something to update" — otherwise the request
// silently 200s and changes nothing, which defeats the point of the guard.
func TestPatchMealItem_BlankNameAloneReturns400(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createUnresolvedMeal(t, st, userID, familyID)
	h := server.NewFoodHandlers(st, nil, t.TempDir())

	for _, name := range []string{"", "   "} {
		r := itemPatchRequest(meal.ID.String(), meal.Items[0].ID.String(), map[string]any{"name": name})
		w := httptest.NewRecorder()
		h.PatchMealItem(w, withClaims(r, userID))
		if w.Code != http.StatusBadRequest {
			t.Errorf("name=%q: expected 400, got %d: %s", name, w.Code, w.Body.String())
		}
	}
}

// Regression: supplying both fdc_id and custom_food_id together used to be
// accepted — resolveReferenceProfile resolves only fdc_id (its precedence
// order), but the item ends up with both IDs persisted, claiming a binding
// to a custom food whose profile was never actually used.
func TestPatchMealItem_BothReferenceIDsReturns400(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	idx := buildUSDAIndex(t, usdaFood(7, "Chicken breast", 165))
	offIdx := buildOFFIndex(t, offFood("111", "Jogurt", "Olma", 65))
	h := server.NewFoodHandlers(st, idx, t.TempDir()).WithOFF(offIdx)
	customFoodID := uuid.New()

	// Extended for off_code (task 6.3): every pairing among
	// fdc_id/off_code/custom_food_id must be rejected, plus all three set.
	cases := []struct {
		name string
		body map[string]any
	}{
		{"fdc_id plus custom_food_id", map[string]any{"fdc_id": 7, "custom_food_id": customFoodID.String()}},
		{"fdc_id plus off_code", map[string]any{"fdc_id": 7, "off_code": "111"}},
		{"off_code plus custom_food_id", map[string]any{"off_code": "111", "custom_food_id": customFoodID.String()}},
		{"all three reference ids", map[string]any{"fdc_id": 7, "off_code": "111", "custom_food_id": customFoodID.String()}},
	}
	for _, c := range cases {
		meal := createUnresolvedMeal(t, st, userID, familyID)
		r := itemPatchRequest(meal.ID.String(), meal.Items[0].ID.String(), c.body)
		w := httptest.NewRecorder()
		h.PatchMealItem(w, withClaims(r, userID))
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d: %s", c.name, w.Code, w.Body.String())
		}
	}
}

// Regression: round-7 review — CreateMealItem already rejected manual
// combined with a reference ID, and the round-2 design/commit said PATCH
// does too, but only Create actually implemented the check. A body like
// {"manual":true,"fdc_id":7,"calories":100} entered the manual branch,
// cleared any existing binding, and silently discarded the supplied fdc_id.
func TestPatchMealItem_ManualPlusReferenceReturns400(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	idx := buildUSDAIndex(t, usdaFood(7, "Chicken breast", 165))
	offIdx := buildOFFIndex(t, offFood("111", "Jogurt", "Olma", 65))
	h := server.NewFoodHandlers(st, idx, t.TempDir()).WithOFF(offIdx)

	cases := []struct {
		name string
		body map[string]any
	}{
		{"manual plus fdc_id", map[string]any{"manual": true, "calories": 100, "fdc_id": 7}},
		{"manual plus custom_food_id", map[string]any{"manual": true, "calories": 100, "custom_food_id": uuid.New().String()}},
		{"manual plus off_code", map[string]any{"manual": true, "calories": 100, "off_code": "111"}},
	}
	for _, c := range cases {
		meal := createUnresolvedMeal(t, st, userID, familyID)
		r := itemPatchRequest(meal.ID.String(), meal.Items[0].ID.String(), c.body)
		w := httptest.NewRecorder()
		h.PatchMealItem(w, withClaims(r, userID))
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d: %s", c.name, w.Code, w.Body.String())
		}
	}
}

// Regression: round-7 review — item was loaded once, outside
// applyItemMutation's transaction, mutated in memory, and blindly Saved
// (writing every column) inside the transaction. The shipped UI can fire a
// weight-only PATCH (the weight input's onBlur) and a binding PATCH (a
// search result click) for the same item close together; a stale
// outside-the-transaction snapshot let whichever request's write committed
// second silently overwrite the other's already-committed change with its
// own stale copy of every other column.
//
// Round 9 review: the round-7 fix's first attempt at closing this reported
// *any* SQLITE_BUSY_SNAPSHOT (this transaction's read snapshot going stale
// because some other transaction committed first) as a 409 conflict — but
// that alone doesn't prove this specific item conflicted; an unrelated
// write anywhere in the database triggers the same condition, and a
// concurrent change to a *different* field (like this test's rename) isn't
// actually incompatible with this request's own change. The item is now
// loaded inside the transaction (this test's own regression) *and* the
// whole transaction retries with a fresh snapshot on SQLITE_BUSY_SNAPSHOT
// (see applyItemMutation) — since the item is reloaded fresh on every
// attempt, a retry naturally re-reads the concurrent rename and merges this
// request's weight change on top of it, rather than rejecting a change that
// was never actually in conflict.
func TestPatchMealItem_ConcurrentNonConflictingWriteMergesRatherThanFalseConflict(t *testing.T) {
	// File-backed, not :memory: — the hook below issues a genuinely separate
	// connection-pool write, and an in-memory SQLite database is private per
	// connection, so a separate connection would see an empty, schema-less
	// database rather than this test's actual data (see newFileFoodTestStorage's
	// own doc comment).
	st := newFileFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createUnresolvedMeal(t, st, userID, familyID)
	itemID := meal.Items[0].ID

	registerRaceSimulation(t, st.DB(), func() {
		// Simulates a second request's PATCH committing in the window
		// between this request's own item load and its conditional write —
		// a plain top-level write, not nested in this request's transaction.
		// Only fires once, so this doesn't affect the retry attempt below.
		if err := st.DB().Model(&database.FoodItem{}).Where("id = ?", itemID).
			Update("name", "Renamed by a concurrent request").Error; err != nil {
			t.Fatalf("simulate concurrent write: %v", err)
		}
	})

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	r := itemPatchRequest(meal.ID.String(), itemID.String(), map[string]any{"weight_grams": 250})
	h.PatchMealItem(w, withClaims(r, userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (a retry merges the non-conflicting concurrent change), got %d: %s", w.Code, w.Body.String())
	}

	var got database.FoodItem
	if err := st.DB().Where("id = ?", itemID).First(&got).Error; err != nil {
		t.Fatalf("reload item: %v", err)
	}
	if got.Name != "Renamed by a concurrent request" {
		t.Errorf("expected the concurrent rename to survive the merge, got name=%q", got.Name)
	}
	if got.WeightGrams != 250 {
		t.Errorf("expected this request's own weight change to also apply, got %v", got.WeightGrams)
	}
}

// Regression: round 9 review — applyItemMutation's retry on
// SQLITE_BUSY_SNAPSHOT must be bounded, not an infinite loop, if the
// staleness genuinely persists across every attempt (e.g. sustained
// writes to the very item this request is trying to change). Round 10
// review: bounded-retry exhaustion is exactly the "a write still based on
// a stale read after every retry attempt is exhausted" case the
// item-resolution requirement already specifies HTTP 409 for — the
// earlier version of this test asserted 500, which contradicted that
// contract instead of exercising it.
func TestPatchMealItem_PersistentBusySnapshotReturns409AfterBoundedRetries(t *testing.T) {
	st := newFileFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createUnresolvedMeal(t, st, userID, familyID)
	itemID := meal.Items[0].ID

	const hookName = "test:persistent-busy-snapshot"
	calls := 0
	injecting := false
	st.DB().Callback().Update().Before("gorm:update").Register(hookName, func(*gorm.DB) {
		if injecting {
			// The injected write below is itself an UPDATE, so it would
			// otherwise re-trigger this same hook — guard against that
			// instead of recursing.
			return
		}
		calls++
		injecting = true
		defer func() { injecting = false }()
		// Unlike registerRaceSimulation, this fires on every attempt (no
		// "only once" guard), so every retry's own write also lands after a
		// fresh conflicting commit — the staleness never actually clears.
		if err := st.DB().Model(&database.FoodItem{}).Where("id = ?", itemID).
			Update("name", fmt.Sprintf("Renamed concurrently, attempt %d", calls)).Error; err != nil {
			t.Fatalf("simulate persistent concurrent write: %v", err)
		}
	})
	t.Cleanup(func() { st.DB().Callback().Update().Remove(hookName) })

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	r := itemPatchRequest(meal.ID.String(), itemID.String(), map[string]any{"weight_grams": 250})
	h.PatchMealItem(w, withClaims(r, userID))

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 once retries are exhausted (matches the item-resolution requirement's "+
			"stale-after-every-retry contract), got %d: %s", w.Code, w.Body.String())
	}
	if calls < 2 {
		t.Errorf("expected more than one attempt (retry actually happened), got %d", calls)
	}

	// The exhausted write must not have applied any part of its own change —
	// only the injected concurrent renames should be visible.
	var reloadedItem database.FoodItem
	if err := st.DB().Where("id = ?", itemID).First(&reloadedItem).Error; err != nil {
		t.Fatalf("reload item: %v", err)
	}
	if reloadedItem.WeightGrams == 250 {
		t.Errorf("expected the exhausted request's own weight change not to apply, but it did")
	}
}

func TestPatchMealItem_SupplyMacrosDirectly(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createUnresolvedMeal(t, st, userID, familyID)

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	r := itemPatchRequest(meal.ID.String(), meal.Items[0].ID.String(), map[string]any{
		"manual": true, "calories": 300, "protein_grams": 25,
	})
	h.PatchMealItem(w, withClaims(r, userID))

	var gotMeal database.FoodMeal
	json.NewDecoder(w.Body).Decode(&gotMeal) //nolint:errcheck
	got := gotMeal.Items[0]
	if got.MacroSource != database.MacroSourceManual {
		t.Errorf("expected manual, got %s", got.MacroSource)
	}
	if got.Calories != 300 || got.ProteinGrams != 25 {
		t.Errorf("expected supplied macros to be stored as given, got %+v", got)
	}
}

func TestPatchMealItem_WeightOnlyChangeRescalesFromExistingBinding(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createUnresolvedMeal(t, st, userID, familyID)
	idx := buildUSDAIndex(t, usdaFood(9, "Rice, cooked", 130))

	h := server.NewFoodHandlers(st, idx, t.TempDir())

	// First bind to a reference food at 100g.
	bindReq := itemPatchRequest(meal.ID.String(), meal.Items[0].ID.String(), map[string]any{"fdc_id": 9, "weight_grams": 100})
	w1 := httptest.NewRecorder()
	h.PatchMealItem(w1, withClaims(bindReq, userID))
	if w1.Code != http.StatusOK {
		t.Fatalf("expected 200 on bind, got %d: %s", w1.Code, w1.Body.String())
	}

	// Then change only the weight — macros must rescale from the same profile.
	weightReq := itemPatchRequest(meal.ID.String(), meal.Items[0].ID.String(), map[string]any{"weight_grams": 200})
	w2 := httptest.NewRecorder()
	h.PatchMealItem(w2, withClaims(weightReq, userID))
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 on weight-only change, got %d: %s", w2.Code, w2.Body.String())
	}
	var gotMeal database.FoodMeal
	json.NewDecoder(w2.Body).Decode(&gotMeal) //nolint:errcheck
	got := gotMeal.Items[0]
	if got.MacroSource != database.MacroSourceReference {
		t.Errorf("expected binding to persist across a weight-only change, got %s", got.MacroSource)
	}
	// 130 kcal/100g * 2.0 (200g) = 260
	if got.Calories < 259.9 || got.Calories > 260.1 {
		t.Errorf("expected calories ~260 after rescale, got %v", got.Calories)
	}
}

func TestPatchMealItem_WeightOnlyChangeRescalesFromEstimatedProfile(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createEstimatedItemMeal(t, st, userID, familyID)

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	r := itemPatchRequest(meal.ID.String(), meal.Items[0].ID.String(), map[string]any{"weight_grams": 200})
	w := httptest.NewRecorder()
	h.PatchMealItem(w, withClaims(r, userID))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var gotMeal database.FoodMeal
	json.NewDecoder(w.Body).Decode(&gotMeal) //nolint:errcheck
	got := gotMeal.Items[0]
	if got.MacroSource != database.MacroSourceEstimated {
		t.Errorf("expected macro_source to remain estimated across a weight-only change, got %s", got.MacroSource)
	}
	// 90 kcal/100g * 2.0 (200g) = 180
	if got.Calories < 179.9 || got.Calories > 180.1 {
		t.Errorf("expected calories ~180 after rescale, got %v", got.Calories)
	}
}

func TestPatchMealItem_WeightOnlyRescaleOnEstimatedRequiresPositiveWeight(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createEstimatedItemMeal(t, st, userID, familyID)

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	r := itemPatchRequest(meal.ID.String(), meal.Items[0].ID.String(), map[string]any{"weight_grams": 0})
	w := httptest.NewRecorder()
	h.PatchMealItem(w, withClaims(r, userID))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// A correction that supplies direct macro values, with the save flag set,
// also creates a reusable CustomFood from the item's per-100g-converted
// macros — so a later, differently-worded photo of the same dish can match
// it via the ranked candidate shortlist.
func TestPatchMealItem_ManualWithSaveAsCustomFoodCreatesReusableFood(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createUnresolvedMeal(t, st, userID, familyID) // item "Chicken", weight 100g

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	r := itemPatchRequest(meal.ID.String(), meal.Items[0].ID.String(), map[string]any{
		"manual": true, "save_as_custom_food": true,
		"calories": 300, "protein_grams": 25, "carbs_grams": 10,
	})
	w := httptest.NewRecorder()
	h.PatchMealItem(w, withClaims(r, userID))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var cf database.CustomFood
	if err := st.DB().Where("user_id = ? AND name = ?", userID, "Chicken").First(&cf).Error; err != nil {
		t.Fatalf("expected a CustomFood to be created, query err: %v", err)
	}
	// item weight is 100g, so per-100g equals the supplied totals directly.
	if cf.CaloriesPer100g != 300 || cf.ProteinPer100g != 25 || cf.CarbsPer100g != 10 {
		t.Errorf("expected per-100g values matching the item's macros at its weight, got %+v", cf)
	}
}

// When the item being saved as a reusable food carries a Canonical Name
// (from non-English recognition), the new CustomFood keeps it too. See
// openspec/specs/food-nutrition-logging "A custom food created from a
// non-English recognition keeps its Canonical Name".
func TestPatchMealItem_SaveAsCustomFoodCopiesCanonicalName(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := database.FoodMeal{UserID: userID, Status: database.MealStatusPendingReview, LoggedAt: time.Now()}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}
	item := database.FoodItem{
		UserID: userID, MealID: meal.ID, Name: "вареники", CanonicalName: "dumplings", WeightGrams: 100,
		MacroSource: database.MacroSourceNone,
	}
	item.ID = uuid.New()
	item.FamilyID = familyID
	if err := st.DB().Create(&item).Error; err != nil {
		t.Fatalf("create item: %v", err)
	}

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	r := itemPatchRequest(meal.ID.String(), item.ID.String(), map[string]any{
		"manual": true, "save_as_custom_food": true, "calories": 300,
	})
	w := httptest.NewRecorder()
	h.PatchMealItem(w, withClaims(r, userID))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var cf database.CustomFood
	if err := st.DB().Where("user_id = ? AND name = ?", userID, "вареники").First(&cf).Error; err != nil {
		t.Fatalf("expected a CustomFood to be created, query err: %v", err)
	}
	if cf.CanonicalName != "dumplings" {
		t.Errorf("expected CanonicalName copied from the item, got %q", cf.CanonicalName)
	}
}

// See openspec/specs/food-nutrition-logging "A canonical_name field on the
// request is rejected": Canonical Name is not user-editable through this
// endpoint, no matter what else the request also contains.
func TestPatchMealItem_CanonicalNameFieldRejected(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createUnresolvedMeal(t, st, userID, familyID)

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	r := itemPatchRequest(meal.ID.String(), meal.Items[0].ID.String(), map[string]any{
		"name": "Chicken thigh", "canonical_name": "chicken thigh",
	})
	w := httptest.NewRecorder()
	h.PatchMealItem(w, withClaims(r, userID))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var reloaded database.FoodItem
	if err := st.DB().Where("id = ?", meal.Items[0].ID).First(&reloaded).Error; err != nil {
		t.Fatalf("reload item: %v", err)
	}
	if reloaded.Name != "Chicken" {
		t.Errorf("expected the item unchanged after a rejected request, got name %q", reloaded.Name)
	}
}

// Without the save flag, behavior is exactly as before this change: no
// CustomFood is created as a side effect of a manual correction.
func TestPatchMealItem_ManualWithoutSaveFlagCreatesNoCustomFood(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createUnresolvedMeal(t, st, userID, familyID)

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	r := itemPatchRequest(meal.ID.String(), meal.Items[0].ID.String(), map[string]any{
		"manual": true, "calories": 300,
	})
	w := httptest.NewRecorder()
	h.PatchMealItem(w, withClaims(r, userID))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var count int64
	st.DB().Model(&database.CustomFood{}).Where("user_id = ?", userID).Count(&count)
	if count != 0 {
		t.Errorf("expected no CustomFood created without the save flag, got %d", count)
	}
}

// A genuinely zero-calorie item (water, black coffee) is a valid reusable
// food — save_as_custom_food is already an explicit opt-in checkbox, and the
// standalone POST /api/food/custom endpoint accepts zero-calorie foods too,
// so this path must not impose a stricter rule than that one.
func TestPatchMealItem_SaveAsCustomFoodZeroCaloriesSucceeds(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createUnresolvedMeal(t, st, userID, familyID) // item "Chicken", weight 100g

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	r := itemPatchRequest(meal.ID.String(), meal.Items[0].ID.String(), map[string]any{
		"manual": true, "save_as_custom_food": true,
	})
	w := httptest.NewRecorder()
	h.PatchMealItem(w, withClaims(r, userID))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var cf database.CustomFood
	if err := st.DB().Where("user_id = ? AND name = ?", userID, "Chicken").First(&cf).Error; err != nil {
		t.Fatalf("expected a CustomFood to be created, query err: %v", err)
	}
	if cf.CaloriesPer100g != 0 {
		t.Errorf("expected zero-calorie CustomFood, got %+v", cf)
	}
}

// A name conflict on the save-as-custom-food side must abort the whole
// PATCH atomically — the item correction is not silently half-applied.
func TestPatchMealItem_SaveAsCustomFoodDuplicateNameReturns409(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createUnresolvedMeal(t, st, userID, familyID) // item name "Chicken"

	existing := database.CustomFood{UserID: userID, Name: "Chicken", CaloriesPer100g: 100}
	existing.ID = uuid.New()
	existing.FamilyID = familyID
	if err := st.DB().Create(&existing).Error; err != nil {
		t.Fatalf("create existing custom food: %v", err)
	}

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	r := itemPatchRequest(meal.ID.String(), meal.Items[0].ID.String(), map[string]any{
		"manual": true, "save_as_custom_food": true, "calories": 300,
	})
	w := httptest.NewRecorder()
	h.PatchMealItem(w, withClaims(r, userID))
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}

	var gotItem database.FoodItem
	if err := st.DB().First(&gotItem, "id = ?", meal.Items[0].ID).Error; err != nil {
		t.Fatalf("reload item: %v", err)
	}
	if gotItem.MacroSource != database.MacroSourceNone {
		t.Errorf("expected the item unchanged when the custom-food save conflicts, got %s", gotItem.MacroSource)
	}
}

// Regression: round-6 review — binding a reference food (with an explicit
// weight_grams, or omitting it and relying on the item's existing weight)
// used to accept zero/negative weight, which ApplyProfile then scales the
// profile by, persisting negative or zero item macros and, via
// applyItemMutation's confirmed-meal recompute, a negative meal aggregate.
// CreateMealItem already rejected this; PatchMealItem's bind branch did not.
func TestPatchMealItem_BindToReferenceRequiresPositiveWeight(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	idx := buildUSDAIndex(t, usdaFood(7, "Chicken breast", 165))
	h := server.NewFoodHandlers(st, idx, t.TempDir())

	for _, weight := range []any{0, -5} {
		meal := createUnresolvedMeal(t, st, userID, familyID)
		r := itemPatchRequest(meal.ID.String(), meal.Items[0].ID.String(), map[string]any{
			"fdc_id": 7, "weight_grams": weight,
		})
		w := httptest.NewRecorder()
		h.PatchMealItem(w, withClaims(r, userID))
		if w.Code != http.StatusBadRequest {
			t.Errorf("weight_grams=%v: expected 400, got %d: %s", weight, w.Code, w.Body.String())
		}
	}
}

// Regression: same round-6 finding, but for the weight-only rescale branch
// (PatchMealItem_WeightOnlyChangeRescalesFromExistingBinding's sibling
// path) — changing only the weight on an already-reference-bound item must
// reject non-positive values too, since it re-runs ApplyProfile exactly
// like the initial bind does.
func TestPatchMealItem_WeightOnlyRescaleRequiresPositiveWeight(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createUnresolvedMeal(t, st, userID, familyID)
	idx := buildUSDAIndex(t, usdaFood(9, "Rice, cooked", 130))
	h := server.NewFoodHandlers(st, idx, t.TempDir())

	bindReq := itemPatchRequest(meal.ID.String(), meal.Items[0].ID.String(), map[string]any{"fdc_id": 9, "weight_grams": 100})
	w1 := httptest.NewRecorder()
	h.PatchMealItem(w1, withClaims(bindReq, userID))
	if w1.Code != http.StatusOK {
		t.Fatalf("expected 200 on bind, got %d: %s", w1.Code, w1.Body.String())
	}

	for _, weight := range []any{0, -10} {
		r := itemPatchRequest(meal.ID.String(), meal.Items[0].ID.String(), map[string]any{"weight_grams": weight})
		w := httptest.NewRecorder()
		h.PatchMealItem(w, withClaims(r, userID))
		if w.Code != http.StatusBadRequest {
			t.Errorf("weight_grams=%v: expected 400, got %d: %s", weight, w.Code, w.Body.String())
		}
	}
}

// Regression: editing an item on a confirmed meal is now permitted (it used
// to 409), and the meal's stored aggregate must be recomputed and persisted
// in the same response, not left stale.
func TestPatchMealItem_ConfirmedMealPermittedAndRecomputesAggregate(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createUnresolvedMeal(t, st, userID, familyID)
	if err := st.DB().Model(&database.FoodMeal{}).Where("id = ?", meal.ID).
		Update("status", database.MealStatusConfirmed).Error; err != nil {
		t.Fatalf("confirm meal: %v", err)
	}

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	r := itemPatchRequest(meal.ID.String(), meal.Items[0].ID.String(), map[string]any{
		"manual": true, "calories": 300, "protein_grams": 25,
	})
	h.PatchMealItem(w, withClaims(r, userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var gotMeal database.FoodMeal
	json.NewDecoder(w.Body).Decode(&gotMeal) //nolint:errcheck
	if gotMeal.Status != database.MealStatusConfirmed {
		t.Errorf("expected meal to stay confirmed, got %s", gotMeal.Status)
	}
	if gotMeal.Calories != 300 || gotMeal.ProteinGrams != 25 {
		t.Errorf("expected the meal's stored aggregate to reflect the edit, got %+v", gotMeal)
	}

	// Persisted, not just returned in the response.
	var reloaded database.FoodMeal
	if err := st.DB().Where("id = ?", meal.ID).First(&reloaded).Error; err != nil {
		t.Fatalf("reload meal: %v", err)
	}
	if reloaded.Calories != 300 {
		t.Errorf("expected persisted aggregate to be 300, got %v", reloaded.Calories)
	}
}

// Regression: applyItemMutation's core guarantee is that the item write and
// the confirmed-meal aggregate recompute commit or roll back together — a
// failure in either half must not leave the other applied. This forces the
// aggregate half specifically to fail (via a GORM Before-update hook that
// aborts the second Update in the transaction, which is always the
// aggregate write) and checks the item write it was paired with was rolled
// back too, not left applied with a stale aggregate.
func TestPatchMealItem_AggregateWriteFailureRollsBackItemChange(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := createUnresolvedMeal(t, st, userID, familyID)
	originalWeight := meal.Items[0].WeightGrams
	if err := st.DB().Model(&database.FoodMeal{}).Where("id = ?", meal.ID).
		Update("status", database.MealStatusConfirmed).Error; err != nil {
		t.Fatalf("confirm meal: %v", err)
	}

	const hookName = "test:fail-aggregate-write"
	updateCalls := 0
	st.DB().Callback().Update().Before("gorm:update").Register(hookName, func(tx *gorm.DB) {
		updateCalls++
		// Call 1 is the item Save; call 2 is the aggregate recompute — see
		// applyItemMutation's transaction body.
		if updateCalls == 2 {
			tx.Error = errors.New("simulated aggregate write failure")
		}
	})
	t.Cleanup(func() { st.DB().Callback().Update().Remove(hookName) })

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	r := itemPatchRequest(meal.ID.String(), meal.Items[0].ID.String(), map[string]any{
		"weight_grams": originalWeight + 500,
	})
	h.PatchMealItem(w, withClaims(r, userID))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when the aggregate write fails, got %d: %s", w.Code, w.Body.String())
	}

	var reloadedItem database.FoodItem
	if err := st.DB().Where("id = ?", meal.Items[0].ID).First(&reloadedItem).Error; err != nil {
		t.Fatalf("reload item: %v", err)
	}
	if reloadedItem.WeightGrams != originalWeight {
		t.Errorf("expected item weight to roll back to %v (unchanged), got %v", originalWeight, reloadedItem.WeightGrams)
	}
}

func TestPatchMealItem_NonEditableStatusesReturn409(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	for _, status := range []string{
		database.MealStatusProcessing, database.MealStatusPendingClarification, database.MealStatusFailed,
	} {
		meal := createUnresolvedMeal(t, st, userID, familyID)
		if err := st.DB().Model(&database.FoodMeal{}).Where("id = ?", meal.ID).
			Update("status", status).Error; err != nil {
			t.Fatalf("set status %s: %v", status, err)
		}

		h := server.NewFoodHandlers(st, nil, t.TempDir())
		w := httptest.NewRecorder()
		r := itemPatchRequest(meal.ID.String(), meal.Items[0].ID.String(), map[string]any{"manual": true, "calories": 1})
		h.PatchMealItem(w, withClaims(r, userID))

		if w.Code != http.StatusConflict {
			t.Errorf("status=%s: expected 409, got %d: %s", status, w.Code, w.Body.String())
		}
	}
}

func TestPatchMealItem_CrossUserReturns404(t *testing.T) {
	st := newFoodTestStorage(t)
	_, familyID := seedFoodUser(t, st)
	otherUserID := uuid.New()
	meal := createUnresolvedMeal(t, st, otherUserID, familyID)

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	r := itemPatchRequest(meal.ID.String(), meal.Items[0].ID.String(), map[string]any{"manual": true, "calories": 1})
	h.PatchMealItem(w, withClaims(r, uuid.New()))

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// --- RetryMeal ---

func TestRetryMeal_FailedMealIsAccepted(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	dir := t.TempDir()

	meal := createFailedMealWithPhoto(t, st, dir, userID, familyID)
	fake := &vision.Fake{
		RecognizeResult: &vision.RecognizeResult{Items: []vision.Item{{Name: "Rice", WeightGrams: 100}}},
	}
	h := server.NewFoodHandlers(st, nil, dir).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.RetryMeal(w, withClaims(mealDetailRequest(http.MethodPost, meal.ID.String()), userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got database.FoodMeal
	json.NewDecoder(w.Body).Decode(&got) //nolint:errcheck
	if got.Status != database.MealStatusPendingReview {
		t.Errorf("expected pending_review after retry, got %s", got.Status)
	}
	if len(fake.RecognizeCalls) != 1 {
		t.Errorf("expected exactly one Recognize call, got %d", len(fake.RecognizeCalls))
	}
}

func TestRetryMeal_LiveProcessingRejectedWith409(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	meal := database.FoodMeal{UserID: userID, Status: database.MealStatusProcessing, LoggedAt: time.Now(), PhotoPath: "x/y.jpg"}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}

	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(&vision.Fake{}, 10<<20, time.Minute)
	w := httptest.NewRecorder()
	h.RetryMeal(w, withClaims(mealDetailRequest(http.MethodPost, meal.ID.String()), userID))

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409 for a live in-flight analysis, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRetryMeal_StaleProcessingIsAccepted(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	dir := t.TempDir()

	meal := createFailedMealWithPhoto(t, st, dir, userID, familyID)
	if err := st.DB().Model(&database.FoodMeal{}).Where("id = ?", meal.ID).
		Update("status", database.MealStatusProcessing).Error; err != nil {
		t.Fatalf("set processing: %v", err)
	}
	// Backdate updated_at past the (very short) vision timeout used below.
	past := time.Now().Add(-time.Hour)
	if err := st.DB().Model(&database.FoodMeal{}).Where("id = ?", meal.ID).
		Update("updated_at", past).Error; err != nil {
		t.Fatalf("backdate updated_at: %v", err)
	}

	fake := &vision.Fake{RecognizeResult: &vision.RecognizeResult{}}
	h := server.NewFoodHandlers(st, nil, dir).WithVision(fake, 10<<20, time.Second)
	w := httptest.NewRecorder()
	h.RetryMeal(w, withClaims(mealDetailRequest(http.MethodPost, meal.ID.String()), userID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for stale processing, got %d: %s", w.Code, w.Body.String())
	}
}

// Regression: RetryMeal's claim used to be an unconditional `WHERE id = ?`
// write once the eligibility check (read, then act) passed — a classic
// TOCTOU gap. Two concurrent requests (a double-click, or a race with
// Reanalyze's own stale-processing-adjacent claim) observing the same
// eligible meal could both pass that check and both restart analysis. The
// claim is now conditioned on (status, updated_at) exactly matching what was
// just read, so a concurrent claim landing first invalidates this one's.
func TestRetryMeal_ConcurrentClaimIsRejected(t *testing.T) {
	st := newFileFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	dir := t.TempDir()
	meal := createFailedMealWithPhoto(t, st, dir, userID, familyID)

	registerRaceSimulation(t, st.DB(), func() {
		sqlDB, err := st.DB().DB()
		if err != nil {
			t.Fatalf("get sql.DB: %v", err)
		}
		if _, err := sqlDB.Exec(
			"UPDATE food_meals SET status = ?, updated_at = ? WHERE id = ?",
			database.MealStatusProcessing, time.Now().UTC(), meal.ID,
		); err != nil {
			t.Fatalf("simulate concurrent claim: %v", err)
		}
	})

	fake := &vision.Fake{RecognizeResult: &vision.RecognizeResult{}}
	h := server.NewFoodHandlers(st, nil, dir).WithVision(fake, 10<<20, time.Second)
	w := httptest.NewRecorder()
	h.RetryMeal(w, withClaims(mealDetailRequest(http.MethodPost, meal.ID.String()), userID))

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 when the meal was concurrently claimed first, got %d: %s", w.Code, w.Body.String())
	}
	if len(fake.RecognizeCalls) != 0 {
		t.Errorf("expected no vision call once the claim failed, got %d", len(fake.RecognizeCalls))
	}
}

// Regression: if a newer attempt (e.g. a concurrent Reanalyze) claims and
// completes against this meal in the window between RetryMeal's own claim
// and persistAnalysis's write, that write's lease check correctly rolls
// back — but RetryMeal used to respond with its own local `meal` struct
// regardless, which still shows status=processing (whatever its own claim
// wrote, never updated since its write never actually landed). The caller
// would see a stale, misleading response instead of what's actually stored.
//
// Uses the same gated-vision-client technique as
// TestReanalyze_ConcurrentCallsOnlyOneProceeds (real goroutine + channel,
// not a GORM hook): the "concurrent" write happens as a plain top-level
// UPDATE while RetryMeal's own goroutine is parked inside Recognize, i.e.
// strictly *before* persistAnalysis's transaction begins — a GORM
// Before-update hook won't work here, since injecting a nested write from
// inside persistAnalysis's own active transaction deadlocks against
// SQLite's single-writer lock instead of racing it.
func TestRetryMeal_RespondsWithCurrentStateWhenSuperseded(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	dir := t.TempDir()
	meal := createFailedMealWithPhoto(t, st, dir, userID, familyID)

	client := &gatedRecognizeClient{
		entered: make(chan struct{}),
		proceed: make(chan struct{}),
		result:  &vision.RecognizeResult{Items: []vision.Item{{Name: "Rice", WeightGrams: 100}}},
	}
	h := server.NewFoodHandlers(st, nil, dir).WithVision(client, 10<<20, time.Minute)

	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		w := httptest.NewRecorder()
		h.RetryMeal(w, withClaims(mealDetailRequest(http.MethodPost, meal.ID.String()), userID))
		done <- w
	}()

	<-client.entered // RetryMeal's own claim has landed; it's now blocked in Recognize.

	// Simulate a newer attempt (e.g. a concurrent Reanalyze) claiming and
	// completing against this same meal while RetryMeal's own call is still
	// in flight. A plain top-level UPDATE, not nested in any transaction.
	if err := st.DB().Model(&database.FoodMeal{}).Where("id = ?", meal.ID).
		Updates(map[string]any{
			"status":     database.MealStatusPendingReview,
			"updated_at": time.Now().UTC().Add(time.Second),
		}).Error; err != nil {
		t.Fatalf("simulate concurrent supersession: %v", err)
	}

	close(client.proceed)
	w := <-done

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got database.FoodMeal
	json.NewDecoder(w.Body).Decode(&got) //nolint:errcheck
	if got.Status != database.MealStatusPendingReview {
		t.Errorf(
			"expected the response to reflect the current superseding state (pending_review), got %s — a stale response would show processing",
			got.Status,
		)
	}
}

// Regression: round-6 review — when applied is false, reloadIfSuperseded
// used to fall back to the known-stale in-memory meal if the reload itself
// failed, so a caller mid-flight when a *newer* attempt deletes the meal
// entirely got back 200 with a ghost meal (still showing "processing")
// instead of a 404 reflecting that it's actually gone. Same gated-vision
// technique as the test above, but the "concurrent" write is a delete, not a
// supersession, and the meal is expected to be reported not found rather
// than returned stale.
func TestRetryMeal_RespondsWith404WhenSupersedingOperationDeletesTheMeal(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	dir := t.TempDir()
	meal := createFailedMealWithPhoto(t, st, dir, userID, familyID)

	client := &gatedRecognizeClient{
		entered: make(chan struct{}),
		proceed: make(chan struct{}),
		result:  &vision.RecognizeResult{Items: []vision.Item{{Name: "Rice", WeightGrams: 100}}},
	}
	h := server.NewFoodHandlers(st, nil, dir).WithVision(client, 10<<20, time.Minute)

	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		w := httptest.NewRecorder()
		h.RetryMeal(w, withClaims(mealDetailRequest(http.MethodPost, meal.ID.String()), userID))
		done <- w
	}()

	<-client.entered // RetryMeal's own claim has landed; it's now blocked in Recognize.

	// Simulate a newer attempt deleting the meal entirely while RetryMeal's
	// own call is still in flight — a plain top-level (soft) delete, not
	// nested in any transaction.
	if err := st.DB().Delete(&database.FoodMeal{}, "id = ?", meal.ID).Error; err != nil {
		t.Fatalf("simulate concurrent delete: %v", err)
	}

	close(client.proceed)
	w := <-done

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 once the meal is actually gone, got %d: %s — a stale fallback would return 200", w.Code, w.Body.String())
	}
}

func TestRetryMeal_ConfirmedMealRejectedWith409(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	meal := database.FoodMeal{UserID: userID, Status: database.MealStatusConfirmed, LoggedAt: time.Now(), PhotoPath: "x/y.jpg"}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.RetryMeal(w, withClaims(mealDetailRequest(http.MethodPost, meal.ID.String()), userID))

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestRetryMeal_ReplacesItemsNotAppends(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	dir := t.TempDir()
	meal := createFailedMealWithPhoto(t, st, dir, userID, familyID)

	// Seed a leftover item from a hypothetical earlier partial run.
	leftover := database.FoodItem{UserID: userID, MealID: meal.ID, Name: "Stale", MacroSource: database.MacroSourceNone}
	leftover.ID = uuid.New()
	leftover.FamilyID = familyID
	if err := st.DB().Create(&leftover).Error; err != nil {
		t.Fatalf("create leftover item: %v", err)
	}

	fake := &vision.Fake{
		RecognizeResult: &vision.RecognizeResult{Items: []vision.Item{{Name: "Fresh", WeightGrams: 50}}},
	}
	h := server.NewFoodHandlers(st, nil, dir).WithVision(fake, 10<<20, time.Second)
	w := httptest.NewRecorder()
	h.RetryMeal(w, withClaims(mealDetailRequest(http.MethodPost, meal.ID.String()), userID))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var items []database.FoodItem
	st.DB().Where("meal_id = ?", meal.ID).Find(&items) //nolint:errcheck
	if len(items) != 1 || items[0].Name != "Fresh" {
		t.Errorf("expected exactly the new item set, got %+v", items)
	}

	// Unscoped: a plain Find on FoodItem (which embeds TenantModel) silently
	// applies the same soft-delete filter as the bug this test needs to
	// catch, so it can't tell "hard-deleted" from "soft-deleted and hidden"
	// apart — exactly like TestDeleteCustomFood_Success couldn't, before
	// persistAnalysis was fixed to Unscoped().Delete().
	var allRowsEver int64
	st.DB().Unscoped().Model(&database.FoodItem{}).Where("meal_id = ?", meal.ID).Count(&allRowsEver)
	if allRowsEver != 1 {
		t.Errorf("expected the leftover row to be hard-deleted (1 row total), got %d rows including soft-deleted", allRowsEver)
	}
}

func TestRetryMeal_CrossUserReturns404(t *testing.T) {
	st := newFoodTestStorage(t)
	_, familyID := seedFoodUser(t, st)
	otherUserID := uuid.New()
	meal := database.FoodMeal{UserID: otherUserID, Status: database.MealStatusFailed, LoggedAt: time.Now(), PhotoPath: "x/y.jpg"}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}

	h := server.NewFoodHandlers(st, nil, t.TempDir())
	w := httptest.NewRecorder()
	h.RetryMeal(w, withClaims(mealDetailRequest(http.MethodPost, meal.ID.String()), uuid.New()))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// createFailedMealWithPhoto creates a failed meal with a real stored photo
// (so RetryMeal's photos.Read succeeds), via the same photostorage.Store the
// handler under test will use.
func createFailedMealWithPhoto(t *testing.T, st database.Storage, dir string, userID, familyID uuid.UUID) database.FoodMeal {
	t.Helper()
	photos := photostorage.New(dir)
	mealID := uuid.New()
	relPath, err := photos.Save(bytes.NewReader(fakeJPEGBytes), 1<<20, userID, photostorage.OwnerMeal, mealID)
	if err != nil {
		t.Fatalf("save photo: %v", err)
	}
	meal := database.FoodMeal{UserID: userID, Status: database.MealStatusFailed, LoggedAt: time.Now(), PhotoPath: relPath}
	meal.ID = mealID
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}
	return meal
}
