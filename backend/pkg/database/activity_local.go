package database

import (
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const activityCandidateLimit = 50000

var ErrActivityLimit = errors.New("too many activity records; request a shorter period")

// ActivityDay describes recorded intervals, not complete sensor coverage.
type ActivityDay struct {
	Date                    string   `json:"date"`
	Steps                   *int64   `json:"steps"`
	StepRecords             int      `json:"step_records"`
	KeptStepRecords         int      `json:"kept_step_records"`
	DroppedStepRecords      int      `json:"dropped_step_records"`
	ExerciseRecords         int      `json:"exercise_records"`
	ExerciseDurationSeconds *int64   `json:"exercise_duration_seconds"`
	ActiveCaloriesKcal      *float64 `json:"active_calories_kcal"`
	ActiveCaloriesRecords   int      `json:"active_calories_records"`
	TotalCaloriesKcal       *float64 `json:"total_calories_kcal"`
	TotalCaloriesRecords    int      `json:"total_calories_records"`
}

// SQLite date comparisons accept preserved offsets but round milliseconds.
// Widen candidate bounds, then compare exact instants in Go. The cap applies
// before exact filtering; callers must narrow dense periods instead of receiving
// silently incomplete totals. GORM retains owner and soft-deletion filters.
func activityCandidates[T any](db *gorm.DB, user uuid.UUID, from, to time.Time, columns string) ([]T, error) {
	var rows []T
	err := db.Model(new(T)).Select(columns).
		Where("user_id = ? AND julianday(start_time) >= julianday(?) AND julianday(start_time) < julianday(?)", user, from.Add(-time.Second), to.Add(time.Second)).
		Limit(activityCandidateLimit + 1).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	if len(rows) > activityCandidateLimit {
		return nil, ErrActivityLimit
	}
	return rows, nil
}

func activityInside(start, from, to time.Time) bool {
	return !start.Before(from) && start.Before(to)
}

// ActivityExercises returns exact-instant filtered rows in stable descending
// start/UUID order. Identity/provenance fields stay internal to this layer.
func ActivityExercises(db *gorm.DB, user uuid.UUID, from, to time.Time) ([]Exercise, error) {
	rows, err := activityCandidates[Exercise](db, user, from, to, "id, start_time, end_time, duration_seconds, exercise_type, distance_meters, steps, avg_cadence_spm, max_cadence_spm, stride_length_m")
	if err != nil {
		return nil, err
	}
	out := make([]Exercise, 0, len(rows))
	for _, row := range rows {
		if activityInside(row.StartTime, from, to) {
			out = append(out, row)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartTime.Equal(out[j].StartTime) {
			return out[i].ID.String() > out[j].ID.String()
		}
		return out[i].StartTime.After(out[j].StartTime)
	})
	return out, nil
}

// ActivityDailyRange uses whole-interval local start dates and one watermark
// across the selected period, matching Step Interval Collapse. No proportional
// counts are invented. Intervals starting outside the period are omitted.
func ActivityDailyRange(db *gorm.DB, user uuid.UUID, loc *time.Location, startDate, endDate string) ([]ActivityDay, error) {
	from, to, err := LocalDayWindow(startDate, endDate, loc)
	if err != nil {
		return nil, err
	}
	first, _ := time.Parse("2006-01-02", startDate)
	last, _ := time.Parse("2006-01-02", endDate)
	if last.Before(first) || last.After(first.AddDate(0, 0, 91)) {
		return nil, errors.New("request at most 92 ordered local days")
	}
	days := []ActivityDay{}
	index := map[string]int{}
	for day := first; !day.After(last); day = day.AddDate(0, 0, 1) {
		date := day.Format("2006-01-02")
		index[date] = len(days)
		days = append(days, ActivityDay{Date: date})
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		steps, err := activityCandidates[Steps](tx, user, from, to, "id, start_time, end_time, count")
		if err != nil {
			return err
		}
		sort.Slice(steps, func(i, j int) bool {
			if !steps[i].StartTime.Equal(steps[j].StartTime) {
				return steps[i].StartTime.Before(steps[j].StartTime)
			}
			if !steps[i].EndTime.Equal(steps[j].EndTime) {
				return steps[i].EndTime.Before(steps[j].EndTime)
			}
			return steps[i].ID.String() < steps[j].ID.String()
		})
		var wm stepWatermark
		for _, row := range steps {
			if !activityInside(row.StartTime, from, to) {
				continue
			}
			d := &days[index[LocalDate(row.StartTime, loc)]]
			d.StepRecords++
			if d.Steps == nil {
				d.Steps = new(int64)
			}
			if wm.admit(row.EndTime) {
				*d.Steps += int64(row.Count)
				d.KeptStepRecords++
			} else {
				d.DroppedStepRecords++
			}
		}
		exercises, err := ActivityExercises(tx, user, from, to)
		if err != nil {
			return err
		}
		for _, row := range exercises {
			d := &days[index[LocalDate(row.StartTime, loc)]]
			d.ExerciseRecords++
			if d.ExerciseDurationSeconds == nil {
				d.ExerciseDurationSeconds = new(int64)
			}
			*d.ExerciseDurationSeconds += int64(row.DurationSeconds)
		}
		active, err := activityCandidates[ActiveCalories](tx, user, from, to, "start_time, calories")
		if err != nil {
			return err
		}
		for _, row := range active {
			if activityInside(row.StartTime, from, to) {
				d := &days[index[LocalDate(row.StartTime, loc)]]
				if d.ActiveCaloriesKcal == nil {
					d.ActiveCaloriesKcal = new(float64)
				}
				*d.ActiveCaloriesKcal += row.Calories
				d.ActiveCaloriesRecords++
			}
		}
		total, err := activityCandidates[TotalCalories](tx, user, from, to, "start_time, calories")
		if err != nil {
			return err
		}
		for _, row := range total {
			if activityInside(row.StartTime, from, to) {
				d := &days[index[LocalDate(row.StartTime, loc)]]
				if d.TotalCaloriesKcal == nil {
					d.TotalCaloriesKcal = new(float64)
				}
				*d.TotalCaloriesKcal += row.Calories
				d.TotalCaloriesRecords++
			}
		}
		return nil
	})
	return days, err
}
