package usda_test

import (
	"testing"

	"github.com/ya-breeze/healthvault/pkg/usda"
)

func TestQueryFor(t *testing.T) {
	tests := []struct {
		name, prep, state, want string
	}{
		{"chicken breast", usda.PrepRoasted, usda.StateCooked, "chicken breast roasted cooked"},
		{"chicken breast", usda.PrepUnknown, usda.StateUnknown, "chicken breast"},
		{"white rice", usda.PrepUnknown, usda.StateCooked, "white rice cooked"},
		{"chicken", usda.PrepBreadedFried, usda.StateCooked, "chicken breaded_fried cooked"},
		{"  spaced  ", "  ", "", "spaced"},
	}
	for _, tc := range tests {
		if got := usda.QueryFor(tc.name, tc.prep, tc.state); got != tc.want {
			t.Errorf("QueryFor(%q,%q,%q) = %q, want %q", tc.name, tc.prep, tc.state, got, tc.want)
		}
	}
}

// The point of the change: preparation tokens must actually move the canonical
// whole-food entry up past the short processed ones.
func TestQueryFor_PreparationImprovesRanking(t *testing.T) {
	const canonical = "Chicken, broilers or fryers, breast, meat only, cooked, roasted"
	idx, err := usda.Open(buildIndex(t,
		food(1, canonical, 165),
		food(2, "Chicken breast tenders, breaded, uncooked", 263),
		food(3, "Chicken breast, roll, oven-roasted", 134),
		food(4, "Chicken breast, deli, rotisserie seasoned, sliced, prepackaged", 98),
	))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer idx.Close() //nolint:errcheck

	rankOf := func(q string) int {
		res, err := idx.Search(q, usda.DefaultCandidates)
		if err != nil {
			t.Fatalf("Search(%q): %v", q, err)
		}
		for i, f := range res {
			if f.Description == canonical {
				return i + 1
			}
		}
		return -1
	}

	bare := rankOf(usda.QueryFor("chicken breast", usda.PrepUnknown, usda.StateUnknown))
	withPrep := rankOf(usda.QueryFor("chicken breast", usda.PrepRoasted, usda.StateCooked))

	if withPrep < 1 {
		t.Fatal("canonical food absent from results even with preparation")
	}
	if withPrep > bare {
		t.Errorf("preparation made ranking worse: bare=%d withPrep=%d", bare, withPrep)
	}
}

// Preparation is a ranking hint, never a filter: a wrong guess must not be able
// to exclude the correct food from the shortlist.
func TestQueryFor_WrongPreparationDoesNotExcludeCorrectFood(t *testing.T) {
	const canonical = "Chicken, broilers or fryers, breast, meat only, cooked, roasted"
	idx, err := usda.Open(buildIndex(t,
		food(1, canonical, 165),
		food(2, "Chicken breast tenders, breaded, uncooked", 263),
	))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer idx.Close() //nolint:errcheck

	// The model guesses "grilled" for something that is actually roasted.
	res, err := idx.Search(usda.QueryFor("chicken breast", usda.PrepGrilled, usda.StateCooked), usda.DefaultCandidates)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	for _, f := range res {
		if f.Description == canonical {
			return
		}
	}
	t.Error("a wrong preparation guess removed the correct food from the shortlist")
}

// An unknown preparation must degrade to a name-only query, not to an empty one.
func TestQueryFor_UnknownStillRetrieves(t *testing.T) {
	idx, err := usda.Open(buildIndex(t, food(1, "Rice, white, long-grain, cooked", 130)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer idx.Close() //nolint:errcheck

	res, err := idx.Search(usda.QueryFor("white rice", usda.PrepUnknown, usda.StateUnknown), usda.DefaultCandidates)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res) == 0 {
		t.Error("name-only query returned nothing")
	}
}
