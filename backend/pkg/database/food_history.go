package database

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/google/uuid"
)

// MealHistorySuggestion is a validated source snapshot, offered before explicit reuse.
// Generic family-visible data reads do not allowlist this column.
type MealHistorySuggestion struct {
	SourceMealID   uuid.UUID               `json:"source_meal_id"`
	SourceName     string                  `json:"source_name"`
	SourceLoggedAt time.Time               `json:"source_logged_at"`
	SourceVersion  string                  `json:"source_version"`
	Mappings       []MealHistoryMapping    `json:"mappings"`
	SourceItems    []MealHistorySourceItem `json:"source_items"`
}
type MealHistorySourceItem struct {
	Name        string  `json:"name"`
	WeightGrams float64 `json:"weight_grams"`
	MacroSource string  `json:"macro_source"`
}
type MealHistoryMapping struct {
	ItemID             uuid.UUID       `json:"item_id"`
	SourceItemID       uuid.UUID       `json:"source_item_id"`
	SourceName         string          `json:"source_name"`
	Profile            NutrientProfile `json:"profile"`
	OriginalSource     string          `json:"original_source"`
	OriginalConfidence float64         `json:"original_confidence"`
}

// SetHistoryVersion covers item edits too: pending-review edits do not touch the meal.
func (m *FoodMeal) SetHistoryVersion() {
	items := append([]FoodItem(nil), m.Items...)
	sort.Slice(items, func(i, j int) bool { return items[i].ID.String() < items[j].ID.String() })
	b, _ := json.Marshal(struct {
		ID           uuid.UUID
		Status, Name string
		LoggedAt     time.Time
		Context      *MealCookingContext
		Suggestion   *MealHistorySuggestion
		Items        []FoodItem
	}{m.ID, m.Status, m.Name, m.LoggedAt, m.CookingContext, m.HistorySuggestion, items})
	m.HistoryVersion = fmt.Sprintf("%x", sha256.Sum256(b))
}

// CurrentProfile uses saved portion totals, including corrections, rather than stale AI estimates.
func (i FoodItem) CurrentProfile() (NutrientProfile, bool) {
	if !i.HasMacros() || i.WeightGrams <= 0 || math.IsInf(i.WeightGrams, 0) || math.IsNaN(i.WeightGrams) {
		return NutrientProfile{}, false
	}
	f := 100 / i.WeightGrams
	p := NutrientProfile{CaloriesPer100g: i.Calories * f, ProteinPer100g: i.ProteinGrams * f, CarbsPer100g: i.CarbsGrams * f, FatPer100g: i.FatGrams * f, SugarPer100g: i.SugarGrams * f, SodiumPer100g: i.SodiumGrams * f, DietaryFiberPer100g: i.DietaryFiberGrams * f, SaturatedFatPer100g: i.SaturatedFatGrams * f}
	return p, ValidHistoryProfile(p)
}
func ValidHistoryProfile(p NutrientProfile) bool {
	for _, v := range []float64{p.CaloriesPer100g, p.ProteinPer100g, p.CarbsPer100g, p.FatPer100g, p.SugarPer100g, p.SodiumPer100g, p.DietaryFiberPer100g, p.SaturatedFatPer100g} {
		if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}
func (i *FoodItem) ApplyHistoryProfile() bool {
	if i.HistoryProfile == nil || i.WeightGrams <= 0 || math.IsNaN(i.WeightGrams) || math.IsInf(i.WeightGrams, 0) || !ValidHistoryProfile(*i.HistoryProfile) {
		return false
	}
	i.applyScaledProfile(*i.HistoryProfile, MacroSourceHistory)
	return true
}
func (i *FoodItem) ClearHistory() {
	i.HistoryProfile = nil
	i.HistorySourceMealID = nil
	i.HistorySourceItemID = nil
	i.HistoryOriginalSource = ""
	i.HistoryOriginalConfidence = 0
}

// NutritionOrigin reports where the nutrient values came from. Recognition
// confidence on a new photo describes identity, not the copied nutrition.
func (i FoodItem) NutritionOrigin() (string, float64) {
	if i.MacroSource == MacroSourceHistory {
		return i.HistoryOriginalSource, i.HistoryOriginalConfidence
	}
	return i.MacroSource, i.Confidence
}
