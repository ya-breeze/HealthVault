package server

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/vision"
)

const (
	adviceHealthWindowDays = 28
	adviceMinSleepDays     = 7
	adviceMinWeightDays    = 3
)

// buildAdviceHealthContext reduces recent health history to the small set of
// sufficiently covered values the advice model may use. Its window ends at
// the caller's yesterday so an accumulating current day cannot distort the
// summary. A missing optional series is normal; a read failure is not.
func buildAdviceHealthContext(
	storage database.Storage,
	userID uuid.UUID,
	loc *time.Location,
	now time.Time,
	profile userProfile,
	target nutritionTargetValues,
) (vision.AdviceHealthContext, error) {
	localMidnightToday, todayLabel := localCalendarToday(now, loc)
	windowEnds := todayLabel.AddDate(0, 0, -1)
	result := vision.AdviceHealthContext{
		WindowDays: adviceHealthWindowDays, WindowEnds: windowEnds.Format("2006-01-02"),
		ActivityTier:   target.ActivityTier,
		ActivitySource: "inferred_from_steps",
	}
	if profile.HasActivityOverride {
		result.ActivitySource = "profile_override"
	}

	stepDays, err := fetchDailySteps(storage, userID, loc, now)
	if err != nil {
		return vision.AdviceHealthContext{}, fmt.Errorf("read advice step context: %w", err)
	}
	meanSteps, validStepDays := trailingStepsAverage(todayLabel, stepDays)
	if validStepDays >= minValidActivityDays {
		result.MeanDailySteps = &vision.AdviceMetricAverage{Value: meanSteps, RecordedDays: validStepDays}
	}

	rangeEndingYesterday := database.TimeRange{
		From: localMidnightToday.AddDate(0, 0, -adviceHealthWindowDays),
		To:   localMidnightToday.Add(-time.Nanosecond),
	}
	sleepRows, err := storage.QueryAggregate(
		"sleeps", "start_time", "duration_seconds", database.AggFamilyCumulative,
		database.BucketDay, loc, userID, rangeEndingYesterday,
	)
	if err != nil {
		return vision.AdviceHealthContext{}, fmt.Errorf("read advice sleep context: %w", err)
	}
	if len(sleepRows) >= adviceMinSleepDays {
		var seconds float64
		for _, row := range sleepRows {
			seconds += toFloat64(row["sum"])
		}
		result.MeanSleepHours = &vision.AdviceMetricAverage{
			Value: seconds / float64(len(sleepRows)) / 3600, RecordedDays: len(sleepRows),
		}
	}

	weightRows, err := storage.QueryAggregate(
		"weights", "time", "kilograms", database.AggFamilyPoint,
		database.BucketDay, loc, userID, rangeEndingYesterday,
	)
	if err != nil {
		return vision.AdviceHealthContext{}, fmt.Errorf("read advice weight context: %w", err)
	}
	if len(weightRows) >= adviceMinWeightDays {
		result.WeightTrend = &vision.AdviceWeightTrend{
			FirstDailyAverageKg:  toFloat64(weightRows[0]["avg"]),
			LatestDailyAverageKg: toFloat64(weightRows[len(weightRows)-1]["avg"]),
			RecordedDays:         len(weightRows),
		}
	}
	return result, nil
}
