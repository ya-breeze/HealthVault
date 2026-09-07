package database

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// DailyTotal is one day's entry in a daily-totals range-query result
// (design.md §7 "Backend: GET /api/food/daily-totals"). UnconfirmedMeals is
// how many of that day's FoodMeal rows are in a status other than
// `confirmed` and therefore contributed nothing to Calories — the signal a
// consumer needs to tell "this day really was a 0 kcal day" apart from "this
// day's total is missing meals I can't see."
type DailyTotal struct {
	Date     string  `json:"date"`
	Calories float64 `json:"calories"`
	// ProteinGrams, CarbsGrams, FatGrams, SugarGrams and SodiumGrams are summed
	// the same way, and over the same `confirmed`-status meals, as Calories.
	// They carry no `omitempty`: a day with confirmed meals whose sum is
	// exactly zero (or no confirmed meals at all) is a legitimate zero, not an
	// absence, and `omitempty` would drop the key — a consumer that types the
	// field as required would then read `undefined` and render `NaN`, the same
	// trap `summaryTargetPayload` (summary_today.go) hit first.
	ProteinGrams     float64 `json:"protein_grams"`
	CarbsGrams       float64 `json:"carbs_grams"`
	FatGrams         float64 `json:"fat_grams"`
	SugarGrams       float64 `json:"sugar_grams"`
	SodiumGrams      float64 `json:"sodium_grams"`
	UnconfirmedMeals int     `json:"unconfirmed_meals"`
}

// DailyTotalsRange computes, for userID across the inclusive Logged-Day
// range [from, to] (both YYYY-MM-DD strings in loc — the caller is
// responsible for clamping `to` to exclude today before calling this, same
// contract as DayRange), one DailyTotal entry per day: the sum of that
// Logged Day's `confirmed`-status FoodMeal.Calories, plus a count of that
// day's rows in any other status. A day with no confirmed meals gets a zero
// entry rather than being omitted, so callers can index by date without a
// presence check first.
//
// The query itself is deliberately not status-filtered — DayRange
// (food_completeness.go) counts Eating Occasions across every status, so a
// day can be Complete on its numbers while some of its meals never reached
// `confirmed` and contributed no calories here. A consumer that joins the
// two results (the Logging Gap computation does) would otherwise read that
// day's under-counted total as fact. Reporting the non-confirmed count
// alongside the sum lets it exclude such a day instead.
func DailyTotalsRange(
	db *gorm.DB, userID uuid.UUID, loc *time.Location, from, to string,
) ([]DailyTotal, error) {
	fromDate, err := time.ParseInLocation("2006-01-02", from, loc)
	if err != nil {
		return nil, fmt.Errorf("parse from date %q: %w", from, err)
	}
	toDate, err := time.ParseInLocation("2006-01-02", to, loc)
	if err != nil {
		return nil, fmt.Errorf("parse to date %q: %w", to, err)
	}
	if toDate.Before(fromDate) {
		return nil, nil
	}

	// Exclusive upper bound: the instant the day after `to` begins in loc.
	// Same UTC-normalization rationale as DayRange (food_completeness.go):
	// FoodMeal.LoggedAt is stored UTC-normalized, and go-sqlite3 stores
	// time.Time as TEXT preserving whatever offset it's given, so the window
	// bound must itself be UTC-offset to compare correctly.
	windowStart, windowEnd := fromDate.UTC(), toDate.AddDate(0, 0, 1).UTC()

	var meals []FoodMeal
	if err := db.Select(
		"logged_at", "calories", "protein_grams", "carbs_grams", "fat_grams",
		"sugar_grams", "sodium_grams", "status",
	).
		Where("user_id = ? AND logged_at >= ? AND logged_at < ?",
			userID, windowStart, windowEnd).
		Find(&meals).Error; err != nil {
		return nil, fmt.Errorf("query meals: %w", err)
	}
	sumsByDate := make(map[string]dailyMealSums, len(meals))
	unconfirmedByDate := make(map[string]int, len(meals))
	for _, m := range meals {
		d := LocalDate(m.LoggedAt, loc)
		if m.Status == MealStatusConfirmed {
			s := sumsByDate[d]
			s.calories += m.Calories
			s.proteinGrams += m.ProteinGrams
			s.carbsGrams += m.CarbsGrams
			s.fatGrams += m.FatGrams
			s.sugarGrams += m.SugarGrams
			s.sodiumGrams += m.SodiumGrams
			sumsByDate[d] = s
			continue
		}
		unconfirmedByDate[d]++
	}

	result := make([]DailyTotal, 0, int(toDate.Sub(fromDate).Hours()/24)+1)
	for d := fromDate; !d.After(toDate); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("2006-01-02")
		s := sumsByDate[dateStr]
		result = append(result, DailyTotal{
			Date:             dateStr,
			Calories:         s.calories,
			ProteinGrams:     s.proteinGrams,
			CarbsGrams:       s.carbsGrams,
			FatGrams:         s.fatGrams,
			SugarGrams:       s.sugarGrams,
			SodiumGrams:      s.sodiumGrams,
			UnconfirmedMeals: unconfirmedByDate[dateStr],
		})
	}

	return result, nil
}

// dailyMealSums accumulates one Logged Day's confirmed-meal sums — grouped
// into one map value rather than five parallel maps, since every field is
// written and read together.
type dailyMealSums struct {
	calories     float64
	proteinGrams float64
	carbsGrams   float64
	fatGrams     float64
	sugarGrams   float64
	sodiumGrams  float64
}
