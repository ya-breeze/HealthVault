package database_test

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ya-breeze/healthvault/pkg/database"
)

func newMeal(userID, familyID uuid.UUID, at time.Time) *database.FoodMeal {
	m := &database.FoodMeal{
		UserID:   userID,
		LoggedAt: at,
		Status:   database.MealStatusPendingReview,
	}
	m.ID = uuid.New()
	m.FamilyID = familyID
	return m
}

// TenantModel has no BeforeCreate hook, so ID and FamilyID must be set explicitly.
func TestFoodMeal_TenantFieldsPersist(t *testing.T) {
	s := newTestStorage(t)
	userID, familyID := seedUserAndFamily(t, s)

	m := newMeal(userID, familyID, time.Now().UTC())
	if err := s.DB().Create(m).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}

	var got database.FoodMeal
	if err := s.DB().First(&got, "id = ?", m.ID).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.FamilyID != familyID {
		t.Errorf("FamilyID = %v, want %v", got.FamilyID, familyID)
	}
	if got.UserID != userID {
		t.Errorf("UserID = %v, want %v", got.UserID, userID)
	}
}

// Items are user-scoped, not only family-scoped, because every ownership rule
// in this capability keys off user_id.
func TestFoodItem_CarriesUserID(t *testing.T) {
	s := newTestStorage(t)
	userID, familyID := seedUserAndFamily(t, s)

	m := newMeal(userID, familyID, time.Now().UTC())
	if err := s.DB().Create(m).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}

	it := database.FoodItem{
		UserID: userID, MealID: m.ID, Name: "rice",
		MacroSource: database.MacroSourceNone, WeightGrams: 100,
	}
	it.ID = uuid.New()
	it.FamilyID = familyID
	if err := s.DB().Create(&it).Error; err != nil {
		t.Fatalf("create item: %v", err)
	}

	var got database.FoodItem
	if err := s.DB().First(&got, "id = ?", it.ID).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.UserID != userID {
		t.Errorf("item UserID = %v, want %v", got.UserID, userID)
	}
}

// A pre-existing row (created before CanonicalName existed) reads back with
// an empty CanonicalName rather than failing to scan or defaulting to some
// sentinel — see openspec/specs/food-nutrition-logging "A pre-existing item
// has no Canonical Name".
func TestFoodItem_CanonicalNameDefaultsEmpty(t *testing.T) {
	s := newTestStorage(t)
	userID, familyID := seedUserAndFamily(t, s)

	m := newMeal(userID, familyID, time.Now().UTC())
	if err := s.DB().Create(m).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}

	it := database.FoodItem{
		UserID: userID, MealID: m.ID, Name: "rice",
		MacroSource: database.MacroSourceNone, WeightGrams: 100,
	}
	it.ID = uuid.New()
	it.FamilyID = familyID
	if err := s.DB().Create(&it).Error; err != nil {
		t.Fatalf("create item: %v", err)
	}

	var got database.FoodItem
	if err := s.DB().First(&got, "id = ?", it.ID).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.CanonicalName != "" {
		t.Errorf("CanonicalName = %q, want empty", got.CanonicalName)
	}
}

// CanonicalName round-trips through create/read like any other text column.
func TestFoodItem_CanonicalNameRoundTrips(t *testing.T) {
	s := newTestStorage(t)
	userID, familyID := seedUserAndFamily(t, s)

	m := newMeal(userID, familyID, time.Now().UTC())
	if err := s.DB().Create(m).Error; err != nil {
		t.Fatalf("create meal: %v", err)
	}

	it := database.FoodItem{
		UserID: userID, MealID: m.ID, Name: "вареники", CanonicalName: "dumplings",
		MacroSource: database.MacroSourceNone, WeightGrams: 100,
	}
	it.ID = uuid.New()
	it.FamilyID = familyID
	if err := s.DB().Create(&it).Error; err != nil {
		t.Fatalf("create item: %v", err)
	}

	var got database.FoodItem
	if err := s.DB().First(&got, "id = ?", it.ID).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.CanonicalName != "dumplings" {
		t.Errorf("CanonicalName = %q, want %q", got.CanonicalName, "dumplings")
	}
}

// A user may legitimately log two meals at the same recorded time; unlike the
// health metric tables there is no unique constraint on (user_id, logged_at).
func TestFoodMeal_TwoMealsSameLoggedAt(t *testing.T) {
	s := newTestStorage(t)
	userID, familyID := seedUserAndFamily(t, s)
	at := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)

	for i := range 2 {
		if err := s.DB().Create(newMeal(userID, familyID, at)).Error; err != nil {
			t.Fatalf("create meal %d: %v", i, err)
		}
	}

	var n int64
	s.DB().Model(&database.FoodMeal{}).Where("user_id = ? AND logged_at = ?", userID, at).Count(&n)
	if n != 2 {
		t.Errorf("meals at same logged_at = %d, want 2", n)
	}
}

// Custom food names are unique per user so name-based precedence over USDA
// has exactly one winner.
func TestCustomFood_DuplicateNameRejected(t *testing.T) {
	s := newTestStorage(t)
	userID, familyID := seedUserAndFamily(t, s)

	mk := func() *database.CustomFood {
		c := &database.CustomFood{UserID: userID, Name: "yogurt", CaloriesPer100g: 60}
		c.ID = uuid.New()
		c.FamilyID = familyID
		return c
	}
	if err := s.DB().Create(mk()).Error; err != nil {
		t.Fatalf("first create: %v", err)
	}
	err := s.DB().Create(mk()).Error
	if err == nil {
		t.Fatal("second create with duplicate name succeeded, want unique violation")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unique") {
		t.Errorf("error = %v, want a uniqueness violation", err)
	}
}

func TestFoodItem_ApplyProfileScalesByWeight(t *testing.T) {
	it := database.FoodItem{WeightGrams: 180}
	it.ApplyProfile(database.NutrientProfile{CaloriesPer100g: 165, ProteinPer100g: 31})

	if math.Abs(it.Calories-297) > 1e-9 {
		t.Errorf("Calories = %v, want 297", it.Calories)
	}
	if math.Abs(it.ProteinGrams-55.8) > 1e-9 {
		t.Errorf("ProteinGrams = %v, want 55.8", it.ProteinGrams)
	}
	if it.MacroSource != database.MacroSourceReference {
		t.Errorf("MacroSource = %q, want %q", it.MacroSource, database.MacroSourceReference)
	}
}

// The bug this guards: aggregating only reference-bound items zeroes out a meal
// logged entirely from package labels.
func TestFoodMeal_AggregateIncludesManualItems(t *testing.T) {
	items := []database.FoodItem{
		{MacroSource: database.MacroSourceReference, Calories: 100, ProteinGrams: 10},
		{MacroSource: database.MacroSourceManual, Calories: 250, ProteinGrams: 5},
		{MacroSource: database.MacroSourceNone, Calories: 999, ProteinGrams: 999},
	}
	var m database.FoodMeal
	m.Aggregate(items)

	if m.Calories != 350 {
		t.Errorf("Calories = %v, want 350 (reference + manual, excluding none)", m.Calories)
	}
	if m.ProteinGrams != 15 {
		t.Errorf("ProteinGrams = %v, want 15", m.ProteinGrams)
	}
}

func TestFoodMeal_AggregateIncludesEstimatedItems(t *testing.T) {
	items := []database.FoodItem{
		{MacroSource: database.MacroSourceReference, Calories: 100},
		{MacroSource: database.MacroSourceEstimated, Calories: 80},
		{MacroSource: database.MacroSourceNone, Calories: 999},
	}
	var m database.FoodMeal
	m.Aggregate(items)
	if m.Calories != 180 {
		t.Errorf("Calories = %v, want 180 (reference + estimated, excluding none)", m.Calories)
	}
}

func TestFoodItem_SetAndReadEstimatedProfile(t *testing.T) {
	it := database.FoodItem{}
	if _, ok := it.EstimatedProfile(); ok {
		t.Fatal("expected no estimate present on a zero-value item")
	}

	it.SetEstimatedProfile(&database.NutrientProfile{CaloriesPer100g: 120, ProteinPer100g: 8})
	p, ok := it.EstimatedProfile()
	if !ok {
		t.Fatal("expected estimate present after SetEstimatedProfile")
	}
	if p.CaloriesPer100g != 120 || p.ProteinPer100g != 8 {
		t.Errorf("EstimatedProfile = %+v, want calories=120 protein=8", p)
	}
	if !it.HasEstimate {
		t.Error("expected HasEstimate=true")
	}
}

// SetEstimatedProfile(nil) is the case where Recognize produced no usable
// estimate for an item — it must leave the item exactly as it found it, not
// flip HasEstimate on with zeroed values (which EstimatedProfile could not
// then distinguish from a real all-zero estimate, e.g. water).
func TestFoodItem_SetEstimatedProfileNilIsNoOp(t *testing.T) {
	it := database.FoodItem{}
	it.SetEstimatedProfile(nil)
	if it.HasEstimate {
		t.Error("expected HasEstimate to remain false when passed a nil profile")
	}
	if _, ok := it.EstimatedProfile(); ok {
		t.Error("expected no usable estimate after SetEstimatedProfile(nil)")
	}
}

// A negative value anywhere in the persisted estimate is treated as no
// usable estimate at all, not stored as nonsensical negative macros.
func TestFoodItem_EstimatedProfileRejectsNegativeValues(t *testing.T) {
	it := database.FoodItem{}
	it.SetEstimatedProfile(&database.NutrientProfile{CaloriesPer100g: 100, SodiumPer100g: -1})
	if _, ok := it.EstimatedProfile(); ok {
		t.Error("expected a negative-valued estimate to be reported as unusable")
	}
}

func TestFoodItem_ApplyEstimatedProfileScalesByWeightAndSetsSource(t *testing.T) {
	it := database.FoodItem{WeightGrams: 200}
	it.SetEstimatedProfile(&database.NutrientProfile{CaloriesPer100g: 150, ProteinPer100g: 10})

	if ok := it.ApplyEstimatedProfile(); !ok {
		t.Fatal("expected ApplyEstimatedProfile to succeed with a usable estimate")
	}
	if it.Calories != 300 || it.ProteinGrams != 20 {
		t.Errorf("Calories/Protein = %v/%v, want 300/20", it.Calories, it.ProteinGrams)
	}
	if it.MacroSource != database.MacroSourceEstimated {
		t.Errorf("MacroSource = %q, want %q", it.MacroSource, database.MacroSourceEstimated)
	}
}

// Applying an estimate that isn't present must change nothing — callers rely
// on this to leave a still-unresolved item at whatever MacroSource it
// already had (typically none) rather than silently zeroing it out.
func TestFoodItem_ApplyEstimatedProfileNoOpWhenAbsent(t *testing.T) {
	it := database.FoodItem{WeightGrams: 200, MacroSource: database.MacroSourceNone, Calories: 0}
	if ok := it.ApplyEstimatedProfile(); ok {
		t.Fatal("expected ApplyEstimatedProfile to report false with no usable estimate")
	}
	if it.MacroSource != database.MacroSourceNone || it.Calories != 0 {
		t.Errorf("expected item unchanged, got MacroSource=%q Calories=%v", it.MacroSource, it.Calories)
	}
}

func TestFoodItem_PlausibleEstimatedProfile_NormalFoodPasses(t *testing.T) {
	it := database.FoodItem{}
	it.SetEstimatedProfile(&database.NutrientProfile{
		CaloriesPer100g: 310, ProteinPer100g: 25, CarbsPer100g: 30, FatPer100g: 10,
		SugarPer100g: 5, DietaryFiberPer100g: 3,
	})
	if _, ok := it.PlausibleEstimatedProfile(); !ok {
		t.Fatal("expected a normal profile to be usable")
	}
}

// No estimate at all is unusable, same as EstimatedProfile.
func TestFoodItem_PlausibleEstimatedProfile_AbsentIsUnusable(t *testing.T) {
	it := database.FoodItem{}
	if _, ok := it.PlausibleEstimatedProfile(); ok {
		t.Fatal("expected no estimate to be reported as unusable")
	}
}

// An individual macro past the 102g/100g ceiling is rejected, even though
// (given non-negative inputs) it also always breaches the combined-sum
// check — the two checks overlap by construction, not a gap.
func TestFoodItem_PlausibleEstimatedProfile_IndividualMacroCeilingRejected(t *testing.T) {
	it := database.FoodItem{}
	it.SetEstimatedProfile(&database.NutrientProfile{CaloriesPer100g: 420, ProteinPer100g: 105})
	if _, ok := it.PlausibleEstimatedProfile(); ok {
		t.Error("expected protein at 105g/100g to be rejected")
	}
}

// A profile where no single macro exceeds 102g/100g but the three still sum
// past 102g/100g must still be rejected — this is the case the individual
// check alone cannot catch.
func TestFoodItem_PlausibleEstimatedProfile_CombinedSumRejectedWithoutIndividualViolation(t *testing.T) {
	it := database.FoodItem{}
	it.SetEstimatedProfile(&database.NutrientProfile{
		CaloriesPer100g: 1080, ProteinPer100g: 40, CarbsPer100g: 40, FatPer100g: 40,
	})
	if _, ok := it.PlausibleEstimatedProfile(); ok {
		t.Error("expected protein=carbs=fat=40 (sum 120) to be rejected despite no individual macro exceeding 102g")
	}
}

// Exactly at the 102g/100g combined ceiling is still usable — the tolerance
// is inclusive, not a strict-less-than boundary.
func TestFoodItem_PlausibleEstimatedProfile_CombinedSumBoundaryPasses(t *testing.T) {
	it := database.FoodItem{}
	// atwater = 34*4 + 34*4 + 34*9 = 578.
	it.SetEstimatedProfile(&database.NutrientProfile{CaloriesPer100g: 578, ProteinPer100g: 34, CarbsPer100g: 34, FatPer100g: 34})
	if _, ok := it.PlausibleEstimatedProfile(); !ok {
		t.Error("expected a combined macro sum of exactly 102g/100g to be usable")
	}
}

// Sugar and fiber are each individually within carbs+2g but their sum is
// not — both are subsets of total carbs, so they must be checked together,
// not independently (see design.md decision 3 / llm-first-macro-estimate).
func TestFoodItem_PlausibleEstimatedProfile_SugarPlusFiberCombinedRejected(t *testing.T) {
	it := database.FoodItem{}
	it.SetEstimatedProfile(&database.NutrientProfile{
		CaloriesPer100g: 100, CarbsPer100g: 10, SugarPer100g: 12, DietaryFiberPer100g: 12,
	})
	if _, ok := it.PlausibleEstimatedProfile(); ok {
		t.Error("expected sugar=12 + fiber=12 against carbs=10 to be rejected even though each individually is within carbs+2g")
	}
}

// Sugar plus fiber exactly at the carbs+2g combined boundary is still usable.
func TestFoodItem_PlausibleEstimatedProfile_SugarPlusFiberBoundaryPasses(t *testing.T) {
	it := database.FoodItem{}
	it.SetEstimatedProfile(&database.NutrientProfile{
		CaloriesPer100g: 100, CarbsPer100g: 10, SugarPer100g: 6, DietaryFiberPer100g: 6,
	})
	if _, ok := it.PlausibleEstimatedProfile(); !ok {
		t.Error("expected sugar=6 + fiber=6 against carbs=10 (exactly carbs+2g) to be usable")
	}
}

// Declared calories below the one-sided Atwater threshold are rejected.
func TestFoodItem_PlausibleEstimatedProfile_CaloriesBelowAtwaterThresholdRejected(t *testing.T) {
	it := database.FoodItem{}
	// atwater = 25*4 = 100, tolerance = max(25, 15) = 25, threshold = 75.
	it.SetEstimatedProfile(&database.NutrientProfile{CaloriesPer100g: 74.9, ProteinPer100g: 25})
	if _, ok := it.PlausibleEstimatedProfile(); ok {
		t.Error("expected calories just below the Atwater threshold to be rejected")
	}
}

// Exactly at the Atwater threshold is still usable.
func TestFoodItem_PlausibleEstimatedProfile_CaloriesAtAtwaterThresholdPasses(t *testing.T) {
	it := database.FoodItem{}
	it.SetEstimatedProfile(&database.NutrientProfile{CaloriesPer100g: 75, ProteinPer100g: 25})
	if _, ok := it.PlausibleEstimatedProfile(); !ok {
		t.Error("expected calories exactly at the Atwater threshold to be usable")
	}
}

// The calorie check is one-sided: calories exceeding the Atwater figure is
// never a violation, since alcohol-containing foods legitimately have
// calories the tracked macros don't capture (wine: ~83 kcal/100g against
// ~10 kcal/100g of P/C/F).
func TestFoodItem_PlausibleEstimatedProfile_WineCaloriesExceedingAtwaterPasses(t *testing.T) {
	it := database.FoodItem{}
	it.SetEstimatedProfile(&database.NutrientProfile{CaloriesPer100g: 83, CarbsPer100g: 2.6})
	if _, ok := it.PlausibleEstimatedProfile(); !ok {
		t.Error("expected wine's above-Atwater calories to be usable, not rejected")
	}
}

// Legitimate high-single-macro foods pass despite being near the individual
// ceiling: protein powder, and pure oil after model rounding pushes it just
// past 100g/100g (within the 2g tolerance).
func TestFoodItem_PlausibleEstimatedProfile_LegitimateHighMacroFoodsPass(t *testing.T) {
	for _, tc := range []struct {
		name string
		p    database.NutrientProfile
	}{
		{
			name: "protein powder",
			p: database.NutrientProfile{
				CaloriesPer100g: 385, ProteinPer100g: 80, CarbsPer100g: 5, FatPer100g: 5,
				SugarPer100g: 2, DietaryFiberPer100g: 1,
			},
		},
		{
			name: "pure oil rounded to 100.4g/100g",
			p:    database.NutrientProfile{CaloriesPer100g: 903.6, FatPer100g: 100.4},
		},
		{
			name: "dry gelatin",
			p:    database.NutrientProfile{CaloriesPer100g: 340, ProteinPer100g: 85},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			it := database.FoodItem{}
			it.SetEstimatedProfile(&tc.p)
			if _, ok := it.PlausibleEstimatedProfile(); !ok {
				t.Errorf("expected %s (%+v) to be usable", tc.name, tc.p)
			}
		})
	}
}

// A legacy row already persisted with MacroSource estimated, whose stored
// profile would fail the new plausibility bounds, must still rescale
// consistently from that same profile on a later weight-only edit —
// PlausibleEstimatedProfile only gates the resolveItems binding decision,
// never ApplyEstimatedProfile/EstimatedProfile's own semantics.
func TestFoodItem_ApplyEstimatedProfileRescalesLegacyImplausibleEstimate(t *testing.T) {
	it := database.FoodItem{WeightGrams: 100, MacroSource: database.MacroSourceEstimated}
	it.SetEstimatedProfile(&database.NutrientProfile{CaloriesPer100g: 2000, ProteinPer100g: 500})
	if _, ok := it.PlausibleEstimatedProfile(); ok {
		t.Fatal("expected this profile to fail the new plausibility bounds (test setup)")
	}

	it.WeightGrams = 250 // simulate a weight-only PATCH
	if ok := it.ApplyEstimatedProfile(); !ok {
		t.Fatal("expected a legacy implausible-but-present estimate to still rescale")
	}
	if it.MacroSource != database.MacroSourceEstimated {
		t.Errorf("MacroSource = %q, want %q", it.MacroSource, database.MacroSourceEstimated)
	}
	// 500g protein/100g * 2.5 (250g) = 1250
	if it.ProteinGrams != 1250 {
		t.Errorf("ProteinGrams = %v, want 1250 (rescaled from the persisted estimate)", it.ProteinGrams)
	}
}

func TestFoodMeal_AggregateManualOnlyIsNonZero(t *testing.T) {
	items := []database.FoodItem{
		{MacroSource: database.MacroSourceManual, Calories: 210, FatGrams: 9},
	}
	var m database.FoodMeal
	m.Aggregate(items)
	if m.Calories == 0 {
		t.Fatal("manual-only meal aggregated to zero calories")
	}
	if m.Calories != 210 || m.FatGrams != 9 {
		t.Errorf("aggregate = %v kcal / %v g fat, want 210 / 9", m.Calories, m.FatGrams)
	}
}

func TestFoodMeal_AggregateIsIdempotent(t *testing.T) {
	items := []database.FoodItem{{MacroSource: database.MacroSourceManual, Calories: 100}}
	var m database.FoodMeal
	m.Aggregate(items)
	m.Aggregate(items)
	if m.Calories != 100 {
		t.Errorf("Calories = %v after two aggregates, want 100", m.Calories)
	}
}

// Frontend types are hand-written against snake_case field names (lib/api.ts),
// so a struct field with no json tag — which encodes as its Go name, e.g.
// "WeightGrams" instead of "weight_grams" — silently breaks every consumer.
// These guard the wire format directly rather than relying on a Go-to-Go
// round trip, which can't detect a missing tag because both sides agree on
// whatever name encoding/json picked.
func TestFoodMeal_JSONFieldsAreSnakeCase(t *testing.T) {
	m := database.FoodMeal{
		UserID: uuid.New(), Status: database.MealStatusConfirmed, LoggedAt: time.Now(),
		Name: "Lunch", ClarifyRound: 1, Calories: 100,
		Items: []database.FoodItem{{Name: "Rice", MacroSource: database.MacroSourceManual}},
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, key := range []string{
		"id", "family_id", "user_id", "status", "logged_at", "name",
		"clarify_round", "calories", "protein_grams", "carbs_grams", "fat_grams",
		"sugar_grams", "sodium_grams", "dietary_fiber_grams", "items",
	} {
		if _, ok := raw[key]; !ok {
			t.Errorf("expected JSON key %q, got keys %v", key, keysOf(raw))
		}
	}
}

func TestFoodItem_JSONFieldsAreSnakeCase(t *testing.T) {
	fdcID := int64(1)
	it := database.FoodItem{
		UserID: uuid.New(), MealID: uuid.New(), Name: "Rice",
		FdcID: &fdcID, MacroSource: database.MacroSourceReference,
		WeightGrams: 100, Calories: 130,
	}
	b, err := json.Marshal(it)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, key := range []string{
		"id", "meal_id", "name", "fdc_id", "macro_source", "weight_grams", "calories", "protein_grams",
	} {
		if _, ok := raw[key]; !ok {
			t.Errorf("expected JSON key %q, got keys %v", key, keysOf(raw))
		}
	}
}

func TestCustomFood_JSONFieldsAreSnakeCase(t *testing.T) {
	c := database.CustomFood{UserID: uuid.New(), Name: "Yogurt", CaloriesPer100g: 60}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, key := range []string{"id", "user_id", "name", "calories_per_100g", "protein_per_100g"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("expected JSON key %q, got keys %v", key, keysOf(raw))
		}
	}
}

func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
