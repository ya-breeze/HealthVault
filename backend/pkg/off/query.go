package off

import (
	"strings"
	"unicode"
)

// sanitizeFTSQuery tokenizes free text into safe, quoted FTS5 terms, dropping
// single-character tokens (they match nearly everything). Same tokenization
// as usda's sanitizeFTSQuery, reused here for buildBrandQuery's brand terms.
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

// buildBrandQuery constructs the brand-required FTS5 MATCH expression used
// to filter Search's results. Every one of brand's tokens is required
// together (AND) against the brands column. A brand predicate that OR's its
// own tokens together would let an unrelated product match on a single
// shared word — "brands:dr OR brands:oetker" lets an unrelated "Dr Pepper"
// product match a "Dr. Oetker" search on the shared token "dr" alone. See
// design.md "A multi-word brand's own tokens are required together (AND),
// not OR'd against each other." Returns "" when brand has no usable tokens,
// which Search treats as "no results" rather than "match on name alone" —
// the brand-required gate is the entire safety property of this query.
//
// name is deliberately not part of this query at all. An earlier version
// AND-joined a name group into the MATCH expression on the theory that OR'd
// name terms only affect ranking; that was wrong — AND-joining any group
// into MATCH makes it a filter regardless of how its own terms are joined
// internally, so a correct brand-matched product with no name-term overlap
// (e.g. a Czech "Bílý jogurt" against a recognized English "yogurt") was
// silently excluded. Name terms are scored against each brand-matched row in
// Go instead — see Search's rankByNameOverlap — which is the only way to
// make them purely ranking-only.
func buildBrandQuery(brand string) string {
	brandTerms := sanitizeFTSQuery(brand)
	if len(brandTerms) == 0 {
		return ""
	}
	brandParts := make([]string, len(brandTerms))
	for i, t := range brandTerms {
		brandParts[i] = "brands:" + t
	}
	return "(" + strings.Join(brandParts, " AND ") + ")"
}

// nameRankTerms tokenizes name the same way as sanitizeFTSQuery but returns
// plain lowercase substrings (no FTS quoting), for scoring name overlap
// against already brand-matched rows in Go rather than filtering in SQL.
func nameRankTerms(name string) []string {
	fields := strings.FieldsFunc(name, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	terms := make([]string, 0, len(fields))
	for _, f := range fields {
		if len(f) < 2 { // single characters match nearly everything
			continue
		}
		terms = append(terms, strings.ToLower(f))
	}
	return terms
}
