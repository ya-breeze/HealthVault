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

// Regression for a code-review finding: a fuzzy custom-food hit is returned
// as the sole candidate and binds unconditionally (see retrieveCandidates),
// so a false positive attaches the wrong macros with no alternative offered.
// Names differing only in a number — a fat percentage, a volume, a strength —
// are exactly what plain edit distance handles worst: they score well above
// fuzzyMatchThreshold while being materially different foods. sameDigitsIn
// vetoes those regardless of score.
func TestFuzzyCustomFoodMatch_DigitOnlyDifferenceIsNotAMatch(t *testing.T) {
	usageByID := map[uuid.UUID]customFoodUsage{}
	for _, c := range []struct{ stored, recognized string }{
		{"Milk 2%", "Milk 3%"},
		{"Cola 0.5l", "Cola 1.5l"},
		{"Beer 4.5%", "Beer 6.5%"},
	} {
		food := database.CustomFood{Name: c.stored}
		food.ID = uuid.New()
		if _, ok := fuzzyCustomFoodMatch([]database.CustomFood{food}, c.recognized, usageByID); ok {
			t.Errorf("fuzzyCustomFoodMatch(%q, %q) matched; want no match", c.stored, c.recognized)
		}
	}
}

// The veto above must not cost ordinary matches: names with no digits at all,
// and names whose digits agree, still match on similarity as before.
func TestFuzzyCustomFoodMatch_DigitVetoDoesNotBlockGenuineMatches(t *testing.T) {
	usageByID := map[uuid.UUID]customFoodUsage{}
	for _, c := range []struct{ stored, recognized string }{
		{"Tvorog", "Tvorogh"},
		{"Milk 2%", "milk  2%"},
	} {
		food := database.CustomFood{Name: c.stored}
		food.ID = uuid.New()
		if _, ok := fuzzyCustomFoodMatch([]database.CustomFood{food}, c.recognized, usageByID); !ok {
			t.Errorf("fuzzyCustomFoodMatch(%q, %q) did not match; want a match", c.stored, c.recognized)
		}
	}
}
