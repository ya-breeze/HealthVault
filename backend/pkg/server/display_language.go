package server

import (
	"encoding/json"

	"github.com/google/uuid"

	"github.com/ya-breeze/healthvault/pkg/database"
)

// defaultDisplayLanguage is used when a user has never saved a
// display_language setting, or their settings document doesn't parse.
const defaultDisplayLanguage = "en"

// DisplayLanguage reads a user's current display_language preference from
// their opaque UserSettings blob (see openspec/specs/display-language
// "Per-User Display Language Setting"). It defaults to "en" when the user has
// no saved settings, the settings document doesn't contain the key, or the
// key isn't a string — recognition and matching always get a usable
// language rather than having to handle absence themselves.
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
	if !ok || lang == "" {
		return defaultDisplayLanguage
	}
	return lang
}
