package database

import (
	"sort"
	"time"
)

// occasionGapThreshold is the maximum gap between consecutive LoggedAt
// timestamps that still counts as the same Eating Occasion. Exactly this gap
// merges (inclusive boundary); anything larger starts a new occasion. See
// design.md §1 "Eating Occasion collapsing" under
// openspec/changes/food-day-completeness.
const occasionGapThreshold = 10 * time.Minute

// CollapseOccasions groups a day's FoodMeal.LoggedAt timestamps into Eating
// Occasions: sorted ascending, a new occasion starts whenever the gap to the
// previous timestamp exceeds occasionGapThreshold, otherwise the timestamp
// merges into the current occasion. Returns the resulting occasion count.
//
// This is the fix for the trap case where a single sitting logged as
// multiple rows a few minutes apart would otherwise inflate the meal count
// (design.md §1): 2026-08-21's three rows at 09:1x/14:39/14:42 collapse to 2
// occasions, not 3.
func CollapseOccasions(loggedAt []time.Time) int {
	if len(loggedAt) == 0 {
		return 0
	}

	sorted := make([]time.Time, len(loggedAt))
	copy(sorted, loggedAt)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Before(sorted[j]) })

	count := 1
	for i := 1; i < len(sorted); i++ {
		if sorted[i].Sub(sorted[i-1]) > occasionGapThreshold {
			count++
		}
	}
	return count
}
