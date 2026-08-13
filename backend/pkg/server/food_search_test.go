package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/off"
	"github.com/ya-breeze/healthvault/pkg/server"
	"github.com/ya-breeze/healthvault/pkg/usda"
	"github.com/ya-breeze/healthvault/pkg/vision"
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

func newSearchRequestRefresh(userID uuid.UUID, q string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/food/search?q="+q+"&refresh=true", nil)
	return withClaims(r, userID)
}

func decodeSearchResponse(t *testing.T, w *httptest.ResponseRecorder) server.FoodSearchResponse {
	t.Helper()
	var resp server.FoodSearchResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp
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

// 4.1: a cache hit must skip translation entirely — the fake client errors
// if Translate is ever called, so this fails loudly if the cache is bypassed.
func TestFoodSearch_CacheHitSkipsTranslation(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	row := database.FoodSearchTranslation{UserID: userID, OriginalQuery: "porridge", TranslatedQuery: "oatmeal"}
	row.ID = uuid.New()
	row.FamilyID = familyID
	if err := st.DB().Create(&row).Error; err != nil {
		t.Fatalf("seed cache row: %v", err)
	}

	idx := buildUSDAIndex(t, usdaFood(1, "Oatmeal, raw", 389))
	fake := &vision.Fake{TranslateErr: errors.New("Translate must not be called on a cache hit")}
	h := server.NewFoodHandlers(st, idx, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.Search(w, newSearchRequest(userID, "porridge"))

	if len(fake.TranslateCalls) != 0 {
		t.Fatalf("expected Translate not to be called, got %d calls", len(fake.TranslateCalls))
	}
	resp := decodeSearchResponse(t, w)
	if resp.TranslatedQuery != "oatmeal" {
		t.Errorf("expected translated_query %q, got %q", "oatmeal", resp.TranslatedQuery)
	}
	found := false
	for _, r := range resp.Results {
		if r.FdcID != nil && *r.FdcID == 1 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected the cached translation's USDA match among results, got %+v", resp.Results)
	}
}

// 4.2: a cache miss calls Translate, persists the result, and uses it.
func TestFoodSearch_CacheMissTranslatesAndPersists(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	idx := buildUSDAIndex(t, usdaFood(1, "Oatmeal, raw", 389))
	fake := &vision.Fake{TranslateResult: "oatmeal"}
	h := server.NewFoodHandlers(st, idx, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.Search(w, newSearchRequest(userID, "porridge"))

	if len(fake.TranslateCalls) != 1 || fake.TranslateCalls[0] != "porridge" {
		t.Fatalf("expected exactly one Translate call with the literal query, got %+v", fake.TranslateCalls)
	}
	var row database.FoodSearchTranslation
	if err := st.DB().Where("user_id = ? AND original_query = ?", userID, "porridge").First(&row).Error; err != nil {
		t.Fatalf("expected a persisted cache row: %v", err)
	}
	if row.TranslatedQuery != "oatmeal" {
		t.Errorf("expected cached translated_query %q, got %q", "oatmeal", row.TranslatedQuery)
	}
	resp := decodeSearchResponse(t, w)
	if resp.TranslatedQuery != "oatmeal" {
		t.Errorf("expected translated_query %q, got %q", "oatmeal", resp.TranslatedQuery)
	}
}

// 4.3: refresh=true calls Translate again and overwrites an existing row,
// even though a cache entry already exists.
func TestFoodSearch_RefreshOverwritesExistingMapping(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	row := database.FoodSearchTranslation{UserID: userID, OriginalQuery: "овсянка", TranslatedQuery: "wrong term"}
	row.ID = uuid.New()
	row.FamilyID = familyID
	if err := st.DB().Create(&row).Error; err != nil {
		t.Fatalf("seed cache row: %v", err)
	}

	idx := buildUSDAIndex(t, usdaFood(1, "Oatmeal, raw", 389))
	fake := &vision.Fake{TranslateResult: "oatmeal"}
	h := server.NewFoodHandlers(st, idx, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.Search(w, newSearchRequestRefresh(userID, "овсянка"))

	if len(fake.TranslateCalls) != 1 {
		t.Fatalf("expected Translate to be called despite an existing cache entry, got %d calls", len(fake.TranslateCalls))
	}
	var updated database.FoodSearchTranslation
	if err := st.DB().Where("user_id = ? AND original_query = ?", userID, "овсянка").First(&updated).Error; err != nil {
		t.Fatalf("expected the row to still exist: %v", err)
	}
	if updated.TranslatedQuery != "oatmeal" {
		t.Errorf("expected the mapping overwritten to %q, got %q", "oatmeal", updated.TranslatedQuery)
	}
	if updated.ID != row.ID {
		t.Errorf("expected the same row upserted in place, got a new row (old id %s, new id %s)", row.ID, updated.ID)
	}
}

// 4.4: a Translate failure falls back to the literal query, writes nothing
// to the cache, and does not surface an error to the caller.
func TestFoodSearch_TranslateFailureFallsBackToLiteralQuery(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	idx := buildUSDAIndex(t, usdaFood(1, "Chicken breast, cooked", 165))
	fake := &vision.Fake{TranslateErr: errors.New("provider unavailable")}
	h := server.NewFoodHandlers(st, idx, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.Search(w, newSearchRequest(userID, "chicken+breast"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 despite translation failure, got %d", w.Code)
	}
	resp := decodeSearchResponse(t, w)
	if resp.TranslatedQuery != "" {
		t.Errorf("expected no translated_query on failure, got %q", resp.TranslatedQuery)
	}
	found := false
	for _, r := range resp.Results {
		if r.FdcID != nil && *r.FdcID == 1 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected literal-query USDA results despite translation failure, got %+v", resp.Results)
	}
	var count int64
	st.DB().Model(&database.FoodSearchTranslation{}).Where("user_id = ?", userID).Count(&count)
	if count != 0 {
		t.Errorf("expected no cache row written on translation failure, found %d", count)
	}
}

// 4.5: an unconfigured vision client (today's baseline — no WithVision call)
// behaves exactly as before this change: literal-only search, no
// translated_query in the response.
func TestFoodSearch_UnconfiguredVisionOmitsTranslatedQuery(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	idx := buildUSDAIndex(t, usdaFood(1, "Chicken breast, cooked", 165))
	h := server.NewFoodHandlers(st, idx, t.TempDir())

	w := httptest.NewRecorder()
	h.Search(w, newSearchRequest(userID, "chicken+breast"))

	resp := decodeSearchResponse(t, w)
	if resp.TranslatedQuery != "" {
		t.Errorf("expected no translated_query when vision is unconfigured, got %q", resp.TranslatedQuery)
	}
}

// 4.6: two users searching the same query text each get their own cache row
// — a translation cached for one user is never reused for another.
func TestFoodSearch_CacheRowsArePerUser(t *testing.T) {
	st := newFoodTestStorage(t)
	userA, familyID := seedFoodUser(t, st)
	userB := uuid.New()
	if err := st.DB().Create(&kinmodels.User{ID: userB, Username: "userb", PasswordHash: "x", FamilyID: familyID}).Error; err != nil {
		t.Fatalf("create second user: %v", err)
	}

	idx := buildUSDAIndex(t, usdaFood(1, "Oatmeal, raw", 389))
	fakeA := &vision.Fake{TranslateResult: "oatmeal"}
	hA := server.NewFoodHandlers(st, idx, t.TempDir()).WithVision(fakeA, 10<<20, time.Second)
	hA.Search(httptest.NewRecorder(), newSearchRequest(userA, "porridge"))

	fakeB := &vision.Fake{TranslateResult: "oatmeal"}
	hB := server.NewFoodHandlers(st, idx, t.TempDir()).WithVision(fakeB, 10<<20, time.Second)
	hB.Search(httptest.NewRecorder(), newSearchRequest(userB, "porridge"))

	var count int64
	st.DB().Model(&database.FoodSearchTranslation{}).Where("original_query = ?", "porridge").Count(&count)
	if count != 2 {
		t.Fatalf("expected 2 distinct per-user cache rows, found %d", count)
	}
	if len(fakeA.TranslateCalls) != 1 || len(fakeB.TranslateCalls) != 1 {
		t.Errorf("expected each user's first search to call Translate independently, got A=%d B=%d",
			len(fakeA.TranslateCalls), len(fakeB.TranslateCalls))
	}
}

// 4.7: a Translate call that blocks past the vision timeout must not hang
// the search request — it's treated as a failure and the request falls
// back to the literal query within the deadline.
func TestFoodSearch_TranslateTimeoutFallsBackWithinDeadline(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	idx := buildUSDAIndex(t, usdaFood(1, "Chicken breast, cooked", 165))
	h := server.NewFoodHandlers(st, idx, t.TempDir()).WithVision(slowRecognizeClient{}, 10<<20, 20*time.Millisecond)

	done := make(chan struct{})
	w := httptest.NewRecorder()
	go func() {
		h.Search(w, newSearchRequest(userID, "chicken+breast"))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Search did not return within a reasonable bound after a stalled Translate call")
	}

	resp := decodeSearchResponse(t, w)
	if resp.TranslatedQuery != "" {
		t.Errorf("expected no translated_query after a timeout, got %q", resp.TranslatedQuery)
	}
	found := false
	for _, r := range resp.Results {
		if r.FdcID != nil && *r.FdcID == 1 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected literal-query USDA results after a translation timeout, got %+v", resp.Results)
	}
}

// 4.8: translation is never attempted when USDA itself is unavailable —
// there is nowhere to use a translated term.
func TestFoodSearch_USDAUnavailableSkipsTranslation(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	fake := &vision.Fake{TranslateErr: errors.New("Translate must not be called when USDA is unavailable")}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.Search(w, newSearchRequest(userID, "porridge"))

	if len(fake.TranslateCalls) != 0 {
		t.Fatalf("expected Translate not to be called when USDA is unavailable, got %d calls", len(fake.TranslateCalls))
	}
	resp := decodeSearchResponse(t, w)
	if !resp.USDAUnavailable {
		t.Errorf("expected usda_unavailable=true")
	}
}

// 4.9: when the translated term equals the (normalized) literal query, the
// mapping is still cached, but translated_query is omitted from the
// response — the common case for an English speaker's first search.
func TestFoodSearch_TranslationEqualToInputOmitsTranslatedQuery(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	idx := buildUSDAIndex(t, usdaFood(1, "Chicken breast, cooked", 165))
	fake := &vision.Fake{TranslateResult: "chicken breast"}
	h := server.NewFoodHandlers(st, idx, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.Search(w, newSearchRequest(userID, "chicken+breast"))

	resp := decodeSearchResponse(t, w)
	if resp.TranslatedQuery != "" {
		t.Errorf("expected translated_query omitted when it matches the input, got %q", resp.TranslatedQuery)
	}
	var row database.FoodSearchTranslation
	if err := st.DB().Where("user_id = ? AND original_query = ?", userID, "chicken breast").First(&row).Error; err != nil {
		t.Fatalf("expected the mapping to still be cached: %v", err)
	}
	if row.TranslatedQuery != "chicken breast" {
		t.Errorf("expected the cached term %q, got %q", "chicken breast", row.TranslatedQuery)
	}
}

// 4.10: a failed refresh leaves the prior cached row untouched — the
// upsert simply never runs, so the old (possibly still-correct) mapping
// stays available for the next plain search.
func TestFoodSearch_FailedRefreshLeavesPriorMappingUnchanged(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	row := database.FoodSearchTranslation{UserID: userID, OriginalQuery: "porridge", TranslatedQuery: "oatmeal"}
	row.ID = uuid.New()
	row.FamilyID = familyID
	if err := st.DB().Create(&row).Error; err != nil {
		t.Fatalf("seed cache row: %v", err)
	}

	idx := buildUSDAIndex(t, usdaFood(1, "Oatmeal, raw", 389))
	fake := &vision.Fake{TranslateErr: errors.New("refresh translation failed")}
	h := server.NewFoodHandlers(st, idx, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.Search(w, newSearchRequestRefresh(userID, "porridge"))

	resp := decodeSearchResponse(t, w)
	if resp.TranslatedQuery != "" {
		t.Errorf("expected no translated_query when the refresh itself failed, got %q", resp.TranslatedQuery)
	}
	var unchanged database.FoodSearchTranslation
	if err := st.DB().Where("user_id = ? AND original_query = ?", userID, "porridge").First(&unchanged).Error; err != nil {
		t.Fatalf("expected the prior row to still exist: %v", err)
	}
	if unchanged.TranslatedQuery != "oatmeal" {
		t.Errorf("expected the prior mapping %q left unchanged, got %q", "oatmeal", unchanged.TranslatedQuery)
	}
	if unchanged.ID != row.ID {
		t.Errorf("expected the same row, got a different one (old id %s, new id %s)", row.ID, unchanged.ID)
	}
}

// dbClosingTranslateClient wraps vision.Fake and closes the storage
// connection right after a successful Translate call, deterministically
// forcing the subsequent cache-write upsert to fail — used to test that a
// failed cache write is treated as a failed translation, not a successful
// one that merely didn't persist.
type dbClosingTranslateClient struct {
	vision.Fake
	storage database.Storage
}

func (c *dbClosingTranslateClient) Translate(ctx context.Context, query string) (string, error) {
	result, err := c.Fake.Translate(ctx, query)
	if err == nil {
		if sqlDB, dbErr := c.storage.DB().DB(); dbErr == nil {
			sqlDB.Close()
		}
	}
	return result, err
}

// A translation that succeeds but whose cache write fails must not be
// reported as applied: the response falls back to the literal query exactly
// as if Translate itself had failed.
func TestFoodSearch_CacheWriteFailureFallsBackToLiteralQuery(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	idx := buildUSDAIndex(t, usdaFood(1, "Chicken breast, cooked", 165))
	fake := &dbClosingTranslateClient{Fake: vision.Fake{TranslateResult: "chicken"}, storage: st}
	h := server.NewFoodHandlers(st, idx, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.Search(w, newSearchRequest(userID, "chicken+breast"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 despite a cache-write failure, got %d", w.Code)
	}
	resp := decodeSearchResponse(t, w)
	if resp.TranslatedQuery != "" {
		t.Errorf("expected no translated_query when the cache write failed, got %q", resp.TranslatedQuery)
	}
	found := false
	for _, r := range resp.Results {
		if r.FdcID != nil && *r.FdcID == 1 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected literal-query USDA results after a cache-write failure, got %+v", resp.Results)
	}
}

// A cross-site request (Sec-Fetch-Site: cross-site) must never trigger a
// translation call or cache write — the endpoint's only side effects — even
// on a cache miss, since the browser attaches the caller's SameSite=Lax
// session cookie to such requests without the caller ever proving it's the
// app's own frontend.
func TestFoodSearch_CrossSiteRequestSkipsTranslation(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	idx := buildUSDAIndex(t, usdaFood(1, "Chicken breast, cooked", 165))
	fake := &vision.Fake{TranslateErr: errors.New("Translate must not be called for a cross-site request")}
	h := server.NewFoodHandlers(st, idx, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	req := newSearchRequest(userID, "chicken+breast")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	w := httptest.NewRecorder()
	h.Search(w, req)

	if len(fake.TranslateCalls) != 0 {
		t.Fatalf("expected Translate not to be called for a cross-site request, got %d calls", len(fake.TranslateCalls))
	}
	resp := decodeSearchResponse(t, w)
	if resp.TranslatedQuery != "" {
		t.Errorf("expected no translated_query for a cross-site request, got %q", resp.TranslatedQuery)
	}
	var count int64
	st.DB().Model(&database.FoodSearchTranslation{}).Where("user_id = ?", userID).Count(&count)
	if count != 0 {
		t.Errorf("expected no cache row written for a cross-site request, found %d", count)
	}
}

// TestFoodSearch_CrossSiteRefreshSkipsTranslation mirrors the cache-miss
// case above for refresh=true, which carries the same side effects.
func TestFoodSearch_CrossSiteRefreshSkipsTranslation(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	row := database.FoodSearchTranslation{UserID: userID, OriginalQuery: "porridge", TranslatedQuery: "oatmeal"}
	row.ID = uuid.New()
	row.FamilyID = familyID
	if err := st.DB().Create(&row).Error; err != nil {
		t.Fatalf("seed cache row: %v", err)
	}

	idx := buildUSDAIndex(t, usdaFood(1, "Oatmeal, raw", 389))
	fake := &vision.Fake{TranslateErr: errors.New("Translate must not be called for a cross-site refresh")}
	h := server.NewFoodHandlers(st, idx, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	req := newSearchRequestRefresh(userID, "porridge")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	w := httptest.NewRecorder()
	h.Search(w, req)

	if len(fake.TranslateCalls) != 0 {
		t.Fatalf("expected Translate not to be called for a cross-site refresh, got %d calls", len(fake.TranslateCalls))
	}
	var unchanged database.FoodSearchTranslation
	if err := st.DB().Where("user_id = ? AND original_query = ?", userID, "porridge").First(&unchanged).Error; err != nil {
		t.Fatalf("expected the prior row to still exist: %v", err)
	}
	if unchanged.TranslatedQuery != "oatmeal" {
		t.Errorf("expected the prior mapping left unchanged by a cross-site refresh, got %q", unchanged.TranslatedQuery)
	}
}

// A successful refresh always reports its translated term, even when that
// term equals the normalized input — otherwise the response is
// indistinguishable from a failed refresh, which also omits
// translated_query.
func TestFoodSearch_RefreshEqualToInputStillReportsTranslatedQuery(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	row := database.FoodSearchTranslation{UserID: userID, OriginalQuery: "chicken breast", TranslatedQuery: "chicken liver"}
	row.ID = uuid.New()
	row.FamilyID = familyID
	if err := st.DB().Create(&row).Error; err != nil {
		t.Fatalf("seed stale cache row: %v", err)
	}

	idx := buildUSDAIndex(t, usdaFood(1, "Chicken breast, cooked", 165))
	fake := &vision.Fake{TranslateResult: "chicken breast"}
	h := server.NewFoodHandlers(st, idx, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.Search(w, newSearchRequestRefresh(userID, "chicken+breast"))

	resp := decodeSearchResponse(t, w)
	if resp.TranslatedQuery != "chicken breast" {
		t.Errorf("expected translated_query %q on a successful refresh even though it equals the input, got %q",
			"chicken breast", resp.TranslatedQuery)
	}
}

// Sec-Fetch-Site values other than "same-origin" — same-site, cross-site, and
// none — must all skip translation and cache writes on both a cache miss and
// refresh=true; only "same-origin" (or a missing header, exercised by every
// other test in this file) is trusted. This widens the original guard, which
// only rejected "cross-site": HealthVault runs sibling apps on other ports on
// this host, so a "same-site" (same host, different port) request would
// otherwise still carry the SameSite=Lax session cookie without ever proving
// it's this app's own frontend.
func TestFoodSearch_NonSameOriginRequestsSkipTranslation(t *testing.T) {
	for _, secFetchSite := range []string{"same-site", "cross-site", "none"} {
		t.Run(secFetchSite+"/cache-miss", func(t *testing.T) {
			st := newFoodTestStorage(t)
			userID, _ := seedFoodUser(t, st)

			idx := buildUSDAIndex(t, usdaFood(1, "Chicken breast, cooked", 165))
			fake := &vision.Fake{TranslateErr: fmt.Errorf("Translate must not be called for a %s request", secFetchSite)}
			h := server.NewFoodHandlers(st, idx, t.TempDir()).WithVision(fake, 10<<20, time.Second)

			req := newSearchRequest(userID, "chicken+breast")
			req.Header.Set("Sec-Fetch-Site", secFetchSite)
			w := httptest.NewRecorder()
			h.Search(w, req)

			if len(fake.TranslateCalls) != 0 {
				t.Fatalf("expected Translate not to be called for a %s request, got %d calls", secFetchSite, len(fake.TranslateCalls))
			}
			var count int64
			st.DB().Model(&database.FoodSearchTranslation{}).Where("user_id = ?", userID).Count(&count)
			if count != 0 {
				t.Errorf("expected no cache row written for a %s request, found %d", secFetchSite, count)
			}
		})

		t.Run(secFetchSite+"/refresh", func(t *testing.T) {
			st := newFoodTestStorage(t)
			userID, familyID := seedFoodUser(t, st)

			row := database.FoodSearchTranslation{UserID: userID, OriginalQuery: "porridge", TranslatedQuery: "oatmeal"}
			row.ID = uuid.New()
			row.FamilyID = familyID
			if err := st.DB().Create(&row).Error; err != nil {
				t.Fatalf("seed cache row: %v", err)
			}

			idx := buildUSDAIndex(t, usdaFood(1, "Oatmeal, raw", 389))
			fake := &vision.Fake{TranslateErr: fmt.Errorf("Translate must not be called for a %s refresh", secFetchSite)}
			h := server.NewFoodHandlers(st, idx, t.TempDir()).WithVision(fake, 10<<20, time.Second)

			req := newSearchRequestRefresh(userID, "porridge")
			req.Header.Set("Sec-Fetch-Site", secFetchSite)
			w := httptest.NewRecorder()
			h.Search(w, req)

			if len(fake.TranslateCalls) != 0 {
				t.Fatalf("expected Translate not to be called for a %s refresh, got %d calls", secFetchSite, len(fake.TranslateCalls))
			}
			var unchanged database.FoodSearchTranslation
			if err := st.DB().Where("user_id = ? AND original_query = ?", userID, "porridge").First(&unchanged).Error; err != nil {
				t.Fatalf("expected the prior row to still exist: %v", err)
			}
			if unchanged.TranslatedQuery != "oatmeal" {
				t.Errorf("expected the prior mapping left unchanged by a %s refresh, got %q", secFetchSite, unchanged.TranslatedQuery)
			}
		})
	}
}

// A custom food's exact-match short-circuit must be Unicode-aware: SQLite's
// built-in LOWER() only case-folds ASCII, so a naive SQL-side comparison
// would miss a non-ASCII name typed in different case and incorrectly fall
// through to translation (spending an LLM call and returning USDA results
// instead of the user's own food).
func TestFoodSearch_CustomFoodMatchIsUnicodeAware(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	custom := database.CustomFood{
		UserID: userID, Name: "Овсянка",
		CaloriesPer100g: 150, ProteinPer100g: 5, CarbsPer100g: 25, FatPer100g: 3,
	}
	custom.ID = uuid.New()
	custom.FamilyID = familyID
	if err := st.DB().Create(&custom).Error; err != nil {
		t.Fatalf("create custom food: %v", err)
	}

	idx := buildUSDAIndex(t, usdaFood(1, "Oatmeal, raw", 389))
	fake := &vision.Fake{TranslateErr: errors.New("Translate must not be called when a custom food matches")}
	h := server.NewFoodHandlers(st, idx, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	// Same word as the stored name, uppercased — SQLite's ASCII-only LOWER()
	// would fail to fold this, unlike strings.EqualFold.
	h.Search(w, newSearchRequest(userID, url.QueryEscape("ОВСЯНКА")))

	if len(fake.TranslateCalls) != 0 {
		t.Fatalf("expected Translate not to be called for an exact custom-food match, got %d calls", len(fake.TranslateCalls))
	}
	resp := decodeSearchResponse(t, w)
	if len(resp.Results) != 1 || resp.Results[0].Source != "custom" {
		t.Fatalf("expected exactly one custom result, got %+v", resp.Results)
	}
	if resp.Results[0].CustomFoodID == nil || *resp.Results[0].CustomFoodID != custom.ID {
		t.Errorf("expected match against %s, got %+v", custom.ID, resp.Results[0].CustomFoodID)
	}
}
