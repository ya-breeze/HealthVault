package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ya-breeze/healthvault/pkg/database"
)

// defaultDisplayLanguage is used when a user has never saved a
// display_language setting, or their settings document doesn't parse.
const defaultDisplayLanguage = "en"

// bcp47Tag matches the shape of a BCP-47 language tag: a 2-8 letter primary
// subtag optionally followed by up to three alphanumeric subtags. Deliberately
// a shape check, not a registry lookup — it does not care whether the tag names
// a real language, only that the value is a language tag rather than arbitrary
// text. "_" is accepted alongside the canonical "-" separator so this stays
// no stricter than IsEnglishDisplayLanguage, which already splits on either.
//
// The subtag repetition is unbounded here and the length ceiling is enforced
// separately by maxDisplayLanguageLen. An earlier version bounded the
// repetition at three instead, using it as an implicit 35-byte cap, but that
// conflated two unrelated jobs and broke the one rule this setting has: a tag
// with four or more subtags after the primary — "ru-Cyrl-RU-u-nu-latn" is
// well-formed BCP-47 and storable, since PUT /users/me/settings keeps the blob
// verbatim — failed the shape check and normalized to "en" here, while the
// frontend's resolveLanguage read the same value's primary subtag and rendered
// Russian. That is precisely the split-brain the display-language spec forbids
// ("the same interpretation SHALL govern both UI rendering and recognition
// requests"): Russian UI, English recognition, and USDA/Open Food Facts
// silently re-enabled for that user. Separating the length cap from the shape
// check keeps the ceiling without inventing a subtag-count rule neither side
// agrees on. Found in code review.
//
// Shape alone still cannot prove a tag names a real language — "ru-Chinese" is
// well-formed — so this bounds how much caller-controlled text can reach the
// model rather than eliminating it. The remaining budget is a few short words
// inside a fixed sentence (see vision.languageDirective), and it only ever
// affects the recognition calls of the account that stored it. Found in code
// review.
var bcp47Tag = regexp.MustCompile(`^[A-Za-z]{2,8}([-_][A-Za-z0-9]{1,8})*$`)

// maxDisplayLanguageLen caps how much tag-shaped text can be interpolated into
// the vision system prompt. Without it the regex above imposes no length limit
// at all: "en" followed by a thousand "-xxxxxxxx" groups is ~9 KB of perfectly
// well-formed text.
//
// 35 is not arbitrary — it is exactly the ceiling the old `{0,3}` bound
// implied (an 8-byte primary subtag plus three 9-byte groups). Keeping the
// same number is the point: dropping the subtag-count rule must not buy any
// extra prompt budget, so this change moves the ceiling from being an
// accident of the repetition bound to being stated outright, without raising
// it by a single byte. Every tag anyone writes in practice is far inside it —
// "zh-Hant-HK", "sr-Latn-RS", "en-US-POSIX" and "ru-Cyrl-RU-u-nu-latn" are
// 20 bytes or fewer.
const maxDisplayLanguageLen = 35

// primarySubtagOnly is the reduction applied to a stored value this file will
// not pass through verbatim. It extracts the same thing the frontend's
// resolveLanguage extracts — the text before the first "-" or "_", after
// trimming — and keeps it only if that is itself a well-formed primary subtag.
//
// Reducing rather than jumping straight to defaultDisplayLanguage is what
// keeps the two sides from disagreeing. Both the frontend and
// vision.IsEnglishDisplayLanguage decide what language a stored value means by
// reading its primary subtag and nothing else, so discarding a value whose
// primary subtag is perfectly legible re-creates the split-brain twice already
// found here: the frontend renders Russian from "ru-…" while this function
// hands the backend "en", which asks the model for English Display Names and
// silently re-enables USDA/Open Food Facts matching for that user. Length and
// shape violations live in the rest of the tag, never in the primary subtag,
// so there is no reason for the language itself to be a casualty of them.
//
// The prompt budget survives intact: the result is at most 8 letters, which is
// far below maxDisplayLanguageLen — the reduction is strictly more bounded
// than the verbatim path it replaces, not a relaxation of it. Casing is left
// as stored, matching the verbatim path; every consumer compares primary
// subtags case-insensitively.
func primarySubtagOnly(lang string) string {
	primary, _, _ := strings.Cut(lang, "-")
	primary, _, _ = strings.Cut(primary, "_")
	if !primarySubtag.MatchString(primary) {
		return defaultDisplayLanguage
	}
	return primary
}

// primarySubtag is bcp47Tag's first component on its own — see that variable
// for why the shape is checked at all.
var primarySubtag = regexp.MustCompile(`^[A-Za-z]{2,8}$`)

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
//
// A value that fails either check is reduced to its primary subtag rather than
// replaced with the default — see primarySubtagOnly. Falling back to "en"
// outright was the third instance of the same split-brain bug in this file:
// "ru-Cyrl-RU-u-nu-latn-x-private-abcde" is 36 bytes, so the ceiling rejected
// it and the backend read English while resolveLanguage read its "ru" and
// rendered the Russian UI. "ru-Cyrl-RU!" failed the shape check the same way.
// Found in code review.
func normalizeDisplayLanguage(lang string) string {
	lang = strings.TrimSpace(lang)
	if len(lang) > maxDisplayLanguageLen || !bcp47Tag.MatchString(lang) {
		return primarySubtagOnly(lang)
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
//
// A real storage failure is logged before falling back, because that fallback
// is otherwise indistinguishable from the user having chosen English while
// silently changing what the request does: the vision call is made with "en",
// the meal is persisted with English Display Names and no Canonical Name, and
// USDA/Open Food Facts matching is re-enabled for it by
// retrieveCandidates' language gate. "No settings row yet" is the ordinary
// case for a user who has never opened the setting, so only errors other than
// gorm.ErrRecordNotFound are logged — the same treatment resolveItems already
// gives its own custom-food-fetch degradation. Found in code review.
func DisplayLanguage(storage database.Storage, userID uuid.UUID) string {
	settingsJSON, err := storage.GetUserSettings(userID)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Error("DisplayLanguage: read user settings", "err", err, "user_id", userID)
		}
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
