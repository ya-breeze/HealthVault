package server_test

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/off"
	"github.com/ya-breeze/healthvault/pkg/server"
	"github.com/ya-breeze/healthvault/pkg/usda"
	kinmodels "github.com/ya-breeze/kin-core/models"
)

func newFoodTestStorage(t *testing.T) database.Storage {
	t.Helper()
	db, err := database.Open(slog.New(slog.NewTextHandler(os.Stderr, nil)), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return database.NewStorage(db)
}

func seedFoodUser(t *testing.T, s database.Storage) (userID, familyID uuid.UUID) {
	t.Helper()
	familyID = uuid.New()
	userID = uuid.New()
	if err := s.DB().Create(&kinmodels.Family{ID: familyID, Name: "TestFamily"}).Error; err != nil {
		t.Fatalf("create family: %v", err)
	}
	user := kinmodels.User{ID: userID, Username: "testuser", PasswordHash: "x", FamilyID: familyID}
	if err := s.DB().Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return userID, familyID
}

func usdaFood(id int64, desc string, kcal float64) usda.Food {
	return usda.Food{
		FdcID: id, Description: desc, DataType: "sr_legacy_food",
		Profile: database.NutrientProfile{CaloriesPer100g: kcal, ProteinPer100g: 31},
	}
}

// buildUSDAIndex builds and promotes a real USDA database with the given
// foods plus enough filler rows to clear MinExpectedRows, returning an open
// read-only Index.
func buildUSDAIndex(t *testing.T, foods ...usda.Food) *usda.Index {
	t.Helper()
	target := filepath.Join(t.TempDir(), "usda.db")
	b, err := usda.NewBuilder(target)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	for _, f := range foods {
		if err := b.Add(f); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	for i := range usda.MinExpectedRows {
		if err := b.Add(usdaFood(int64(900000+i), "Filler food item", 1)); err != nil {
			t.Fatalf("Add filler: %v", err)
		}
	}
	if _, err := b.Promote(); err != nil {
		t.Fatalf("Promote: %v", err)
	}
	idx, err := usda.Open(target)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { idx.Close() }) //nolint:errcheck
	return idx
}

func offFood(code, name, brands string, kcal float64) off.Food {
	return off.Food{
		Code: code, ProductName: name, Brands: brands,
		Profile: database.NutrientProfile{CaloriesPer100g: kcal, ProteinPer100g: 5},
	}
}

// buildOFFIndex builds and promotes a real Open Food Facts database with the
// given products plus enough filler rows to clear MinExpectedRows, returning
// an open read-only Index. Mirrors buildUSDAIndex.
func buildOFFIndex(t *testing.T, products ...off.Food) *off.Index {
	t.Helper()
	target := filepath.Join(t.TempDir(), "off.db")
	b, err := off.NewBuilder(target)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	for _, p := range products {
		if err := b.Add(p); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	for i := range off.MinExpectedRows {
		if err := b.Add(offFood(fmt.Sprintf("filler-%d", i), "Filler product", "Filler Brand", 1)); err != nil {
			t.Fatalf("Add filler: %v", err)
		}
	}
	if _, err := b.Promote(); err != nil {
		t.Fatalf("Promote: %v", err)
	}
	idx, err := off.Open(target)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { idx.Close() }) //nolint:errcheck
	return idx
}

func newSearchRequest(userID uuid.UUID, q string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/food/search?q="+q, nil)
	return withClaims(r, userID)
}

func TestFoodSearch_CustomFoodExactMatchWinsOutright(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	custom := database.CustomFood{
		UserID: userID, Name: "Overnight Oats",
		CaloriesPer100g: 150, ProteinPer100g: 5, CarbsPer100g: 25, FatPer100g: 3,
	}
	custom.ID = uuid.New()
	custom.FamilyID = familyID
	if err := st.DB().Create(&custom).Error; err != nil {
		t.Fatalf("create custom food: %v", err)
	}

	idx := buildUSDAIndex(t, usdaFood(1, "Oats, raw", 389))
	h := server.NewFoodHandlers(st, idx, t.TempDir())

	w := httptest.NewRecorder()
	// Case-insensitive match against a differently-cased query.
	h.Search(w, newSearchRequest(userID, "overnight+oats"))

	var resp server.FoodSearchResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Results) != 1 || resp.Results[0].Source != "custom" {
		t.Fatalf("expected exactly one custom result, got %+v", resp.Results)
	}
	if resp.Results[0].CustomFoodID == nil || *resp.Results[0].CustomFoodID != custom.ID {
		t.Errorf("expected custom_food_id %s, got %+v", custom.ID, resp.Results[0].CustomFoodID)
	}
}

func TestFoodSearch_FallsThroughToUSDAWhenNoCustomMatch(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	idx := buildUSDAIndex(t, usdaFood(1, "Chicken, broilers or fryers, breast, meat only, cooked, roasted", 165))
	h := server.NewFoodHandlers(st, idx, t.TempDir())

	w := httptest.NewRecorder()
	h.Search(w, newSearchRequest(userID, "chicken+breast"))

	var resp server.FoodSearchResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.USDAUnavailable {
		t.Fatalf("expected USDA available, got unavailable")
	}
	found := false
	for _, r := range resp.Results {
		if r.Source == "usda" && r.FdcID != nil && *r.FdcID == 1 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected fdc_id 1 among results, got %+v", resp.Results)
	}
}

func TestFoodSearch_CrossUserCustomFoodIsolation(t *testing.T) {
	st := newFoodTestStorage(t)
	_, familyID := seedFoodUser(t, st)
	otherUserID := uuid.New()

	custom := database.CustomFood{
		UserID: otherUserID, Name: "Secret Recipe",
		CaloriesPer100g: 100, ProteinPer100g: 1, CarbsPer100g: 1, FatPer100g: 1,
	}
	custom.ID = uuid.New()
	custom.FamilyID = familyID
	if err := st.DB().Create(&custom).Error; err != nil {
		t.Fatalf("create custom food: %v", err)
	}

	idx := buildUSDAIndex(t, usdaFood(1, "Filler", 1))
	h := server.NewFoodHandlers(st, idx, t.TempDir())

	callerID := uuid.New()
	w := httptest.NewRecorder()
	h.Search(w, newSearchRequest(callerID, "secret+recipe"))

	var resp server.FoodSearchResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, r := range resp.Results {
		if r.Source == "custom" {
			t.Fatalf("expected no custom result for another user's food, got %+v", resp.Results)
		}
	}
}

func TestFoodSearch_BeforeFirstImportReturnsEmptyAndUnavailable(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	h := server.NewFoodHandlers(st, nil, t.TempDir())

	w := httptest.NewRecorder()
	h.Search(w, newSearchRequest(userID, "chicken"))

	var resp server.FoodSearchResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.USDAUnavailable {
		t.Errorf("expected usda_unavailable=true before any import")
	}
	if len(resp.Results) != 0 {
		t.Errorf("expected no results, got %+v", resp.Results)
	}
}

func TestFoodSearch_Unauthenticated(t *testing.T) {
	st := newFoodTestStorage(t)
	h := server.NewFoodHandlers(st, nil, t.TempDir())

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/food/search?q=chicken", nil)
	h.Search(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}
