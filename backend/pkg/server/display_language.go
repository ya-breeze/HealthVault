package server

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/ya-breeze/healthvault/pkg/database"
)

// defaultDisplayLanguage is used when a user has never saved a
// display_language setting, or their settings document doesn't parse.
const defaultDisplayLanguage = "en"

// bcp47Tag matches the shape of a BCP-47 language tag: a 2-8 letter primary
// subtag optionally followed by alphanumeric subtags. Deliberately a shape
// check, not a registry lookup — it does not care whether the tag names a
// real language, only that the value is a language tag rather than arbitrary
// text. "_" is accepted alongside the canonical "-" separator so this stays
// no stricter than IsEnglishDisplayLanguage, which already splits on either.
var bcp47Tag = regexp.MustCompile(`^[A-Za-z]{2,8}([-_][A-Za-z0-9]{1,8})*$`)

// normalizeDisplayLanguage trims a stored display_language and rejects
// anything that isn't shaped like a BCP-47 tag, falling back to the default.
//
// UserSettings is an opaque blob the PUT handler stores verbatim (it only
// caps the body size), so this value is caller-controlled text that reaches
// two places where unvalidated input matters: it is interpolated into the
// vision system prompt (see vision.languageDirective), where a multi-kilobyte
// value would ship arbitrary instructions to the model on every recognition
// call; and it gates reference-DB matching (see IsEnglishDisplayLanguage),
// where a value like "en " — English with a stray trailing space — parses as
// some non-English language and silently switches USDA/Open Food Facts
// matching off for that user with no diagnostic anywhere. Normalizing once
// here, at the single read boundary every caller goes through, closes both.
// Found in code review.
func normalizeDisplayLanguage(lang string) string {
	lang = strings.TrimSpace(lang)
	if !bcp47Tag.MatchString(lang) {
		return defaultDisplayLanguage
	}
	return lang
}

// DisplayLanguage reads a user's current display_language preference from
// their opaque UserSettings blob (see openspec/specs/display-language
// "Per-User Display Language Setting"). It defaults to "en" when the user has
// no saved settings, the settings document doesn't contain the key, the key
// isn't a string, or its value isn't shaped like a language tag (see
// normalizeDisplayLanguage) — recognition and matching always get a usable
// language rather than having to handle absence or garbage themselves.
func DisplayLanguage(storage database.Storage, userID uuid.UUID) string {
	settingsJSON, err := storage.GetUserSettings(userID)
	if err != nil {
		return defaultDisplayLanguage
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(settingsJSON), &obj); err != nil {
		return defaultDisplayLanguage
	}
	lang, ok := obj["display_language"].(string)
	if !ok {
		return defaultDisplayLanguage
	}
	return normalizeDisplayLanguage(lang)
}
