package off

import (
	"strings"
	"unicode"
)

// sanitizeFTSQuery tokenizes free text into safe, quoted FTS5 terms, dropping
// single-character tokens (they match nearly everything). Same tokenization
// as usda's sanitizeFTSQuery, reused here for both the brand and name term
// groups built by buildQuery.
func sanitizeFTSQuery(s string) []string {
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
	return terms
}

// buildQuery constructs the brand-required, name-ranking FTS5 MATCH
// expression for Search.
//
// brand is mandatory: every one of its tokens is required together (AND)
// against the brands column. A brand predicate that OR's its own tokens
// together would let an unrelated product match on a single shared word —
// "brands:dr OR brands:oetker" lets an unrelated "Dr Pepper" product match a
// "Dr. Oetker" search on the shared token "dr" alone. See design.md "A
// multi-word brand's own tokens are required together (AND), not OR'd
// against each other." Returns "" when brand has no usable tokens, which
// Search treats as "no results" rather than "match on name alone" — the
// brand-required gate is the entire safety property of this query.
//
// name terms are OR'd among themselves against product_name, exactly as
// usda.QueryFor's terms are OR'd — this is ranking-only recall within the
// brand-matched set, never a filter. The two groups are joined with AND, so
// a name mismatch only re-weights results within an already brand-matched
// set; it can never let a wrong-brand row in, and it can never (on its own)
// keep every brand-matched row out either.
func buildQuery(name, brand string) string {
	brandTerms := sanitizeFTSQuery(brand)
	if len(brandTerms) == 0 {
		return ""
	}
	brandParts := make([]string, len(brandTerms))
	for i, t := range brandTerms {
		brandParts[i] = "brands:" + t
	}
	brandGroup := "(" + strings.Join(brandParts, " AND ") + ")"

	nameTerms := sanitizeFTSQuery(name)
	if len(nameTerms) == 0 {
		return brandGroup
	}
	nameParts := make([]string, len(nameTerms))
	for i, t := range nameTerms {
		nameParts[i] = "product_name:" + t
	}
	nameGroup := "(" + strings.Join(nameParts, " OR ") + ")"

	return brandGroup + " AND " + nameGroup
}
