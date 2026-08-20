package server

import "testing"

func TestFuzzySimilarity_IdenticalStringsScoreOne(t *testing.T) {
	if got := fuzzySimilarity("chicken soup", "chicken soup"); got != 1 {
		t.Errorf("similarity = %v, want 1", got)
	}
}

func TestFuzzySimilarity_CaseAndWhitespaceInsensitive(t *testing.T) {
	if got := fuzzySimilarity("Chicken   Soup", "chicken soup"); got != 1 {
		t.Errorf("similarity = %v, want 1 (normalization should erase case/whitespace differences)", got)
	}
}

func TestFuzzySimilarity_NearMissScoresHigh(t *testing.T) {
	got := fuzzySimilarity("chicken soup", "chiken soup") // one dropped letter
	if got < fuzzyMatchThreshold {
		t.Errorf("similarity = %v, want >= threshold %v for a one-letter typo", got, fuzzyMatchThreshold)
	}
}

func TestFuzzySimilarity_DifferentFoodsScoreLow(t *testing.T) {
	got := fuzzySimilarity("chicken soup", "chicken salad")
	if got >= fuzzyMatchThreshold {
		t.Errorf("similarity = %v, want < threshold %v for a different dish", got, fuzzyMatchThreshold)
	}
}

func TestFuzzySimilarity_EmptyStringsScoreOne(t *testing.T) {
	if got := fuzzySimilarity("", ""); got != 1 {
		t.Errorf("similarity = %v, want 1", got)
	}
}

func TestFuzzySimilarity_CyrillicRunesMeasuredCorrectly(t *testing.T) {
	// Each Cyrillic character is multiple UTF-8 bytes; a byte-based distance
	// would wildly overcount edits on this pair.
	got := fuzzySimilarity("вареники", "варeники") // identical length, one substituted letter
	if got < fuzzyMatchThreshold {
		t.Errorf("similarity = %v, want >= threshold for a one-rune substitution", got)
	}
	if got == 1 {
		t.Errorf("similarity = 1, want < 1 since the strings differ by one rune")
	}
}

func TestLevenshteinDistance_KnownPairs(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "abc", 0},
		{"", "abc", 3},
		{"kitten", "sitting", 3},
		{"flaw", "lawn", 2},
	}
	for _, c := range cases {
		if got := levenshteinDistance(c.a, c.b); got != c.want {
			t.Errorf("levenshteinDistance(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
