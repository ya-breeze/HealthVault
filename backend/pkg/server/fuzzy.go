package server

import "strings"

// fuzzyMatchThreshold decides whether a candidate custom food is offered as
// the sole (deterministic) match for a recognized item's Display Name — see
// openspec/changes/russian-localization/design.md decision 5. Tunable: start
// conservative to avoid false-positive reuse (e.g. "chicken soup" matching
// "chicken salad").
const fuzzyMatchThreshold = 0.82

// normalizeForFuzzyMatch trims, lowercases, and collapses internal
// whitespace so formatting differences alone (extra spaces, mixed case)
// don't affect similarity scoring.
func normalizeForFuzzyMatch(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// levenshteinDistance is the classic edit distance between a and b, operated
// on runes (not bytes) so multi-byte text (e.g. Cyrillic) is measured
// correctly. O(len(a)*len(b)) time, O(min(len(a),len(b))) space via a rolling
// two-row table — fine at the catalog sizes this runs against (one user's
// own custom foods, realistically dozens).
func levenshteinDistance(a, b string) int {
	ar, br := []rune(a), []rune(b)
	if len(ar) > len(br) {
		ar, br = br, ar // shorter one drives row width
	}
	la, lb := len(ar), len(br)
	if la == 0 {
		return lb
	}
	prev := make([]int, la+1)
	curr := make([]int, la+1)
	for j := 0; j <= la; j++ {
		prev[j] = j
	}
	for i := 1; i <= lb; i++ {
		curr[0] = i
		for j := 1; j <= la; j++ {
			cost := 1
			if br[i-1] == ar[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			m := del
			if ins < m {
				m = ins
			}
			if sub < m {
				m = sub
			}
			curr[j] = m
		}
		prev, curr = curr, prev
	}
	return prev[la]
}

// fuzzySimilarity is a normalized 0..1 score: 1 for identical (after
// normalization) strings, decreasing toward 0 as edit distance grows
// relative to the longer normalized string's length.
func fuzzySimilarity(a, b string) float64 {
	na, nb := normalizeForFuzzyMatch(a), normalizeForFuzzyMatch(b)
	if na == nb {
		return 1
	}
	maxLen := len([]rune(na))
	if l := len([]rune(nb)); l > maxLen {
		maxLen = l
	}
	if maxLen == 0 {
		return 1
	}
	return 1 - float64(levenshteinDistance(na, nb))/float64(maxLen)
}

// digitsIn returns just the digit runes of s, in order. Used by
// sameDigitsIn below.
func digitsIn(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// sameDigitsIn reports whether a and b carry the same digits in the same
// order. A fuzzy custom-food match binds unconditionally (see
// fuzzyMatchThreshold and food_upload.go's retrieveCandidates), so a
// false positive silently attaches the wrong macros with no alternative
// offered — and names that differ *only* in a number are exactly the case
// plain edit distance handles worst: "Milk 2%" vs "Milk 3%" scores 0.857 and
// "Cola 0.5l" vs "Cola 1.5l" 0.889, both comfortably over the threshold,
// while being different foods with materially different macros. Digits in a
// food name are near-always a defining quantity (fat percentage, volume,
// strength), not incidental spelling, so a mismatch here vetoes the match
// rather than merely lowering its score. Names with no digits at all are
// unaffected (both sides yield ""). Found in code review.
func sameDigitsIn(a, b string) bool {
	return digitsIn(a) == digitsIn(b)
}

// fuzzyMinNearMatchLen is the shortest normalized name for which a *near*
// match — anything scoring below 1 — is accepted at all. Below it, only names
// that are identical after normalization (case and whitespace differences
// alone) match.
//
// fuzzyMatchThreshold is a length-normalized score, so the number of
// differing characters it tolerates grows with the name: at 6 runes it admits
// any single-character difference (0.833), at 12 it admits two (also 0.833).
// That scaling is backwards for short names, where one letter is usually what
// distinguishes two different foods rather than a typo — "Butter"/"Batter",
// "Muffin"/"Puffin" and "Pepper"/"Popper" all score 0.833 and all clear the
// threshold today. A hit here is offered as the sole candidate with
// unconditional-bind weight and suppresses Open Food Facts and USDA for that
// item (see retrieveCandidates), so a false positive silently attaches the
// wrong macros with no alternative offered, while a false negative only costs
// the user a manual resolve — an asymmetry that argues for being strict
// exactly where the score is least trustworthy. Ten runes is where one
// differing character starts to read as a misspelling rather than a different
// word ("chicken breast"/"chiken breast"). Found in code review.
const fuzzyMinNearMatchLen = 10

// nearMatchAllowed reports whether a and b are long enough for a below-perfect
// similarity score between them to be trusted — see fuzzyMinNearMatchLen.
// Measured on the shorter of the two normalized names, so pairing a short name
// with a long one can't buy tolerance the short one hasn't earned.
func nearMatchAllowed(a, b string) bool {
	n := len([]rune(normalizeForFuzzyMatch(a)))
	if m := len([]rune(normalizeForFuzzyMatch(b))); m < n {
		n = m
	}
	return n >= fuzzyMinNearMatchLen
}
