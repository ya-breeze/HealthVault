package server

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// referenceToday is a fixed UTC "today" used throughout these tests so the
// 28-day trailing window (today-1 .. today-28) is deterministic.
var referenceToday = time.Date(2026, time.January, 29, 0, 0, 0, 0, time.UTC)

// fullWindow returns dailySteps entries for every one of the 28 trailing
// days (today-1 .. today-28), each with the given step count, as a base to
// mutate for individual exclusion-rule cases.
func fullWindow(stepsPerDay float64) []dailySteps {
	days := make([]dailySteps, 0, trailingWindowDays)
	for i := 1; i <= trailingWindowDays; i++ {
		days = append(days, dailySteps{Date: referenceToday.AddDate(0, 0, -i), Sum: stepsPerDay})
	}
	return days
}

// withoutDay removes the entry for today-i (a zero-record day: no entry at
// all, not an entry with Sum 0 — see dailySteps's doc comment).
func withoutDay(days []dailySteps, i int) []dailySteps {
	date := referenceToday.AddDate(0, 0, -i)
	out := make([]dailySteps, 0, len(days))
	for _, d := range days {
		if d.Date.Equal(date) {
			continue
		}
		out = append(out, d)
	}
	return out
}

// withDay sets (or overrides) the entry for today-i to the given sum.
func withDay(days []dailySteps, i int, sum float64) []dailySteps {
	date := referenceToday.AddDate(0, 0, -i)
	out := make([]dailySteps, 0, len(days))
	found := false
	for _, d := range days {
		if d.Date.Equal(date) {
			out = append(out, dailySteps{Date: date, Sum: sum})
			found = true
			continue
		}
		out = append(out, d)
	}
	if !found {
		out = append(out, dailySteps{Date: date, Sum: sum})
	}
	return out
}

func TestTrailingStepsAverage_ZeroRecordDayAtTrailingEdgeExcluded(t *testing.T) {
	days := withoutDay(fullWindow(1000), 1) // yesterday has no step records at all

	avg, valid := trailingStepsAverage(referenceToday, days)
	if valid != trailingWindowDays-1 {
		t.Fatalf("valid = %d, want %d", valid, trailingWindowDays-1)
	}
	if avg != 1000 {
		t.Errorf("avg = %v, want 1000 (the zero-record day must not dilute the average)", avg)
	}
}

func TestTrailingStepsAverage_ZeroRecordDayInInteriorExcludedNotAveragedAsZero(t *testing.T) {
	days := withoutDay(fullWindow(1000), 15) // an interior day, not at the trailing edge

	avg, valid := trailingStepsAverage(referenceToday, days)
	if valid != trailingWindowDays-1 {
		t.Fatalf("valid = %d, want %d", valid, trailingWindowDays-1)
	}
	if avg != 1000 {
		t.Errorf("avg = %v, want 1000 (interior zero-record day must be excluded, not counted as a 0-step day)", avg)
	}
}

func TestTrailingStepsAverage_SubFloorDayAtTrailingEdgeTrimmed(t *testing.T) {
	days := withDay(fullWindow(1000), 1, 499) // yesterday: a real but sub-floor day

	avg, valid := trailingStepsAverage(referenceToday, days)
	if valid != trailingWindowDays-1 {
		t.Fatalf("valid = %d, want %d (the sub-floor trailing day must be trimmed)", valid, trailingWindowDays-1)
	}
	if avg != 1000 {
		t.Errorf("avg = %v, want 1000", avg)
	}
}

func TestTrailingStepsAverage_ExactlyFloorDayAtTrailingEdgeKeptNotTrimmed(t *testing.T) {
	days := withDay(fullWindow(1000), 1, 500) // exactly the floor: kept, and stops trimming

	avg, valid := trailingStepsAverage(referenceToday, days)
	if valid != trailingWindowDays {
		t.Fatalf("valid = %d, want %d (a day at exactly the floor must be kept)", valid, trailingWindowDays)
	}
	wantAvg := (float64(trailingWindowDays-1)*1000 + 500) / float64(trailingWindowDays)
	if avg != wantAvg {
		t.Errorf("avg = %v, want %v", avg, wantAvg)
	}
}

func TestTrailingStepsAverage_AboveFloorDayAtTrailingEdgeKept(t *testing.T) {
	days := withDay(fullWindow(1000), 1, 501)

	avg, valid := trailingStepsAverage(referenceToday, days)
	if valid != trailingWindowDays {
		t.Fatalf("valid = %d, want %d", valid, trailingWindowDays)
	}
	wantAvg := (float64(trailingWindowDays-1)*1000 + 501) / float64(trailingWindowDays)
	if avg != wantAvg {
		t.Errorf("avg = %v, want %v", avg, wantAvg)
	}
}

// Trailing-edge trimming stops scanning backward at the first clean
// (>=500-step) day; an older, lower day behind it is a real interior
// low-activity day and must be kept, not trimmed.
func TestTrailingStepsAverage_TrimmingStopsAtFirstCleanDayScanningBackward(t *testing.T) {
	days := fullWindow(1000)
	days = withDay(days, 1, 100) // trailing-edge, sub-floor: trimmed
	days = withDay(days, 2, 50)  // still trailing-edge, sub-floor: trimmed
	days = withDay(days, 3, 900) // first clean day: kept, trimming stops here
	days = withDay(days, 4, 50)  // older than the clean day: a real low day, kept

	avg, valid := trailingStepsAverage(referenceToday, days)
	wantValid := trailingWindowDays - 2 // days 1 and 2 trimmed; day 4 kept despite being <500
	if valid != wantValid {
		t.Fatalf("valid = %d, want %d", valid, wantValid)
	}
	wantTotal := float64(trailingWindowDays-4)*1000 + 900 + 50
	wantAvg := wantTotal / float64(wantValid)
	if avg != wantAvg {
		t.Errorf("avg = %v, want %v", avg, wantAvg)
	}
}

func TestInferActivityTier_SevenValidDaysIsAvailable(t *testing.T) {
	days := []dailySteps{}
	for i := 1; i <= 7; i++ {
		days = append(days, dailySteps{Date: referenceToday.AddDate(0, 0, -i), Sum: 1000})
	}

	tier, ok := inferActivityTier(referenceToday, days)
	if !ok {
		t.Fatalf("expected ok=true with exactly 7 valid days")
	}
	if tier != tierSedentary {
		t.Errorf("tier = %+v, want %+v", tier, tierSedentary)
	}
}

func TestInferActivityTier_SixValidDaysIsUnavailable(t *testing.T) {
	days := []dailySteps{}
	for i := 1; i <= 6; i++ {
		days = append(days, dailySteps{Date: referenceToday.AddDate(0, 0, -i), Sum: 1000})
	}

	_, ok := inferActivityTier(referenceToday, days)
	if ok {
		t.Fatalf("expected ok=false (insufficient_activity_data) with only 6 valid days")
	}
}

func TestTierForAverage_Boundaries(t *testing.T) {
	tests := []struct {
		avg  float64
		want activityTier
	}{
		{4999, tierSedentary},
		{5000, tierLight},
		{7499, tierLight},
		{7500, tierModerate},
		{9999, tierModerate},
		{10000, tierActive},
		{12499, tierActive},
		{12500, tierExtra},
	}
	for _, tt := range tests {
		got := tierForAverage(tt.avg)
		if got != tt.want {
			t.Errorf("tierForAverage(%v) = %+v, want %+v", tt.avg, got, tt.want)
		}
	}
}

// resolveActivityTier's override branch never touches storage, so a nil
// Storage is safe here and lets these run as pure unit tests.
func TestResolveActivityTier_OverrideValues(t *testing.T) {
	tests := []struct {
		override string
		wantName string
		wantMult float64
	}{
		{"sedentary", "Sedentary", 1.2},
		{"light", "Lightly active", 1.375},
		{"moderate", "Moderately active", 1.55},
		// The two non-obvious cases: the enum names do not positionally
		// match the tier names.
		{"active", "Very active", 1.725},
		{"very_active", "Extra active", 1.9},
	}
	for _, tt := range tests {
		profile := userProfile{HasActivityOverride: true, ActivityOverride: tt.override}
		name, mult, ok, err := resolveActivityTier(nil, uuid.Nil, profile, referenceToday)
		if err != nil {
			t.Fatalf("override %q: unexpected error: %v", tt.override, err)
		}
		if !ok {
			t.Fatalf("override %q: expected ok=true", tt.override)
		}
		if name != tt.wantName || mult != tt.wantMult {
			t.Errorf("override %q: got (%q, %v), want (%q, %v)", tt.override, name, mult, tt.wantName, tt.wantMult)
		}
	}
}
