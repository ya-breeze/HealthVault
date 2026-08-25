package database_test

import (
	"testing"
	"time"

	"github.com/ya-breeze/healthvault/pkg/database"
)

func TestResolveTimezone(t *testing.T) {
	cases := []struct {
		name         string
		settingsJSON string
		want         string // want.String() of the resolved *time.Location
	}{
		{"missing key", `{}`, "UTC"},
		{"empty string", `{"timezone":""}`, "UTC"},
		{"invalid zone name", `{"timezone":"Not/A_Zone"}`, "UTC"},
		{"valid zone name", `{"timezone":"America/Los_Angeles"}`, "America/Los_Angeles"},
		{"malformed json", `not json`, "UTC"},
		{"wrong type", `{"timezone":123}`, "UTC"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := database.ResolveTimezone(tc.settingsJSON)
			if got.String() != tc.want {
				t.Errorf("ResolveTimezone(%q) = %v, want %v", tc.settingsJSON, got, tc.want)
			}
		})
	}
}

// A UTC timestamp just after midnight UTC on 2026-08-21 falls on 2026-08-20
// in America/Los_Angeles (UTC-7 in August, DST) — the day-boundary shift
// this whole feature exists to resolve. See design.md §2 "Local Day boundary".
func TestLocalDate_ShiftsAcrossDayBoundaryByZone(t *testing.T) {
	ts := time.Date(2026, 8, 21, 2, 0, 0, 0, time.UTC)

	utcDate := database.LocalDate(ts, time.UTC)
	if utcDate != "2026-08-21" {
		t.Errorf("LocalDate in UTC = %q, want 2026-08-21", utcDate)
	}

	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	laDate := database.LocalDate(ts, loc)
	if laDate != "2026-08-20" {
		t.Errorf("LocalDate in America/Los_Angeles = %q, want 2026-08-20", laDate)
	}
}

func TestResolveUsualMealsPerDay(t *testing.T) {
	cases := []struct {
		name         string
		settingsJSON string
		want         int
	}{
		{"missing key", `{}`, 3},
		{"zero", `{"usual_meals_per_day":0}`, 3},
		{"negative", `{"usual_meals_per_day":-2}`, 3},
		{"non-integer", `{"usual_meals_per_day":2.5}`, 3},
		{"non-numeric type", `{"usual_meals_per_day":"3"}`, 3},
		{"malformed json", `not json`, 3},
		{"valid positive integer", `{"usual_meals_per_day":5}`, 5},
		{"valid integer as whole float", `{"usual_meals_per_day":4.0}`, 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := database.ResolveUsualMealsPerDay(tc.settingsJSON)
			if got != tc.want {
				t.Errorf("ResolveUsualMealsPerDay(%q) = %d, want %d", tc.settingsJSON, got, tc.want)
			}
		})
	}
}

func TestSettingsRawString(t *testing.T) {
	cases := []struct {
		name         string
		settingsJSON string
		key          string
		want         string
	}{
		{"missing key", `{}`, "timezone", ""},
		{"present string", `{"timezone":"Europe/Warsaw"}`, "timezone", "Europe/Warsaw"},
		{"wrong type", `{"timezone":42}`, "timezone", ""},
		{"malformed json", `not json`, "timezone", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := database.SettingsRawString(tc.settingsJSON, tc.key)
			if got != tc.want {
				t.Errorf("SettingsRawString(%q, %q) = %q, want %q", tc.settingsJSON, tc.key, got, tc.want)
			}
		})
	}
}
