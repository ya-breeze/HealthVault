package server

import (
	"testing"

	"github.com/ya-breeze/healthvault/pkg/database"
)

// saturated_fat must be wired into the same three seams fiber was: the
// explain_nutrition_signal allowlist, and both meal/item value lookups.
func TestNutritionSignalValue_SaturatedFat(t *testing.T) {
	if !nutritionHistorySignals["saturated_fat"] {
		t.Fatal("expected saturated_fat to be an accepted explain_nutrition_signal signal")
	}
	meal := database.FoodMeal{SaturatedFatGrams: 7.5}
	if got := nutritionSignalMealValue(meal, "saturated_fat"); got != 7.5 {
		t.Errorf("nutritionSignalMealValue(saturated_fat) = %v, want 7.5", got)
	}
	item := database.FoodItem{SaturatedFatGrams: 3.25}
	if got := nutritionSignalItemValue(item, "saturated_fat"); got != 3.25 {
		t.Errorf("nutritionSignalItemValue(saturated_fat) = %v, want 3.25", got)
	}
}
