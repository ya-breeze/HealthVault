package server

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ya-breeze/healthvault/pkg/database"
)

// Regression for a code-review finding: a tied fuzzy score with no usage
// history to break it (both candidates never confirmed in a meal, so both
// have a zero usageByID LastUsed) used to fall through to whichever food
// customFoodsForUser happened to return first — unordered DB row order, not
// a stable rule. fuzzyCustomFoodMatch must be deterministic across repeated,
// otherwise-identical calls, the same way rankedCustomFoodCandidates already
// is for its own tie-break.
func TestFuzzyCustomFoodMatch_TiedScoreAndUsageIsDeterministic(t *testing.T) {
	a := database.CustomFood{Name: "Tvorog"}
	a.ID = uuid.MustParse("00000000-0000-0000-0000-000000000001")
	b := database.CustomFood{Name: "Tvorog"}
	b.ID = uuid.MustParse("00000000-0000-0000-0000-000000000002")

	usageByID := map[uuid.UUID]customFoodUsage{} // neither food has ever been used

	got1, ok1 := fuzzyCustomFoodMatch([]database.CustomFood{a, b}, "Tvorog", usageByID)
	got2, ok2 := fuzzyCustomFoodMatch([]database.CustomFood{b, a}, "Tvorog", usageByID)
	if !ok1 || !ok2 {
		t.Fatalf("expected a match in both orderings, got ok1=%v ok2=%v", ok1, ok2)
	}
	if got1.ID != a.ID || got2.ID != a.ID {
		t.Fatalf("expected the lexicographically-smaller ID to win deterministically regardless of input order, got %s and %s", got1.ID, got2.ID)
	}
}

func TestFuzzyCustomFoodMatch_TiedScoreBrokenByMostRecentlyUsed(t *testing.T) {
	a := database.CustomFood{Name: "Tvorog"}
	a.ID = uuid.MustParse("00000000-0000-0000-0000-000000000009") // would lose the ID tie-break
	b := database.CustomFood{Name: "Tvorog"}
	b.ID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

	now := time.Now()
	usageByID := map[uuid.UUID]customFoodUsage{
		a.ID: {LastUsed: now},                           // used just now
		b.ID: {LastUsed: now.Add(-30 * 24 * time.Hour)}, // used a month ago
	}

	got, ok := fuzzyCustomFoodMatch([]database.CustomFood{a, b}, "Tvorog", usageByID)
	if !ok {
		t.Fatal("expected a match")
	}
	if got.ID != a.ID {
		t.Fatalf("expected most-recently-used to win the tie ahead of the ID tie-break, got %s", got.ID)
	}
}
