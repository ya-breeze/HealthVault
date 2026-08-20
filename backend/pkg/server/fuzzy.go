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
