package database

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// DailyTotal is one day's entry in a daily-totals range-query result
// (design.md §7 "Backend: GET /api/food/daily-totals").
type DailyTotal struct {
	Date     string  `json:"date"`
	Calories float64 `json:"calories"`
}

// DailyTotalsRange computes, for userID across the inclusive Logged-Day
// range [from, to] (both YYYY-MM-DD strings in loc — the caller is
// responsible for clamping `to` to exclude today before calling this, same
// contract as DayRange), one DailyTotal entry per day: the sum of that
// Logged Day's `confirmed`-status FoodMeal.Calories. A day with no confirmed
// meals gets a zero entry rather than being omitted, so callers can index by
// date without a presence check first.
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
	if err := db.Select("logged_at", "calories").
		Where("user_id = ? AND status = ? AND logged_at >= ? AND logged_at < ?",
			userID, MealStatusConfirmed, windowStart, windowEnd).
		Find(&meals).Error; err != nil {
		return nil, fmt.Errorf("query meals: %w", err)
	}
	caloriesByDate := make(map[string]float64, len(meals))
	for _, m := range meals {
		d := LocalDate(m.LoggedAt, loc)
		caloriesByDate[d] += m.Calories
	}

	result := make([]DailyTotal, 0, int(toDate.Sub(fromDate).Hours()/24)+1)
	for d := fromDate; !d.After(toDate); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("2006-01-02")
		result = append(result, DailyTotal{
			Date:     dateStr,
			Calories: caloriesByDate[dateStr],
		})
	}

	return result, nil
}
