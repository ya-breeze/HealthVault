package server

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/usda"
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
		{"Tvorog zapekanka", "Tvorog zapekanke"},
		{"Milk 2%", "milk  2%"}, // identical after normalization
	} {
		food := database.CustomFood{Name: c.stored}
		food.ID = uuid.New()
		if _, ok := fuzzyCustomFoodMatch([]database.CustomFood{food}, c.recognized, usageByID); !ok {
			t.Errorf("fuzzyCustomFoodMatch(%q, %q) did not match; want a match", c.stored, c.recognized)
		}
	}
}

// Regression for a code-review finding: fuzzyMatchThreshold is a
// length-normalized score, so one differing character clears it in any name of
// six runes or more — and in a short food name one letter is usually what
// makes it a different food. Because a fuzzy hit binds unconditionally and
// suppresses Open Food Facts and USDA for that item, "Batter" silently
// inheriting "Butter"'s macros is a real wrong-macros outcome, not just a
// missed suggestion. See fuzzyMinNearMatchLen.
func TestFuzzyCustomFoodMatch_ShortNamesMustMatchExactly(t *testing.T) {
	usageByID := map[uuid.UUID]customFoodUsage{}
	for _, c := range []struct{ stored, recognized string }{
		{"Butter", "Batter"},
		{"Muffin", "Puffin"},
		{"Pepper", "Popper"},
		{"Borscht", "Borschk"},
	} {
		food := database.CustomFood{Name: c.stored}
		food.ID = uuid.New()
		if _, ok := fuzzyCustomFoodMatch([]database.CustomFood{food}, c.recognized, usageByID); ok {
			t.Errorf("fuzzyCustomFoodMatch(%q, %q) matched; want no match", c.stored, c.recognized)
		}
	}
}

// The length gate must not cost the near-miss matching the feature exists for:
// names long enough that a single differing character reads as a misspelling
// still match, and short names still match themselves.
func TestFuzzyCustomFoodMatch_LengthGateKeepsLongNearMissesAndShortExactMatches(t *testing.T) {
	usageByID := map[uuid.UUID]customFoodUsage{}
	for _, c := range []struct{ stored, recognized string }{
		{"Chicken breast", "Chiken breast"}, // 14 runes, one dropped letter
		{"Овсяная каша", "Овсяная кaша"},    // 12 runes, one substituted letter
		{"Butter", "  butter "},             // short, but identical once normalized
	} {
		food := database.CustomFood{Name: c.stored}
		food.ID = uuid.New()
		if _, ok := fuzzyCustomFoodMatch([]database.CustomFood{food}, c.recognized, usageByID); !ok {
			t.Errorf("fuzzyCustomFoodMatch(%q, %q) did not match; want a match", c.stored, c.recognized)
		}
	}
}

func TestIngredientCandidateMatches(t *testing.T) {
	cases := []struct {
		name, description string
		want              bool
	}{
		{"cucumber", "Cucumber, peeled, raw", true},
		// Plain English plural: description leads with "Tomatoes" against the
		// singular ingredient name "tomato" — this is exactly the case
		// stemmedWords exists for.
		{"tomato", "Tomatoes, red, ripe, raw, year round average", true},
		{"chicken breast", "Chicken, broiler or fryers, breast, skinless, boneless, meat only, raw", true},
		// Same two words present, but the base food name (the leading word,
		// before the first qualifier) is a different food — must be rejected
		// even though "chicken" and "breast" both appear somewhere in it.
		{"chicken breast", "Turkey breast, chicken-fried", false},
		// Shares one word only ("bread"/"breast" don't even share a stem) —
		// nowhere near a real match.
		{"chicken breast", "Bread, white, commercially prepared", false},
		{"", "Cucumber, peeled, raw", false},
		// Regression (found in code review): a single-word ingredient name
		// used to accept any candidate whose FIRST word matched, which a
		// processed variant's head noun satisfies just as well as the plain
		// ingredient — "Tomato sauce, canned" is not raw tomato and must be
		// rejected, even though "tomato" is both its first word and its only
		// name-derived word.
		{"tomato", "Tomato sauce, canned", false},
		{"tomato", "Tomato juice, canned, without added ascorbic acid", false},
	}
	for _, c := range cases {
		if got := ingredientCandidateMatches(c.name, c.description); got != c.want {
			t.Errorf("ingredientCandidateMatches(%q, %q) = %v, want %v", c.name, c.description, got, c.want)
		}
	}
}

// Regression: FTS5 (usda/query.go's sanitizeFTSQuery) does exact-token
// matching with no stemming, so a bare singular ingredient name like
// "tomato" returns zero USDA search results against SR Legacy's actual
// plural description ("Tomatoes, red, ripe, raw, ...") — found live while
// testing resolveIngredientReference against a real built index, not by
// inspection. ingredientSearchTerm's naive-plural hint is what fixes it.
func TestNaivePlural(t *testing.T) {
	cases := []struct{ word, want string }{
		{"tomato", "tomatoes"},
		{"potato", "potatoes"},
		{"cucumber", "cucumbers"},
		{"onion", "onions"},
		{"box", "boxes"},
		{"dish", "dishes"},
		{"branch", "branches"},
	}
	for _, c := range cases {
		if got := naivePlural(c.word); got != c.want {
			t.Errorf("naivePlural(%q) = %q, want %q", c.word, got, c.want)
		}
	}
}

// buildIngredientTestUSDAIndex mirrors food_search_test.go's buildUSDAIndex
// helper — duplicated rather than shared because that one lives in the
// separate server_test (external) package, invisible to this file's
// internal package server.
func buildIngredientTestUSDAIndex(t *testing.T, foods ...usda.Food) *usda.Index {
	t.Helper()
	target := filepath.Join(t.TempDir(), "usda.db")
	b, err := usda.NewBuilder(target)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	for _, f := range foods {
		if err := b.Add(f); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	for i := range usda.MinExpectedRows {
		f := usda.Food{
			FdcID: int64(900000 + i), Description: "Filler food item", DataType: "sr_legacy_food",
		}
		if err := b.Add(f); err != nil {
			t.Fatalf("Add filler: %v", err)
		}
	}
	if _, err := b.Promote(); err != nil {
		t.Fatalf("Promote: %v", err)
	}
	idx, err := usda.Open(target)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { idx.Close() }) //nolint:errcheck
	return idx
}

func cucumberTomatoIndex(t *testing.T) *usda.Index {
	return buildIngredientTestUSDAIndex(t,
		usda.Food{
			FdcID: 1001, Description: "Cucumber, peeled, raw", DataType: "sr_legacy_food",
			Profile: database.NutrientProfile{CaloriesPer100g: 12, ProteinPer100g: 0.7, SodiumPer100g: 0.002},
		},
		usda.Food{
			FdcID: 1002, Description: "Tomatoes, red, ripe, raw, year round average", DataType: "sr_legacy_food",
			Profile: database.NutrientProfile{CaloriesPer100g: 18, ProteinPer100g: 0.9, SodiumPer100g: 0.005},
		},
	)
}

func TestResolveIngredientReference_FullyResolvedSetsShadowProfile(t *testing.T) {
	h := NewFoodHandlers(nil, cucumberTomatoIndex(t), t.TempDir())

	item := database.FoodItem{WeightGrams: 200}
	item.SetIngredients([]database.IngredientReferenceEntry{
		{Name: "огурец", CanonicalNameEN: "cucumber", WeightGrams: 100},
		{Name: "помидор", CanonicalNameEN: "tomato", WeightGrams: 100},
	})

	h.resolveIngredientReference(&item)

	if !item.HasIngredientReference {
		t.Fatalf("expected HasIngredientReference=true for two fully-resolved ingredients")
	}
	// Equal weights, so the combined per-100g figure is the plain average of
	// the two per-100g profiles: (12+18)/2 = 15 kcal, (0.7+0.9)/2 = 0.8g protein.
	if got, want := item.IngredientReferenceCaloriesPer100g, 15.0; got != want {
		t.Errorf("IngredientReferenceCaloriesPer100g = %v, want %v", got, want)
	}
	if got, want := item.IngredientReferenceProteinPer100g, 0.8; got != want {
		t.Errorf("IngredientReferenceProteinPer100g = %v, want %v", got, want)
	}

	entries, ok := item.Ingredients()
	if !ok || len(entries) != 2 {
		t.Fatalf("Ingredients() = %v, %v; want 2 entries", entries, ok)
	}
	for _, e := range entries {
		if !e.Resolved || e.FdcID == nil {
			t.Errorf("entry %+v: want Resolved=true with a non-nil FdcID", e)
		}
	}
}

func TestResolveIngredientReference_PartialResolutionLeavesNoShadowProfile(t *testing.T) {
	h := NewFoodHandlers(nil, cucumberTomatoIndex(t), t.TempDir())

	item := database.FoodItem{WeightGrams: 200}
	item.SetIngredients([]database.IngredientReferenceEntry{
		{Name: "огурец", CanonicalNameEN: "cucumber", WeightGrams: 100},
		{Name: "неизвестный ингредиент", CanonicalNameEN: "zzzznotarealfood", WeightGrams: 100},
	})

	h.resolveIngredientReference(&item)

	if item.HasIngredientReference {
		t.Fatalf("expected HasIngredientReference=false when only one of two ingredients resolved")
	}
	entries, ok := item.Ingredients()
	if !ok || len(entries) != 2 {
		t.Fatalf("Ingredients() = %v, %v; want 2 entries (partial resolution must still persist)", entries, ok)
	}
	if !entries[0].Resolved {
		t.Errorf("entries[0] (cucumber) should have resolved")
	}
	if entries[1].Resolved {
		t.Errorf("entries[1] (zzzznotarealfood) should not have resolved")
	}
}

// Regression (found in code review): a blank canonical_name_en used to be
// dropped by toIngredientEstimates before it ever reached persistence,
// shrinking a two-ingredient dish down to one and letting the
// all-or-nothing gate see it as "fully resolved". It must instead survive
// into the persisted breakdown and simply fail its own USDA search, keeping
// HasIngredientReference false for the whole item.
func TestResolveIngredientReference_BlankCanonicalNameBlocksTheGate(t *testing.T) {
	h := NewFoodHandlers(nil, cucumberTomatoIndex(t), t.TempDir())

	item := database.FoodItem{WeightGrams: 200}
	item.SetIngredients([]database.IngredientReferenceEntry{
		{Name: "огурец", CanonicalNameEN: "cucumber", WeightGrams: 100},
		{Name: "?", CanonicalNameEN: "", WeightGrams: 100},
	})

	h.resolveIngredientReference(&item)

	if item.HasIngredientReference {
		t.Fatalf("expected HasIngredientReference=false with a blank-named ingredient present")
	}
	entries, ok := item.Ingredients()
	if !ok || len(entries) != 2 {
		t.Fatalf("Ingredients() = %v, %v; want both entries preserved", entries, ok)
	}
	if entries[1].Resolved {
		t.Errorf("the blank-named entry should never resolve")
	}
}

// Regression (found in code review): a zero-weight ingredient's identity
// match used to still increment resolvedCount even though it contributed
// nothing to the sum, letting HasIngredientReference become true over a
// dish where one ingredient's actual mass was never accounted for.
func TestResolveIngredientReference_ZeroWeightIngredientBlocksTheGate(t *testing.T) {
	h := NewFoodHandlers(nil, cucumberTomatoIndex(t), t.TempDir())

	item := database.FoodItem{WeightGrams: 200}
	item.SetIngredients([]database.IngredientReferenceEntry{
		{Name: "огурец", CanonicalNameEN: "cucumber", WeightGrams: 100},
		{Name: "помидор", CanonicalNameEN: "tomato", WeightGrams: 0},
	})

	h.resolveIngredientReference(&item)

	if item.HasIngredientReference {
		t.Fatalf("expected HasIngredientReference=false with a zero-weight ingredient present")
	}
	entries, ok := item.Ingredients()
	if !ok || len(entries) != 2 {
		t.Fatalf("Ingredients() = %v, %v; want both entries preserved", entries, ok)
	}
	if entries[1].Resolved {
		t.Errorf("a zero-weight entry should never be marked Resolved, even though its identity is findable")
	}
}

func TestResolveIngredientReference_NoIngredientsIsNoOp(t *testing.T) {
	h := NewFoodHandlers(nil, cucumberTomatoIndex(t), t.TempDir())
	item := database.FoodItem{WeightGrams: 200}
	h.resolveIngredientReference(&item) // atomic item, no breakdown — must not panic or set anything
	if item.HasIngredientReference || item.IngredientsJSON != "" {
		t.Fatalf("expected no-op for an item with no ingredient breakdown, got %+v", item)
	}
}

func TestResolveIngredientReference_NilUSDAIndexIsNoOp(t *testing.T) {
	h := NewFoodHandlers(nil, nil, t.TempDir())
	item := database.FoodItem{WeightGrams: 200}
	item.SetIngredients([]database.IngredientReferenceEntry{
		{Name: "огурец", CanonicalNameEN: "cucumber", WeightGrams: 100},
	})
	h.resolveIngredientReference(&item) // must not panic with h.usda == nil
	if item.HasIngredientReference {
		t.Fatalf("expected no resolution to happen with a nil USDA index")
	}
}
