package usda

import (
	"strings"
	"unicode"
)

// Preparation and state vocabularies emitted by the vision model. Both may be
// Unknown, which simply contributes no token to the query.
const (
	PrepUnknown      = ""
	PrepRaw          = "raw"
	PrepBoiled       = "boiled"
	PrepSteamed      = "steamed"
	PrepRoasted      = "roasted"
	PrepBaked        = "baked"
	PrepGrilled      = "grilled"
	PrepFried        = "fried"
	PrepBreadedFried = "breaded_fried"
	PrepBraised      = "braised"

	StateUnknown = ""
	StateRaw     = "raw"
	StateCooked  = "cooked"
)

// QueryFor builds the retrieval term for a recognized item.
//
// Preparation and state are included because SR Legacy encodes them as trailing
// description qualifiers — "Chicken, broilers or fryers, breast, meat only,
// cooked, roasted" — so a name-only query leaves those tokens unused and lets
// short processed entries outrank the canonical whole food. Measured on the real
// dataset, adding the preparation moved the correct food from rank 12 to rank 3
// for "chicken breast"; adding the state matters even more for magnitude, since
// raw white rice is ~360 kcal/100g against ~130 cooked.
//
// These are ranking hints, never filters. Terms are OR-joined, so a wrong guess
// ("grilled" for something pan-fried) only re-weights results — it can never
// exclude the correct food from the shortlist. A SQL filter on the model's guess
// would be the obvious alternative and would delete the right answer outright.
//
// breaded_fried is split on the underscore by the tokenizer, contributing both
// "breaded" and "fried", which is the desired behaviour.
func QueryFor(name, preparation, state string) string {
	parts := make([]string, 0, 3)
	for _, p := range []string{name, preparation, state} {
		if s := strings.TrimSpace(p); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " ")
}

// sanitizeFTSQuery turns free-text into a safe FTS5 MATCH expression.
//
// Two reasons this is not optional. First, FTS5 MATCH has its own syntax —
// bare punctuation, quotes or a trailing operator raise a syntax error rather
// than returning no rows, so unsanitized user text turns a miss into a 500.
// Second, terms are joined with OR rather than implicit AND: a vision model
// emits "grilled chicken breast" while the matching SR Legacy row reads
// "Chicken, broilers or fryers, breast, meat only, cooked, roasted". Requiring
// every term would return nothing at all. OR keeps recall, and bm25 still ranks
// rows matching more terms higher — which is what a candidate shortlist wants.
func sanitizeFTSQuery(s string) string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	terms := make([]string, 0, len(fields))
	for _, f := range fields {
		if len(f) < 2 { // single characters match nearly everything
			continue
		}
		terms = append(terms, `"`+strings.ToLower(f)+`"`)
	}
	return strings.Join(terms, " OR ")
}
