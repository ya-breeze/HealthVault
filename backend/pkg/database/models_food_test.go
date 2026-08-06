package database_test

import (
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
