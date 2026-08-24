package server

import "time"

// activityTier is one of the five standard Mifflin-St Jeor/Harris-Benedict
// activity multiplier tiers (see design.md's "Activity tiers" table).
type activityTier struct {
	Name       string
	Multiplier float64
}

var (
	tierSedentary = activityTier{Name: "Sedentary", Multiplier: 1.2}
	tierLight     = activityTier{Name: "Lightly active", Multiplier: 1.375}
	tierModerate  = activityTier{Name: "Moderately active", Multiplier: 1.55}
	tierActive    = activityTier{Name: "Very active", Multiplier: 1.725}
	tierExtra     = activityTier{Name: "Extra active", Multiplier: 1.9}
)

// trailingWindowDays is the size of the trailing step-history window used to
// infer activity level from steps (design.md "Trailing window: 28 calendar
// days ending yesterday").
const trailingWindowDays = 28

// minValidActivityDays is the minimum number of valid days that must remain
// after exclusion for an inferred tier to be reported at all, rather than
// "unavailable" (design.md: "7 is one calendar week").
const minValidActivityDays = 7

// lowStepFloor is the per-day step count below which a day at the trailing
// edge of the window is discarded as a data fragment rather than treated as
// a real low-activity day (design.md "The 500-step floor").
const lowStepFloor = 500.0

// dailySteps is one day's aggregated step total, keyed to a UTC calendar
// day — the shape trailingStepsAverage needs as input. A day with no step
// records at all simply has no entry, rather than an entry with Sum 0: the
// two are not the same thing (see design.md's zero-record-day rule).
type dailySteps struct {
	Date time.Time
	Sum  float64
}

// trailingStepsAverage computes the trailing 28-day step average ending the
// day before `today`, per design.md's two exclusion rules:
//
//  1. Zero-record days (no entry in `days` for that calendar date) are
//     always excluded, wherever they fall in the window.
//  2. Low-but-nonzero (<500 step) days are trimmed only from the trailing
//     edge: walking backward from yesterday, each such day is discarded
//     until the first day that clears 500, after which trimming never
//     resumes even if a later (older) day is itself under 500.
//
// `today` is reduced to a UTC calendar day; the window is the 28 calendar
// days immediately before it. Returns (average, validDayCount) — callers
// must treat validDayCount < minValidActivityDays as "unavailable" rather
// than trusting the average (task 1.3).
func trailingStepsAverage(today time.Time, days []dailySteps) (float64, int) {
	today = today.UTC().Truncate(24 * time.Hour)

	byDate := make(map[time.Time]float64, len(days))
	for _, d := range days {
		byDate[d.Date.UTC().Truncate(24*time.Hour)] = d.Sum
	}

	var total float64
	var validDays int
	trimming := true
	for i := 1; i <= trailingWindowDays; i++ {
		date := today.AddDate(0, 0, -i)
		sum, ok := byDate[date]
		if !ok {
			// Zero-record day: always excluded, and does not itself stop
			// the trailing-edge trim scan (a sync gap doesn't "clear" the
			// floor, but it has no data to evaluate against it either).
			continue
		}
		if trimming {
			if sum < lowStepFloor {
				continue
			}
			trimming = false
		}
		total += sum
		validDays++
	}

	if validDays == 0 {
		return 0, 0
	}
	return total / float64(validDays), validDays
}

// tierForAverage maps a trailing average steps/day figure to its activity
// tier per design.md's 5-tier table (task 1.2).
func tierForAverage(avgStepsPerDay float64) activityTier {
	switch {
	case avgStepsPerDay < 5000:
		return tierSedentary
	case avgStepsPerDay < 7500:
		return tierLight
	case avgStepsPerDay < 10000:
		return tierModerate
	case avgStepsPerDay < 12500:
		return tierActive
	default:
		return tierExtra
	}
}

// inferActivityTier infers the caller's activity tier from their trailing
// step history (task 1.1-1.3 combined). ok is false — "unavailable" — when
// fewer than minValidActivityDays valid days remain after exclusion, rather
// than a default tier being guessed.
func inferActivityTier(today time.Time, days []dailySteps) (tier activityTier, ok bool) {
	avg, validDays := trailingStepsAverage(today, days)
	if validDays < minValidActivityDays {
		return activityTier{}, false
	}
	return tierForAverage(avg), true
}
