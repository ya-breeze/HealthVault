package server

import (
	"math"
	"testing"
)

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-6
}

// The standard case: the fat-floor never engages, so carbs/fat come straight
// from the 50/50 kcal split of the remaining calories after protein.
func TestComputeNutritionTarget_StandardCase(t *testing.T) {
	calories, protein, carbs, fat := computeNutritionTarget(
		80,     // weightKg
		1.80,   // heightM
		30,     // ageYears
		"male", // sex
		1.55,   // activityMultiplier (moderate)
		75,     // goalWeightKg
	)

	// BMR = 10*80 + 6.25*180 - 5*30 + 5 = 1780; calories = 1780*1.55 = 2759
	if !almostEqual(calories, 2759) {
		t.Errorf("calories = %v, want 2759", calories)
	}
	// protein = 1.6*75 = 120
	if !almostEqual(protein, 120) {
		t.Errorf("protein = %v, want 120", protein)
	}
	// remaining = 2759 - 120*4 = 2279; carbs = 2279/2/4, fat = 2279/2/9
	if !almostEqual(carbs, 284.875) {
		t.Errorf("carbs = %v, want 284.875", carbs)
	}
	if !almostEqual(fat, 2279.0/18.0) {
		t.Errorf("fat = %v, want %v (fat floor of 60 must not engage)", fat, 2279.0/18.0)
	}
}

// The fat-floor-engaged case: the 50/50 split would put fat below
// 0.8g/kg-goal, so fat is pinned to the floor and carbs absorb the
// difference — but carbs stay positive here.
func TestComputeNutritionTarget_FatFloorEngaged(t *testing.T) {
	calories, protein, carbs, fat := computeNutritionTarget(
		55,       // weightKg
		1.50,     // heightM
		45,       // ageYears
		"female", // sex
		1.2,      // activityMultiplier (sedentary)
		70,       // goalWeightKg
	)

	// BMR = 10*55 + 6.25*150 - 5*45 - 161 = 1101.5; calories = 1101.5*1.2 = 1321.8
	if !almostEqual(calories, 1321.8) {
		t.Errorf("calories = %v, want 1321.8", calories)
	}
	// protein = 1.6*70 = 112
	if !almostEqual(protein, 112) {
		t.Errorf("protein = %v, want 112", protein)
	}
	// fat floor = 0.8*70 = 56; unfloored fat would be 873.8/18 = 48.54 < 56
	if !almostEqual(fat, 56) {
		t.Errorf("fat = %v, want 56 (the floor value)", fat)
	}
	// carbs recomputed from the remaining kcal after the floored fat:
	// (873.8 - 56*9)/4 = 92.45
	if !almostEqual(carbs, 92.45) {
		t.Errorf("carbs = %v, want 92.45", carbs)
	}
}

// When protein alone meets or exceeds calories, the floor-recomputed carbs
// go negative and must clamp to 0, not report a negative gram value.
func TestComputeNutritionTarget_ProteinExceedsCaloriesClampsCarbsToZero(t *testing.T) {
	calories, protein, carbs, fat := computeNutritionTarget(
		40,       // weightKg
		1.40,     // heightM
		80,       // ageYears
		"female", // sex
		1.2,      // activityMultiplier (sedentary)
		150,      // goalWeightKg (drives protein well above calories)
	)

	// BMR = 10*40 + 6.25*140 - 5*80 - 161 = 714; calories = 714*1.2 = 856.8
	if !almostEqual(calories, 856.8) {
		t.Errorf("calories = %v, want 856.8", calories)
	}
	// protein = 1.6*150 = 240; proteinKcal = 960 > calories(856.8)
	if !almostEqual(protein, 240) {
		t.Errorf("protein = %v, want 240", protein)
	}
	if !almostEqual(fat, 120) {
		t.Errorf("fat = %v, want 120 (the floor value: 0.8*150)", fat)
	}
	if carbs != 0 {
		t.Errorf("carbs = %v, want 0 (clamped, not negative)", carbs)
	}
}
