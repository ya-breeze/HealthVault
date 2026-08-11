package server

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const maxHintLength = 500

func normalizeHint(raw string) (string, error) {
	hint := strings.TrimSpace(raw)
	if utf8.RuneCountInString(hint) > maxHintLength {
		return "", fmt.Errorf("hint must be at most %d characters", maxHintLength)
	}
	return hint, nil
}
