package server

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ya-breeze/healthvault/pkg/database"
)

var referenceNow = time.Date(2026, time.August, 24, 0, 0, 0, 0, time.UTC)

func TestParseBirthdate_FutureDateRejected(t *testing.T) {
	_, ok := parseBirthdate("2027-01-01", referenceNow)
	if ok {
		t.Errorf("expected a future birthdate to be rejected")
	}
}

func TestParseBirthdate_UnparsableRejected(t *testing.T) {
	for _, raw := range []string{"not-a-date", "2026/08/24", "24-08-2026", ""} {
		if _, ok := parseBirthdate(raw, referenceNow); ok {
			t.Errorf("raw=%q: expected an unparsable birthdate to be rejected", raw)
		}
	}
}

func TestParseBirthdate_AgeBoundaries(t *testing.T) {
	tests := []struct {
		name   string
		years  int
		wantOK bool
	}{
		{"age 4 rejected (below minimum)", 4, false},
		{"age 5 accepted (minimum, inclusive)", 5, true},
		{"age 120 accepted (maximum, inclusive)", 120, true},
		{"age 121 rejected (above maximum)", 121, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bd := referenceNow.AddDate(-tt.years, 0, 0)
			raw := bd.Format(birthdateLayout)
			_, ok := parseBirthdate(raw, referenceNow)
			if ok != tt.wantOK {
				t.Errorf("age %d: ok = %v, want %v", tt.years, ok, tt.wantOK)
			}
		})
	}
}

func newProfileTestStorage(t *testing.T) database.Storage {
	t.Helper()
	db, err := database.Open(slog.New(slog.NewTextHandler(io.Discard, nil)), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return database.NewStorage(db)
}

// An unrecognized sex value in the settings blob must surface as
// HasSex=false ("malformed/absent -> not set"), never a parse error or a
// silently-coerced default.
func TestReadUserProfile_UnrecognizedSexRejected(t *testing.T) {
	st := newProfileTestStorage(t)
	userID, familyID := uuid.New(), uuid.New()
	if err := st.UpsertUserSettings(userID, familyID, `{"sex":"other","birthdate":"1990-01-01"}`); err != nil {
		t.Fatalf("UpsertUserSettings: %v", err)
	}

	profile, err := readUserProfile(st, userID)
	if err != nil {
		t.Fatalf("readUserProfile: %v", err)
	}
	if profile.HasSex {
		t.Errorf("expected HasSex=false for an unrecognized sex value, got Sex=%q", profile.Sex)
	}
	if !profile.HasBirthdate {
		t.Errorf("expected the valid birthdate alongside it to still parse")
	}
}

func TestReadUserProfile_ValidSexValuesAccepted(t *testing.T) {
	st := newProfileTestStorage(t)
	for _, sex := range []string{"male", "female"} {
		userID, familyID := uuid.New(), uuid.New()
		if err := st.UpsertUserSettings(userID, familyID, `{"sex":"`+sex+`"}`); err != nil {
			t.Fatalf("UpsertUserSettings: %v", err)
		}
		profile, err := readUserProfile(st, userID)
		if err != nil {
			t.Fatalf("readUserProfile: %v", err)
		}
		if !profile.HasSex || profile.Sex != sex {
			t.Errorf("sex=%q: got HasSex=%v Sex=%q", sex, profile.HasSex, profile.Sex)
		}
	}
}

// An unrecognized activity_override value must surface as
// HasActivityOverride=false, the same "malformed/absent -> not set" contract
// TestReadUserProfile_UnrecognizedSexRejected verifies for sex.
func TestReadUserProfile_UnrecognizedActivityOverrideRejected(t *testing.T) {
	st := newProfileTestStorage(t)
	userID, familyID := uuid.New(), uuid.New()
	if err := st.UpsertUserSettings(userID, familyID, `{"activity_override":"bogus"}`); err != nil {
		t.Fatalf("UpsertUserSettings: %v", err)
	}

	profile, err := readUserProfile(st, userID)
	if err != nil {
		t.Fatalf("readUserProfile: %v", err)
	}
	if profile.HasActivityOverride {
		t.Errorf("expected HasActivityOverride=false for an unrecognized value, got ActivityOverride=%q", profile.ActivityOverride)
	}
}

// Malformed settings JSON (not even a valid JSON object) must yield an empty
// profile, not a panic or a propagated parse error — the settings blob is
// schema-agnostic at the storage layer, so interpretation failures here are
// not storage failures.
func TestReadUserProfile_MalformedSettingsJSONYieldsEmptyProfile(t *testing.T) {
	st := newProfileTestStorage(t)
	userID, familyID := uuid.New(), uuid.New()
	if err := st.UpsertUserSettings(userID, familyID, `not json`); err != nil {
		t.Fatalf("UpsertUserSettings: %v", err)
	}

	profile, err := readUserProfile(st, userID)
	if err != nil {
		t.Fatalf("readUserProfile: %v", err)
	}
	if profile.HasBirthdate || profile.HasSex || profile.HasActivityOverride {
		t.Errorf("expected an empty profile for malformed settings JSON, got %+v", profile)
	}
}
