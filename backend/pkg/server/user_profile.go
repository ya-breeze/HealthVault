package server

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ya-breeze/healthvault/pkg/database"
)

// minProfileAgeYears and maxProfileAgeYears bound the ages a birthdate may
// plausibly imply (user-profile spec: "5 and 120 themselves are valid ages").
const (
	minProfileAgeYears = 5
	maxProfileAgeYears = 120
)

// birthdateLayout is the only accepted birthdate shape (user-profile spec:
// "ISO 8601 date (YYYY-MM-DD)").
const birthdateLayout = "2006-01-02"

// userProfile is the settings-blob-derived birthdate/sex/activity_override
// for one user, already validated per user-profile's "interpreted, not
// assumed" contract: an absent, unparsable, or implausible value surfaces as
// Has*=false, never a parse error or a silently-coerced default.
type userProfile struct {
	Birthdate           time.Time
	HasBirthdate        bool
	Sex                 string
	HasSex              bool
	ActivityOverride    string
	HasActivityOverride bool
}

// readUserProfile reads and validates a user's profile fields from their
// UserSettings blob, the same way DisplayLanguage already reads
// display_language: the blob stays schema-agnostic at the storage layer
// (user-settings's PUT only checks "is this valid JSON"), and interpretation
// happens here, at read time, per feature.
//
// The returned error is non-nil only for a genuine storage failure — a
// missing row (gorm.ErrRecordNotFound) or unparsable/implausible field
// values are not errors, they surface as a zero-value/Has*=false profile,
// per the "interpreted, not assumed" contract above. Callers must
// distinguish the two: an empty profile from a real DB failure must not be
// reported to the client as "you haven't set up your profile yet".
func readUserProfile(storage database.Storage, userID uuid.UUID) (userProfile, error) {
	settingsJSON, err := storage.GetUserSettings(userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return userProfile{}, nil
		}
		return userProfile{}, err
	}
	return parseUserProfile(settingsJSON), nil
}

// parseUserProfile interprets an already-read settings blob. Split out from
// readUserProfile so a caller that needs several things from the same blob
// can read it once — see SummaryTodayHandler, whose whole justification is
// being one cheap call.
func parseUserProfile(settingsJSON string) userProfile {
	var obj map[string]any
	if err := json.Unmarshal([]byte(settingsJSON), &obj); err != nil {
		return userProfile{}
	}

	var p userProfile
	if raw, ok := obj["birthdate"].(string); ok {
		if bd, ok := parseBirthdate(raw, time.Now().UTC()); ok {
			p.Birthdate = bd
			p.HasBirthdate = true
		}
	}
	if raw, ok := obj["sex"].(string); ok && (raw == "male" || raw == "female") {
		p.Sex = raw
		p.HasSex = true
	}
	if raw, ok := obj["activity_override"].(string); ok {
		if _, valid := activityOverrideTiers[raw]; valid {
			p.ActivityOverride = raw
			p.HasActivityOverride = true
		}
	}
	return p
}

// parseBirthdate parses a YYYY-MM-DD birthdate and rejects it (ok=false) if
// it fails to parse, names a future date, or implies an age outside
// [5, 120] years as of now (user-profile spec's plausibility bounds).
func parseBirthdate(raw string, now time.Time) (time.Time, bool) {
	bd, err := time.Parse(birthdateLayout, raw)
	if err != nil {
		return time.Time{}, false
	}
	if bd.After(now) {
		return time.Time{}, false
	}
	age := calendarAge(bd, now)
	if age < minProfileAgeYears || age > maxProfileAgeYears {
		return time.Time{}, false
	}
	return bd, true
}

// calendarAge returns the number of completed years between birthdate and
// now — the same "calendar age" every age-sensitive computation in this
// change uses (design.md: "computed fresh every call").
func calendarAge(birthdate, now time.Time) int {
	age := now.Year() - birthdate.Year()
	if now.Month() < birthdate.Month() ||
		(now.Month() == birthdate.Month() && now.Day() < birthdate.Day()) {
		age--
	}
	return age
}
