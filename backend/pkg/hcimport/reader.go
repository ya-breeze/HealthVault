package hcimport

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/ya-breeze/healthvault/pkg/ingest"
)

const joulesToKcal = 1.0 / 4184.0

// Counts holds the number of records imported per type.
type Counts struct {
	HeartRate        int `json:"heart_rate"`
	Steps            int `json:"steps"`
	Sleep            int `json:"sleep"`
	Exercise         int `json:"exercise"`
	Distance         int `json:"distance"`
	TotalCalories    int `json:"total_calories"`
	OxygenSaturation int `json:"oxygen_saturation"`
	Speed            int `json:"speed"`
}

// Read opens the Health Connect SQLite database at dbPath, reads all supported
// tables, and returns an ingest.PayloadJSON ready for ingest.Process plus the
// per-type counts.
func Read(dbPath string) (*ingest.PayloadJSON, *Counts, error) {
	db, err := sql.Open("sqlite3", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return nil, nil, fmt.Errorf("open hc db: %w", err)
	}
	defer db.Close()

	p := &ingest.PayloadJSON{}
	c := &Counts{}

	type reader struct {
		name string
		fn   func(*sql.DB, *ingest.PayloadJSON, *Counts) error
	}
	readers := []reader{
		{"heart_rate", readHeartRate},
		{"steps", readSteps},
		{"sleep", readSleep},
		{"exercise", readExercise},
		{"distance", readDistance},
		{"total_calories", readTotalCalories},
		{"oxygen_saturation", readOxygenSaturation},
		{"speed", readSpeed},
	}
	for _, r := range readers {
		t := time.Now()
		if err := r.fn(db, p, c); err != nil {
			slog.Error("hcimport: read table failed", "table", r.name, "err", err)
			return nil, nil, err
		}
		slog.Info("hcimport: table read", "table", r.name, "duration", time.Since(t))
	}

	return p, c, nil
}

func msToRFC3339(ms int64) string {
	return time.UnixMilli(ms).UTC().Format(time.RFC3339Nano)
}

func readHeartRate(db *sql.DB, p *ingest.PayloadJSON, c *Counts) error {
	rows, err := db.Query(`
		SELECT s.beats_per_minute, s.epoch_millis
		FROM heart_rate_record_series_table s
		ORDER BY s.epoch_millis`)
	if err != nil {
		return fmt.Errorf("query heart_rate: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var bpm int
		var epochMs int64
		if err := rows.Scan(&bpm, &epochMs); err != nil {
			return fmt.Errorf("scan heart_rate: %w", err)
		}
		p.HeartRate = append(p.HeartRate, ingest.HeartRateJSON{
			BPM:  bpm,
			Time: msToRFC3339(epochMs),
		})
		c.HeartRate++
	}
	return rows.Err()
}

func readSteps(db *sql.DB, p *ingest.PayloadJSON, c *Counts) error {
	rows, err := db.Query(`
		SELECT count, start_time, end_time
		FROM steps_record_table
		ORDER BY start_time`)
	if err != nil {
		return fmt.Errorf("query steps: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var count int
		var startMs, endMs int64
		if err := rows.Scan(&count, &startMs, &endMs); err != nil {
			return fmt.Errorf("scan steps: %w", err)
		}
		p.Steps = append(p.Steps, ingest.StepsJSON{
			Count:     count,
			StartTime: msToRFC3339(startMs),
			EndTime:   msToRFC3339(endMs),
		})
		c.Steps++
	}
	return rows.Err()
}

func readSleep(db *sql.DB, p *ingest.PayloadJSON, c *Counts) error {
	rows, err := db.Query(`
		SELECT row_id, start_time, end_time
		FROM sleep_session_record_table
		ORDER BY start_time`)
	if err != nil {
		return fmt.Errorf("query sleep: %w", err)
	}
	defer rows.Close()

	type sessionRow struct {
		rowID   int64
		startMs int64
		endMs   int64
	}
	var sessions []sessionRow
	for rows.Next() {
		var s sessionRow
		if err := rows.Scan(&s.rowID, &s.startMs, &s.endMs); err != nil {
			return fmt.Errorf("scan sleep: %w", err)
		}
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	stageRows, err := db.Query(`
		SELECT parent_key, stage_start_time, stage_end_time, stage_type
		FROM sleep_stages_table
		ORDER BY parent_key, stage_start_time`)
	if err != nil {
		return fmt.Errorf("query sleep_stages: %w", err)
	}
	defer stageRows.Close()

	type stageRow struct {
		parentKey   int64
		startMs     int64
		endMs       int64
		stageType   int
	}
	stagesByParent := map[int64][]stageRow{}
	for stageRows.Next() {
		var s stageRow
		if err := stageRows.Scan(&s.parentKey, &s.startMs, &s.endMs, &s.stageType); err != nil {
			return fmt.Errorf("scan sleep_stage: %w", err)
		}
		stagesByParent[s.parentKey] = append(stagesByParent[s.parentKey], s)
	}
	if err := stageRows.Err(); err != nil {
		return err
	}

	for _, s := range sessions {
		durationSec := int(time.UnixMilli(s.endMs).Sub(time.UnixMilli(s.startMs)).Seconds())

		var stages []ingest.SleepStageJSON
		for _, st := range stagesByParent[s.rowID] {
			stageDur := int(time.UnixMilli(st.endMs).Sub(time.UnixMilli(st.startMs)).Seconds())
			stages = append(stages, ingest.SleepStageJSON{
				Stage:           sleepStageName(st.stageType),
				StartTime:       msToRFC3339(st.startMs),
				EndTime:         msToRFC3339(st.endMs),
				DurationSeconds: stageDur,
			})
		}

		p.Sleep = append(p.Sleep, ingest.SleepJSON{
			SessionEndTime:  msToRFC3339(s.endMs),
			DurationSeconds: durationSec,
			Stages:          stages,
		})
		c.Sleep++
	}
	return nil
}

func readExercise(db *sql.DB, p *ingest.PayloadJSON, c *Counts) error {
	rows, err := db.Query(`
		SELECT exercise_type, start_time, end_time
		FROM exercise_session_record_table
		ORDER BY start_time`)
	if err != nil {
		return fmt.Errorf("query exercise: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var exType int
		var startMs, endMs int64
		if err := rows.Scan(&exType, &startMs, &endMs); err != nil {
			return fmt.Errorf("scan exercise: %w", err)
		}
		durationSec := int(time.UnixMilli(endMs).Sub(time.UnixMilli(startMs)).Seconds())
		p.Exercise = append(p.Exercise, ingest.ExerciseJSON{
			Type:            exerciseTypeName(exType),
			StartTime:       msToRFC3339(startMs),
			EndTime:         msToRFC3339(endMs),
			DurationSeconds: durationSec,
		})
		c.Exercise++
	}
	return rows.Err()
}

func readDistance(db *sql.DB, p *ingest.PayloadJSON, c *Counts) error {
	rows, err := db.Query(`
		SELECT distance, start_time, end_time
		FROM distance_record_table
		ORDER BY start_time`)
	if err != nil {
		return fmt.Errorf("query distance: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var meters float64
		var startMs, endMs int64
		if err := rows.Scan(&meters, &startMs, &endMs); err != nil {
			return fmt.Errorf("scan distance: %w", err)
		}
		p.Distance = append(p.Distance, ingest.DistanceJSON{
			Meters:    meters,
			StartTime: msToRFC3339(startMs),
			EndTime:   msToRFC3339(endMs),
		})
		c.Distance++
	}
	return rows.Err()
}

func readTotalCalories(db *sql.DB, p *ingest.PayloadJSON, c *Counts) error {
	rows, err := db.Query(`
		SELECT energy, start_time, end_time
		FROM total_calories_burned_record_table
		ORDER BY start_time`)
	if err != nil {
		return fmt.Errorf("query total_calories: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var joules float64
		var startMs, endMs int64
		if err := rows.Scan(&joules, &startMs, &endMs); err != nil {
			return fmt.Errorf("scan total_calories: %w", err)
		}
		p.TotalCalories = append(p.TotalCalories, ingest.CaloriesJSON{
			Calories:  joules * joulesToKcal,
			StartTime: msToRFC3339(startMs),
			EndTime:   msToRFC3339(endMs),
		})
		c.TotalCalories++
	}
	return rows.Err()
}

func readOxygenSaturation(db *sql.DB, p *ingest.PayloadJSON, c *Counts) error {
	rows, err := db.Query(`
		SELECT percentage, time
		FROM oxygen_saturation_record_table
		ORDER BY time`)
	if err != nil {
		return fmt.Errorf("query oxygen_saturation: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var pct float64
		var epochMs int64
		if err := rows.Scan(&pct, &epochMs); err != nil {
			return fmt.Errorf("scan oxygen_saturation: %w", err)
		}
		p.OxygenSaturation = append(p.OxygenSaturation, ingest.OxygenSaturationJSON{
			Percentage: pct,
			Time:       msToRFC3339(epochMs),
		})
		c.OxygenSaturation++
	}
	return rows.Err()
}

func readSpeed(db *sql.DB, p *ingest.PayloadJSON, c *Counts) error {
	rows, err := db.Query(`
		SELECT s.speed, s.epoch_millis
		FROM speed_record_table s
		ORDER BY s.epoch_millis`)
	if err != nil {
		return fmt.Errorf("query speed: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var mps float64
		var epochMs int64
		if err := rows.Scan(&mps, &epochMs); err != nil {
			return fmt.Errorf("scan speed: %w", err)
		}
		p.Speed = append(p.Speed, ingest.SpeedJSON{
			MetersPerSecond: mps,
			Time:            msToRFC3339(epochMs),
		})
		c.Speed++
	}
	return rows.Err()
}
