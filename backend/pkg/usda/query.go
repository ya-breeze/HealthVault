package usda

import (
	"strings"
	"unicode"
)

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
