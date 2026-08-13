package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/server"
	"github.com/ya-breeze/healthvault/pkg/vision"
)

func newMealUploadRequest(t *testing.T, filename string, data []byte, hint ...string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("photo", filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write photo bytes: %v", err)
	}
	if len(hint) > 0 {
		if err := mw.WriteField("hint", hint[0]); err != nil {
			t.Fatalf("write hint: %v", err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/food/meals", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	return r
}

func TestCreateMeal_OptionalHintIsTrimmedAndForwarded(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	fake := &vision.Fake{}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)
	w := httptest.NewRecorder()
	h.CreateMeal(w, withClaims(newMealUploadRequest(t, "photo.jpg", fakeJPEGBytes, "  grilled chicken with red beans  "), userID))
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if len(fake.RecognizeCalls) != 1 || fake.RecognizeCalls[0].Hint != "grilled chicken with red beans" {
		t.Fatalf("expected trimmed upload hint, got %+v", fake.RecognizeCalls)
	}
}

func TestCreateMeal_HintUnicodeBoundaryAndWhitespaceOnly(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want string
	}{
		{name: "500 Unicode characters", raw: strings.Repeat("🙂", 500), want: strings.Repeat("🙂", 500)},
		{name: "whitespace only", raw: " \n\t ", want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := newFoodTestStorage(t)
			userID, _ := seedFoodUser(t, st)
			fake := &vision.Fake{}
			h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)
			w := httptest.NewRecorder()
			h.CreateMeal(w, withClaims(newMealUploadRequest(t, "photo.jpg", fakeJPEGBytes, tc.raw), userID))
			if w.Code != http.StatusCreated {
				t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
			}
			if len(fake.RecognizeCalls) != 1 || fake.RecognizeCalls[0].Hint != tc.want {
				t.Fatalf("expected hint %q, got %+v", tc.want, fake.RecognizeCalls)
			}
		})
	}
}

func TestCreateMeal_OverlongHintHasNoSideEffects(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	dir := t.TempDir()
	fake := &vision.Fake{}
	h := server.NewFoodHandlers(st, nil, dir).WithVision(fake, 10<<20, time.Second)
	w := httptest.NewRecorder()
	h.CreateMeal(w, withClaims(newMealUploadRequest(t, "photo.jpg", fakeJPEGBytes, strings.Repeat("🙂", 501)), userID))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	var count int64
	if err := st.DB().Model(&database.FoodMeal{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("expected no meal row, count=%d err=%v", count, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read photo dir: %v", err)
	}
	if len(entries) != 0 || len(fake.RecognizeCalls) != 0 {
		t.Fatalf("expected no photo or vision side effects, entries=%d calls=%d", len(entries), len(fake.RecognizeCalls))
	}
}

func TestCreateMeal_NoMatchLeavesItemUnresolved(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	fake := &vision.Fake{
		RecognizeResult: &vision.RecognizeResult{
			Items: []vision.Item{{Name: "Mystery food", WeightGrams: 100, Confidence: 0.5}},
			Model: "fake-model", Raw: `{"items":[{"name":"Mystery food"}]}`,
		},
	}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.CreateMeal(w, withClaims(newMealUploadRequest(t, "photo.jpg", fakeJPEGBytes), userID))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var meal database.FoodMeal
	if err := json.NewDecoder(w.Body).Decode(&meal); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if meal.Status != database.MealStatusPendingReview {
		t.Errorf("expected pending_review, got %s", meal.Status)
	}
	if meal.PhotoPath == "" {
		t.Error("expected a stored photo path")
	}
	if len(meal.Items) != 1 || meal.Items[0].MacroSource != database.MacroSourceNone {
		t.Errorf("expected one unresolved item, got %+v", meal.Items)
	}
	if len(fake.RecognizeCalls) != 1 {
		t.Fatalf("expected exactly one Recognize call, got %d", len(fake.RecognizeCalls))
	}
}

func TestCreateMeal_USDAMatchViaSelect(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	idx := buildUSDAIndex(t, usdaFood(42, "Chicken, broilers or fryers, breast, meat only, cooked, roasted", 165))

	fake := &vision.Fake{
		RecognizeResult: &vision.RecognizeResult{
			Items: []vision.Item{{Name: "chicken breast", Preparation: "roasted", State: "cooked", WeightGrams: 180, Confidence: 0.9}},
		},
		SelectResult: &vision.SelectResult{
			Selections: []vision.Selection{{ItemIndex: 0, CandidateIndex: 0}},
		},
	}
	h := server.NewFoodHandlers(st, idx, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.CreateMeal(w, withClaims(newMealUploadRequest(t, "photo.jpg", fakeJPEGBytes), userID))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var meal database.FoodMeal
	json.NewDecoder(w.Body).Decode(&meal) //nolint:errcheck
	if len(meal.Items) != 1 {
		t.Fatalf("expected one item, got %+v", meal.Items)
	}
	item := meal.Items[0]
	if item.MacroSource != database.MacroSourceReference {
		t.Errorf("expected reference macro source, got %s", item.MacroSource)
	}
	if item.FdcID == nil || *item.FdcID != 42 {
		t.Errorf("expected fdc_id 42, got %v", item.FdcID)
	}
	// 165 kcal/100g * 1.8 = 297
	if item.Calories < 296.9 || item.Calories > 297.1 {
		t.Errorf("expected calories ~297, got %v", item.Calories)
	}
	// The meal's own aggregate is left at zero until confirm.
	if meal.Calories != 0 {
		t.Errorf("expected meal aggregate to stay zero pre-confirm, got %v", meal.Calories)
	}
	if len(fake.SelectCalls) != 1 || len(fake.SelectCalls[0]) != 1 {
		t.Fatalf("expected exactly one Select call with one item, got %+v", fake.SelectCalls)
	}
}

// A recognized brand routes matching to Open Food Facts first, and its
// candidates are used without ever querying USDA (h.usda is nil here, so
// binding via fdc_id would panic/fail if USDA were consulted at all).
func TestCreateMeal_OFFMatchViaSelect(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	offIdx := buildOFFIndex(t, offFood("8594001222227", "Bílý jogurt", "Olma", 65))

	fake := &vision.Fake{
		RecognizeResult: &vision.RecognizeResult{
			Items: []vision.Item{{Name: "Bílý jogurt", Brand: "Olma", WeightGrams: 150, Confidence: 0.9}},
		},
		SelectResult: &vision.SelectResult{
			Selections: []vision.Selection{{ItemIndex: 0, CandidateIndex: 0}},
		},
	}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second).WithOFF(offIdx)

	w := httptest.NewRecorder()
	h.CreateMeal(w, withClaims(newMealUploadRequest(t, "photo.jpg", fakeJPEGBytes), userID))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var meal database.FoodMeal
	json.NewDecoder(w.Body).Decode(&meal) //nolint:errcheck
	if len(meal.Items) != 1 {
		t.Fatalf("expected one item, got %+v", meal.Items)
	}
	item := meal.Items[0]
	if item.OffCode == nil || *item.OffCode != "8594001222227" {
		t.Errorf("expected off_code bound, got %v", item.OffCode)
	}
	if item.FdcID != nil {
		t.Errorf("expected no fdc_id (USDA must not have been queried), got %v", item.FdcID)
	}
	// 65 kcal/100g * 1.5 = 97.5
	if item.Calories < 97.4 || item.Calories > 97.6 {
		t.Errorf("expected calories ~97.5, got %v", item.Calories)
	}
	// The candidate offered to Select must carry the recognized item's own
	// name/brand (design.md "Selection is offered the recognized item's own
	// name and brand") and the OFF candidate's Brands text.
	if len(fake.SelectCalls) != 1 || len(fake.SelectCalls[0]) != 1 {
		t.Fatalf("expected exactly one Select call with one item, got %+v", fake.SelectCalls)
	}
	sent := fake.SelectCalls[0][0]
	if sent.ItemName != "Bílý jogurt" || sent.ItemBrand != "Olma" {
		t.Errorf("expected ItemName/ItemBrand on the Select payload, got %+v", sent)
	}
	if len(sent.Candidates) != 1 || sent.Candidates[0].Brands != "Olma" {
		t.Errorf("expected the OFF candidate's Brands populated, got %+v", sent.Candidates)
	}
}

// When a brand is present but Open Food Facts returns zero candidates for
// it, resolution falls back to USDA rather than leaving the item unresolved.
func TestCreateMeal_BrandPresentOFFMissFallsBackToUSDA(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	// No product under "Olma" in the OFF index — only unrelated filler rows.
	offIdx := buildOFFIndex(t)
	usdaIdx := buildUSDAIndex(t, usdaFood(42, "Yogurt, plain", 60))

	fake := &vision.Fake{
		RecognizeResult: &vision.RecognizeResult{
			Items: []vision.Item{{Name: "yogurt", Brand: "Olma", WeightGrams: 150, Confidence: 0.9}},
		},
		SelectResult: &vision.SelectResult{
			Selections: []vision.Selection{{ItemIndex: 0, CandidateIndex: 0}},
		},
	}
	h := server.NewFoodHandlers(st, usdaIdx, t.TempDir()).WithVision(fake, 10<<20, time.Second).WithOFF(offIdx)

	w := httptest.NewRecorder()
	h.CreateMeal(w, withClaims(newMealUploadRequest(t, "photo.jpg", fakeJPEGBytes), userID))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var meal database.FoodMeal
	json.NewDecoder(w.Body).Decode(&meal) //nolint:errcheck
	item := meal.Items[0]
	if item.FdcID == nil || *item.FdcID != 42 {
		t.Errorf("expected fallback to USDA fdc_id 42, got %v", item.FdcID)
	}
	if item.OffCode != nil {
		t.Errorf("expected no off_code (OFF had no match), got %v", item.OffCode)
	}
}

// A brand naming a real manufacturer that just isn't in the OFF index for
// this product must not let a same-named product from a *different* brand
// through — the brand-required query still returns zero, and resolution
// falls back to USDA, rather than binding to the wrong brand's product.
func TestCreateMeal_BrandNotInIndexForThatNameFallsBackToUSDA(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	// The index has this product, but only under a different brand.
	offIdx := buildOFFIndex(t, offFood("222", "yogurt", "Danone", 60))
	usdaIdx := buildUSDAIndex(t, usdaFood(42, "Yogurt, plain", 60))

	fake := &vision.Fake{
		RecognizeResult: &vision.RecognizeResult{
			Items: []vision.Item{{Name: "yogurt", Brand: "Olma", WeightGrams: 150, Confidence: 0.9}},
		},
		SelectResult: &vision.SelectResult{
			Selections: []vision.Selection{{ItemIndex: 0, CandidateIndex: 0}},
		},
	}
	h := server.NewFoodHandlers(st, usdaIdx, t.TempDir()).WithVision(fake, 10<<20, time.Second).WithOFF(offIdx)

	w := httptest.NewRecorder()
	h.CreateMeal(w, withClaims(newMealUploadRequest(t, "photo.jpg", fakeJPEGBytes), userID))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var meal database.FoodMeal
	json.NewDecoder(w.Body).Decode(&meal) //nolint:errcheck
	item := meal.Items[0]
	if item.OffCode != nil {
		t.Errorf("expected no off_code (Danone product must not match an Olma search), got %v", item.OffCode)
	}
	if item.FdcID == nil || *item.FdcID != 42 {
		t.Errorf("expected fallback to USDA fdc_id 42, got %v", item.FdcID)
	}
}

// No recognized brand means Open Food Facts is never queried at all — even
// though the OFF index here contains a product that would match on name
// alone, resolution must go straight to USDA.
func TestCreateMeal_NoBrandSkipsOFFEntirely(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	offIdx := buildOFFIndex(t, offFood("111", "Chicken breast", "SomeBrand", 999))
	usdaIdx := buildUSDAIndex(t, usdaFood(42, "Chicken, broilers or fryers, breast, meat only, cooked, roasted", 165))

	fake := &vision.Fake{
		RecognizeResult: &vision.RecognizeResult{
			Items: []vision.Item{{Name: "chicken breast", WeightGrams: 180, Confidence: 0.9}}, // Brand left empty
		},
		SelectResult: &vision.SelectResult{
			Selections: []vision.Selection{{ItemIndex: 0, CandidateIndex: 0}},
		},
	}
	h := server.NewFoodHandlers(st, usdaIdx, t.TempDir()).WithVision(fake, 10<<20, time.Second).WithOFF(offIdx)

	w := httptest.NewRecorder()
	h.CreateMeal(w, withClaims(newMealUploadRequest(t, "photo.jpg", fakeJPEGBytes), userID))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var meal database.FoodMeal
	json.NewDecoder(w.Body).Decode(&meal) //nolint:errcheck
	item := meal.Items[0]
	if item.FdcID == nil || *item.FdcID != 42 {
		t.Errorf("expected USDA fdc_id 42 (OFF must not have been queried without a brand), got %v", item.FdcID)
	}
	if item.OffCode != nil {
		t.Errorf("expected no off_code, got %v", item.OffCode)
	}
}

func TestCreateMeal_ClarificationQuestionsSetPendingClarification(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	fake := &vision.Fake{
		RecognizeResult: &vision.RecognizeResult{
			Items:                  []vision.Item{{Name: "some sauce", WeightGrams: 30}},
			ClarificationQuestions: []string{"Is this a cream-based or tomato-based sauce?"},
		},
	}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.CreateMeal(w, withClaims(newMealUploadRequest(t, "photo.jpg", fakeJPEGBytes), userID))

	var meal database.FoodMeal
	json.NewDecoder(w.Body).Decode(&meal) //nolint:errcheck
	if meal.Status != database.MealStatusPendingClarification {
		t.Errorf("expected pending_clarification, got %s", meal.Status)
	}
	if len(fake.SelectCalls) != 0 {
		t.Errorf("expected no Select call when clarification is needed, got %d", len(fake.SelectCalls))
	}
}

func TestCreateMeal_RecognizeErrorMarksFailedAndRetainsPhoto(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	fake := &vision.Fake{RecognizeErr: errors.New("vision provider unavailable")}
	dir := t.TempDir()
	h := server.NewFoodHandlers(st, nil, dir).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.CreateMeal(w, withClaims(newMealUploadRequest(t, "photo.jpg", fakeJPEGBytes), userID))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 (the meal row itself is created), got %d: %s", w.Code, w.Body.String())
	}
	var meal database.FoodMeal
	json.NewDecoder(w.Body).Decode(&meal) //nolint:errcheck
	if meal.Status != database.MealStatusFailed {
		t.Errorf("expected failed, got %s", meal.Status)
	}
	if meal.PhotoPath == "" {
		t.Error("expected photo path to be retained on failure")
	}

	var persisted database.FoodMeal
	if err := st.DB().First(&persisted, "id = ?", meal.ID).Error; err != nil {
		t.Fatalf("reload meal: %v", err)
	}
	if persisted.Status != database.MealStatusFailed {
		t.Errorf("expected persisted status failed, got %s", persisted.Status)
	}
}

// Regression: round-7 review — failMeal's own status-write UPDATE affecting
// zero rows is used as the single signal for "a newer attempt superseded
// this one, reload rather than error" everywhere it's called from. That
// conflated two different situations: a legitimate supersession (someone
// else's newer lease owns the row now) and this attempt's own write failing
// outright (a real database error). The latter used to be treated the same
// as the former — reloaded and potentially returned as a 200/201 with the
// meal silently stuck in `processing`, hiding the failure entirely. It must
// now surface as a 500.
func TestCreateMeal_FailMealWriteErrorReturns500NotStuckProcessing(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	fake := &vision.Fake{RecognizeErr: errors.New("vision provider unavailable")}
	dir := t.TempDir()
	h := server.NewFoodHandlers(st, nil, dir).WithVision(fake, 10<<20, time.Second)

	const hookName = "test:fail-meal-write-error"
	st.DB().Callback().Update().Before("gorm:update").Register(hookName, func(tx *gorm.DB) {
		// The only UPDATE in this flow is failMeal's own status write
		// (analyzeMeal's runAnalysis error path); poisoning it unconditionally
		// is safe here.
		tx.Error = errors.New("simulated status-write failure")
	})
	t.Cleanup(func() { st.DB().Callback().Update().Remove(hookName) })

	w := httptest.NewRecorder()
	h.CreateMeal(w, withClaims(newMealUploadRequest(t, "photo.jpg", fakeJPEGBytes), userID))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when failMeal's own write errors, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateMeal_UnconfiguredVisionMarksFailed(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	// No WithVision call: exercises NewFoodHandlers' own defaults
	// (vision.Unconfigured{}, a real upload byte limit and timeout).
	h := server.NewFoodHandlers(st, nil, t.TempDir())

	w := httptest.NewRecorder()
	h.CreateMeal(w, withClaims(newMealUploadRequest(t, "photo.jpg", fakeJPEGBytes), userID))

	var meal database.FoodMeal
	json.NewDecoder(w.Body).Decode(&meal) //nolint:errcheck
	if meal.Status != database.MealStatusFailed {
		t.Errorf("expected failed when vision is unconfigured, got %s", meal.Status)
	}
}

// slowRecognizeClient blocks until its context is cancelled, standing in for
// a vision call that outruns HCW_VISION_TIMEOUT.
type slowRecognizeClient struct{}

func (slowRecognizeClient) Recognize(ctx context.Context, _ []byte, _ string, _ string) (*vision.RecognizeResult, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (slowRecognizeClient) EstimateWeights(context.Context, []byte, string, []vision.WeightEstimateInput) (*vision.WeightEstimateResult, error) {
	return &vision.WeightEstimateResult{}, nil
}

func (slowRecognizeClient) Clarify(context.Context, []vision.Item, []vision.ClarifyTurn) (*vision.RecognizeResult, error) {
	return &vision.RecognizeResult{}, nil
}

func (slowRecognizeClient) Select(context.Context, []vision.ItemCandidates) (*vision.SelectResult, error) {
	return &vision.SelectResult{}, nil
}

// Translate also blocks until its context is cancelled, so this double can
// stand in for a stalled translation call in food search timeout tests too.
func (slowRecognizeClient) Translate(ctx context.Context, _ string) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

func TestCreateMeal_TimeoutMarksFailed(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	h := server.NewFoodHandlers(st, nil, t.TempDir()).
		WithVision(slowRecognizeClient{}, 10<<20, time.Millisecond)

	w := httptest.NewRecorder()
	h.CreateMeal(w, withClaims(newMealUploadRequest(t, "photo.jpg", fakeJPEGBytes), userID))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var meal database.FoodMeal
	json.NewDecoder(w.Body).Decode(&meal) //nolint:errcheck
	if meal.Status != database.MealStatusFailed {
		t.Errorf("expected failed on timeout, got %s", meal.Status)
	}
	if meal.PhotoPath == "" {
		t.Error("expected photo to be retained on timeout")
	}
}

func TestCreateMeal_MissingPhotoFieldReturns400(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(&vision.Fake{}, 10<<20, time.Second)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.Close() //nolint:errcheck
	r := httptest.NewRequest(http.MethodPost, "/api/food/meals", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())

	w := httptest.NewRecorder()
	h.CreateMeal(w, withClaims(r, userID))

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestCreateMeal_OversizedUploadReturns413(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(&vision.Fake{}, 16, time.Second)

	w := httptest.NewRecorder()
	h.CreateMeal(w, withClaims(newMealUploadRequest(t, "photo.jpg", fakeJPEGBytes), userID))

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateMeal_HEICRejectedWith415(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(&vision.Fake{}, 10<<20, time.Second)

	heic := make([]byte, 40)
	copy(heic[4:8], "ftyp")
	copy(heic[8:12], "heic")

	w := httptest.NewRecorder()
	h.CreateMeal(w, withClaims(newMealUploadRequest(t, "photo.heic", heic), userID))

	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("expected 415, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateMeal_NonImageRejected(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(&vision.Fake{}, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.CreateMeal(w, withClaims(newMealUploadRequest(t, "not-an-image.txt", []byte("hello world")), userID))

	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("expected 415, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateMeal_TraversalShapedFilenameIgnored(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	dir := t.TempDir()
	h := server.NewFoodHandlers(st, nil, dir).WithVision(&vision.Fake{}, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.CreateMeal(w, withClaims(newMealUploadRequest(t, "../../../../etc/passwd.jpg", fakeJPEGBytes), userID))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var meal database.FoodMeal
	json.NewDecoder(w.Body).Decode(&meal) //nolint:errcheck
	if meal.PhotoPath == "" {
		t.Fatal("expected a stored photo path")
	}
	if bytes.Contains([]byte(meal.PhotoPath), []byte("..")) || bytes.Contains([]byte(meal.PhotoPath), []byte("etc")) {
		t.Errorf("expected the client filename to have no effect on the stored path, got %q", meal.PhotoPath)
	}
}

func TestCreateMeal_Unauthenticated(t *testing.T) {
	st := newFoodTestStorage(t)
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(&vision.Fake{}, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.CreateMeal(w, newMealUploadRequest(t, "photo.jpg", fakeJPEGBytes))

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestCreateMeal_CustomFoodWinsOverUSDAInSelection(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	idx := buildUSDAIndex(t, usdaFood(1, "Oats, raw", 389))

	custom := database.CustomFood{UserID: userID, Name: "Overnight Oats", CaloriesPer100g: 150}
	custom.ID = uuid.New()
	custom.FamilyID = familyID
	if err := st.DB().Create(&custom).Error; err != nil {
		t.Fatalf("create custom food: %v", err)
	}

	fake := &vision.Fake{
		RecognizeResult: &vision.RecognizeResult{
			Items: []vision.Item{{Name: "Overnight Oats", WeightGrams: 200}},
		},
		SelectResult: &vision.SelectResult{
			Selections: []vision.Selection{{ItemIndex: 0, CandidateIndex: 0}},
		},
	}
	h := server.NewFoodHandlers(st, idx, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.CreateMeal(w, withClaims(newMealUploadRequest(t, "photo.jpg", fakeJPEGBytes), userID))

	var meal database.FoodMeal
	json.NewDecoder(w.Body).Decode(&meal) //nolint:errcheck
	if len(meal.Items) != 1 || meal.Items[0].CustomFoodID == nil || *meal.Items[0].CustomFoodID != custom.ID {
		t.Fatalf("expected the custom food to be offered as the sole candidate, got %+v", meal.Items)
	}
	// Exactly one candidate should have been offered to Select (the custom
	// food), never the USDA "Oats, raw" match too.
	if len(fake.SelectCalls) != 1 || len(fake.SelectCalls[0][0].Candidates) != 1 {
		t.Fatalf("expected exactly one candidate offered, got %+v", fake.SelectCalls)
	}
}

// --- Estimate fallback (composite-food-recognition) ---

// No candidates at all (no USDA/OFF configured, no custom food) — the item
// falls back to Recognize's own persisted estimate instead of staying
// unresolved.
func TestCreateMeal_EmptyShortlistFallsBackToEstimate(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	fake := &vision.Fake{
		RecognizeResult: &vision.RecognizeResult{
			Items: []vision.Item{{
				Name: "Mexican vegetable mix", WeightGrams: 200, Confidence: 0.7,
				EstimatedProfile: &database.NutrientProfile{CaloriesPer100g: 90, ProteinPer100g: 3, CarbsPer100g: 15},
			}},
		},
	}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.CreateMeal(w, withClaims(newMealUploadRequest(t, "photo.jpg", fakeJPEGBytes), userID))
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var meal database.FoodMeal
	json.NewDecoder(w.Body).Decode(&meal) //nolint:errcheck
	if len(meal.Items) != 1 {
		t.Fatalf("expected one item, got %+v", meal.Items)
	}
	item := meal.Items[0]
	if item.MacroSource != database.MacroSourceEstimated {
		t.Fatalf("expected estimated macro source, got %s", item.MacroSource)
	}
	// 90 kcal/100g * 2.0 (200g) = 180
	if item.Calories < 179.9 || item.Calories > 180.1 {
		t.Errorf("expected calories ~180, got %v", item.Calories)
	}
	if len(fake.SelectCalls) != 0 {
		t.Errorf("expected no Select call with an empty shortlist, got %d", len(fake.SelectCalls))
	}
}

// A zero weight (vision returned no usable weight for this item) must not
// silently "resolve" to a zeroed-out estimate — the item should stay
// unresolved so it's still flagged for review, not hidden behind a
// legitimate-looking estimated badge with 0 calories.
func TestCreateMeal_EmptyShortlistZeroWeightEstimateStaysNone(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	fake := &vision.Fake{
		RecognizeResult: &vision.RecognizeResult{
			Items: []vision.Item{{
				Name: "Mexican vegetable mix", WeightGrams: 0, Confidence: 0.7,
				EstimatedProfile: &database.NutrientProfile{CaloriesPer100g: 90, ProteinPer100g: 3, CarbsPer100g: 15},
			}},
		},
	}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.CreateMeal(w, withClaims(newMealUploadRequest(t, "photo.jpg", fakeJPEGBytes), userID))
	var meal database.FoodMeal
	json.NewDecoder(w.Body).Decode(&meal) //nolint:errcheck
	if len(meal.Items) != 1 || meal.Items[0].MacroSource != database.MacroSourceNone {
		t.Fatalf("expected macro_source none for a zero-weight item despite a usable estimate, got %+v", meal.Items)
	}
	if !meal.Items[0].HasEstimate {
		t.Error("expected the estimate to remain stored on the row for a later weight correction")
	}
}

// No usable estimate either (Recognize produced none) — the item stays
// macro_source none, exactly as before this change.
func TestCreateMeal_EmptyShortlistNoEstimateStaysNone(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)

	fake := &vision.Fake{
		RecognizeResult: &vision.RecognizeResult{
			Items: []vision.Item{{Name: "Mystery food", WeightGrams: 100, Confidence: 0.5}},
		},
	}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.CreateMeal(w, withClaims(newMealUploadRequest(t, "photo.jpg", fakeJPEGBytes), userID))
	var meal database.FoodMeal
	json.NewDecoder(w.Body).Decode(&meal) //nolint:errcheck
	if len(meal.Items) != 1 || meal.Items[0].MacroSource != database.MacroSourceNone {
		t.Fatalf("expected macro_source none with no estimate, got %+v", meal.Items)
	}
}

// Select explicitly returning -1 ("none of these") still falls back to the
// item's own persisted estimate.
func TestCreateMeal_SelectNoneOfTheseFallsBackToEstimate(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	idx := buildUSDAIndex(t, usdaFood(1, "Unrelated food", 50))

	fake := &vision.Fake{
		RecognizeResult: &vision.RecognizeResult{
			Items: []vision.Item{{
				Name: "chicken curry", WeightGrams: 250, Confidence: 0.8,
				EstimatedProfile: &database.NutrientProfile{CaloriesPer100g: 140, ProteinPer100g: 9},
			}},
		},
		SelectResult: &vision.SelectResult{
			Selections: []vision.Selection{{ItemIndex: 0, CandidateIndex: -1}},
		},
	}
	h := server.NewFoodHandlers(st, idx, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.CreateMeal(w, withClaims(newMealUploadRequest(t, "photo.jpg", fakeJPEGBytes), userID))
	var meal database.FoodMeal
	json.NewDecoder(w.Body).Decode(&meal) //nolint:errcheck
	if len(meal.Items) != 1 {
		t.Fatalf("expected one item, got %+v", meal.Items)
	}
	item := meal.Items[0]
	if item.MacroSource != database.MacroSourceEstimated {
		t.Fatalf("expected estimated macro source after an explicit non-match, got %s", item.MacroSource)
	}
	// 140 kcal/100g * 2.5 (250g) = 350
	if item.Calories < 349.9 || item.Calories > 350.1 {
		t.Errorf("expected calories ~350, got %v", item.Calories)
	}
}

// A matched candidate takes precedence: the estimate is discarded for macro
// purposes (macro_source stays reference) even though it remains stored on
// the row.
func TestCreateMeal_MatchedCandidateDiscardsUnusedEstimate(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	idx := buildUSDAIndex(t, usdaFood(42, "Chicken breast", 165))

	fake := &vision.Fake{
		RecognizeResult: &vision.RecognizeResult{
			Items: []vision.Item{{
				Name: "chicken breast", WeightGrams: 180, Confidence: 0.9,
				EstimatedProfile: &database.NutrientProfile{CaloriesPer100g: 500, ProteinPer100g: 500},
			}},
		},
		SelectResult: &vision.SelectResult{
			Selections: []vision.Selection{{ItemIndex: 0, CandidateIndex: 0}},
		},
	}
	h := server.NewFoodHandlers(st, idx, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.CreateMeal(w, withClaims(newMealUploadRequest(t, "photo.jpg", fakeJPEGBytes), userID))
	var meal database.FoodMeal
	json.NewDecoder(w.Body).Decode(&meal) //nolint:errcheck
	item := meal.Items[0]
	if item.MacroSource != database.MacroSourceReference {
		t.Fatalf("expected reference macro source (matched candidate wins), got %s", item.MacroSource)
	}
	// 165 kcal/100g * 1.8 = 297, not the wildly different estimate values.
	if item.Calories < 296.9 || item.Calories > 297.1 {
		t.Errorf("expected calories from the matched reference (~297), not the discarded estimate, got %v", item.Calories)
	}
	if !item.HasEstimate {
		t.Error("expected the estimate to remain stored on the row even though unused")
	}
}

// --- Ranked custom-food candidates (composite-food-recognition) ---

// logConfirmedCustomFoodUse creates a confirmed meal with one item bound to
// customFoodID, so it counts toward that food's future ranking score.
func logConfirmedCustomFoodUse(t *testing.T, st database.Storage, userID, familyID, customFoodID uuid.UUID, loggedAt time.Time) {
	t.Helper()
	meal := database.FoodMeal{UserID: userID, Status: database.MealStatusConfirmed, LoggedAt: loggedAt}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatalf("create confirmed meal: %v", err)
	}
	item := database.FoodItem{
		UserID: userID, MealID: meal.ID, Name: "prior use", WeightGrams: 100,
		MacroSource: database.MacroSourceReference, CustomFoodID: &customFoodID,
	}
	item.ID = uuid.New()
	item.FamilyID = familyID
	if err := st.DB().Create(&item).Error; err != nil {
		t.Fatalf("create confirmed item: %v", err)
	}
}

// A differently-worded recognized item still matches a previously-used
// custom food via the ranked shortlist, not only an exact name match.
func TestCreateMeal_RankedCustomFoodOffersDifferentlyWordedName(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	custom := database.CustomFood{UserID: userID, Name: "Mexican vegetable mix", CaloriesPer100g: 90}
	custom.ID = uuid.New()
	custom.FamilyID = familyID
	if err := st.DB().Create(&custom).Error; err != nil {
		t.Fatalf("create custom food: %v", err)
	}
	logConfirmedCustomFoodUse(t, st, userID, familyID, custom.ID, time.Now().Add(-30*24*time.Hour))

	fake := &vision.Fake{
		RecognizeResult: &vision.RecognizeResult{
			// Deliberately worded differently from the saved custom food's name.
			Items: []vision.Item{{Name: "Mexican veggie mix", WeightGrams: 200, Confidence: 0.6}},
		},
		SelectResult: &vision.SelectResult{
			Selections: []vision.Selection{{ItemIndex: 0, CandidateIndex: 0}},
		},
	}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.CreateMeal(w, withClaims(newMealUploadRequest(t, "photo.jpg", fakeJPEGBytes), userID))
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if len(fake.SelectCalls) != 1 || len(fake.SelectCalls[0][0].Candidates) != 1 {
		t.Fatalf("expected exactly one ranked custom-food candidate offered, got %+v", fake.SelectCalls)
	}
	got := fake.SelectCalls[0][0].Candidates[0]
	if got.CustomFoodID == nil || *got.CustomFoodID != custom.ID {
		t.Errorf("expected the previously-used custom food offered as a candidate, got %+v", got)
	}
}

// A custom food used only on a still-pending_review meal must not yet count
// toward its own future ranking — only confirmed usage does.
func TestCreateMeal_RankedCustomFoodExcludesUnconfirmedUsage(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	custom := database.CustomFood{UserID: userID, Name: "Mexican vegetable mix", CaloriesPer100g: 90}
	custom.ID = uuid.New()
	custom.FamilyID = familyID
	if err := st.DB().Create(&custom).Error; err != nil {
		t.Fatalf("create custom food: %v", err)
	}
	// A pending_review meal (not confirmed) already bound to this custom food.
	pending := database.FoodMeal{UserID: userID, Status: database.MealStatusPendingReview, LoggedAt: time.Now()}
	pending.ID = uuid.New()
	pending.FamilyID = familyID
	if err := st.DB().Create(&pending).Error; err != nil {
		t.Fatalf("create pending meal: %v", err)
	}
	pendingItem := database.FoodItem{
		UserID: userID, MealID: pending.ID, Name: "unverified match", WeightGrams: 100,
		MacroSource: database.MacroSourceReference, CustomFoodID: &custom.ID,
	}
	pendingItem.ID = uuid.New()
	pendingItem.FamilyID = familyID
	if err := st.DB().Create(&pendingItem).Error; err != nil {
		t.Fatalf("create pending item: %v", err)
	}

	fake := &vision.Fake{
		RecognizeResult: &vision.RecognizeResult{
			Items: []vision.Item{{Name: "Mexican veggie mix", WeightGrams: 200, Confidence: 0.6}},
		},
	}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.CreateMeal(w, withClaims(newMealUploadRequest(t, "photo.jpg", fakeJPEGBytes), userID))
	var meal database.FoodMeal
	json.NewDecoder(w.Body).Decode(&meal) //nolint:errcheck
	if len(meal.Items) != 1 || meal.Items[0].MacroSource != database.MacroSourceNone {
		t.Fatalf("expected macro_source none: an unconfirmed use must not have surfaced the candidate, got %+v", meal.Items)
	}
	if len(fake.SelectCalls) != 0 {
		t.Errorf("expected no Select call (no candidates offered), got %d", len(fake.SelectCalls))
	}
}

// Frequency outranks recency: a custom food used twice (confirmed, long ago)
// ranks ahead of one used once (confirmed, very recently).
func TestCreateMeal_RankedCustomFoodFrequencyOutranksRecency(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)

	frequent := database.CustomFood{UserID: userID, Name: "Frequent Mix", CaloriesPer100g: 90}
	frequent.ID = uuid.New()
	frequent.FamilyID = familyID
	if err := st.DB().Create(&frequent).Error; err != nil {
		t.Fatalf("create frequent custom food: %v", err)
	}
	recent := database.CustomFood{UserID: userID, Name: "Recent Mix", CaloriesPer100g: 90}
	recent.ID = uuid.New()
	recent.FamilyID = familyID
	if err := st.DB().Create(&recent).Error; err != nil {
		t.Fatalf("create recent custom food: %v", err)
	}
	logConfirmedCustomFoodUse(t, st, userID, familyID, frequent.ID, time.Now().Add(-60*24*time.Hour))
	logConfirmedCustomFoodUse(t, st, userID, familyID, frequent.ID, time.Now().Add(-45*24*time.Hour))
	logConfirmedCustomFoodUse(t, st, userID, familyID, recent.ID, time.Now().Add(-1*time.Hour))

	fake := &vision.Fake{
		RecognizeResult: &vision.RecognizeResult{
			Items: []vision.Item{{Name: "Some new mix", WeightGrams: 200, Confidence: 0.6}},
		},
	}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	w := httptest.NewRecorder()
	h.CreateMeal(w, withClaims(newMealUploadRequest(t, "photo.jpg", fakeJPEGBytes), userID))
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if len(fake.SelectCalls) != 1 || len(fake.SelectCalls[0][0].Candidates) != 2 {
		t.Fatalf("expected both custom foods offered as candidates, got %+v", fake.SelectCalls)
	}
	candidates := fake.SelectCalls[0][0].Candidates
	if candidates[0].CustomFoodID == nil || *candidates[0].CustomFoodID != frequent.ID {
		t.Errorf("expected the twice-used food ranked first despite being older, got %+v", candidates)
	}
}

// A branded item with Open Food Facts candidates still also gets
// frequency/recency-ranked custom-food candidates alongside them.
func TestCreateMeal_RankedCustomFoodAdditiveWithOFFCandidates(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	offIdx := buildOFFIndex(t, offFood("8594001222227", "Bílý jogurt", "Olma", 65))

	custom := database.CustomFood{UserID: userID, Name: "My Own Yogurt Blend", CaloriesPer100g: 80}
	custom.ID = uuid.New()
	custom.FamilyID = familyID
	if err := st.DB().Create(&custom).Error; err != nil {
		t.Fatalf("create custom food: %v", err)
	}
	logConfirmedCustomFoodUse(t, st, userID, familyID, custom.ID, time.Now().Add(-10*24*time.Hour))

	fake := &vision.Fake{
		RecognizeResult: &vision.RecognizeResult{
			Items: []vision.Item{{Name: "Bílý jogurt", Brand: "Olma", WeightGrams: 150, Confidence: 0.9}},
		},
		SelectResult: &vision.SelectResult{
			Selections: []vision.Selection{{ItemIndex: 0, CandidateIndex: 0}},
		},
	}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second).WithOFF(offIdx)

	w := httptest.NewRecorder()
	h.CreateMeal(w, withClaims(newMealUploadRequest(t, "photo.jpg", fakeJPEGBytes), userID))
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if len(fake.SelectCalls) != 1 || len(fake.SelectCalls[0][0].Candidates) != 2 {
		t.Fatalf("expected both the OFF candidate and the ranked custom food offered, got %+v", fake.SelectCalls)
	}
	var sawOFF, sawCustom bool
	for _, c := range fake.SelectCalls[0][0].Candidates {
		if c.OffCode != nil {
			sawOFF = true
		}
		if c.CustomFoodID != nil && *c.CustomFoodID == custom.ID {
			sawCustom = true
		}
	}
	if !sawOFF || !sawCustom {
		t.Errorf("expected both OFF and custom-food candidates present, got %+v", fake.SelectCalls[0][0].Candidates)
	}
}
