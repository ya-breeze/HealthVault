package hcimport_test

import (
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/ya-breeze/healthvault/pkg/hcimport"
)

// buildHCDB creates a minimal Health Connect SQLite DB with one record per supported table.
func buildHCDB(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "hc-test-*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	f.Close()

	db, err := sql.Open("sqlite3", f.Name())
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	defer db.Close()

	epochMs := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC).UnixMilli()
	endMs := epochMs + 3600000 // +1h

	stmts := []string{
		`CREATE TABLE heart_rate_record_series_table (beats_per_minute INTEGER, epoch_millis INTEGER)`,
		fmt.Sprintf(`INSERT INTO heart_rate_record_series_table VALUES (72, %d)`, epochMs),

		`CREATE TABLE steps_record_table (count INTEGER, start_time INTEGER, end_time INTEGER)`,
		fmt.Sprintf(`INSERT INTO steps_record_table VALUES (1500, %d, %d)`, epochMs, endMs),

		`CREATE TABLE sleep_session_record_table (row_id INTEGER PRIMARY KEY, start_time INTEGER, end_time INTEGER)`,
		fmt.Sprintf(`INSERT INTO sleep_session_record_table VALUES (1, %d, %d)`, epochMs, endMs),

		`CREATE TABLE sleep_stages_table (parent_key INTEGER, stage_start_time INTEGER, stage_end_time INTEGER, stage_type INTEGER)`,
		fmt.Sprintf(`INSERT INTO sleep_stages_table VALUES (1, %d, %d, 6)`, epochMs, endMs),

		`CREATE TABLE exercise_session_record_table (exercise_type INTEGER, start_time INTEGER, end_time INTEGER)`,
		fmt.Sprintf(`INSERT INTO exercise_session_record_table VALUES (33, %d, %d)`, epochMs, endMs),

		`CREATE TABLE distance_record_table (distance REAL, start_time INTEGER, end_time INTEGER)`,
		fmt.Sprintf(`INSERT INTO distance_record_table VALUES (5000.0, %d, %d)`, epochMs, endMs),

		// 4184 J = exactly 1 kcal
		`CREATE TABLE total_calories_burned_record_table (energy REAL, start_time INTEGER, end_time INTEGER)`,
		fmt.Sprintf(`INSERT INTO total_calories_burned_record_table VALUES (4184.0, %d, %d)`, epochMs, endMs),

		`CREATE TABLE oxygen_saturation_record_table (percentage REAL, time INTEGER)`,
		fmt.Sprintf(`INSERT INTO oxygen_saturation_record_table VALUES (98.5, %d)`, epochMs),

		`CREATE TABLE speed_record_table (speed REAL, epoch_millis INTEGER)`,
		fmt.Sprintf(`INSERT INTO speed_record_table VALUES (3.5, %d)`, epochMs),
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("exec %q: %v", s, err)
		}
	}
	return f.Name()
}

func TestRead_AllTables(t *testing.T) {
	dbPath := buildHCDB(t)

	payload, counts, err := hcimport.Read(dbPath)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	cases := []struct {
		name string
		got  int
	}{
		{"heart_rate", counts.HeartRate},
		{"steps", counts.Steps},
		{"sleep", counts.Sleep},
		{"exercise", counts.Exercise},
		{"distance", counts.Distance},
		{"total_calories", counts.TotalCalories},
		{"oxygen_saturation", counts.OxygenSaturation},
		{"speed", counts.Speed},
	}
	for _, c := range cases {
		if c.got != 1 {
			t.Errorf("%s: want 1 record, got %d", c.name, c.got)
		}
	}

	if payload.HeartRate[0].BPM != 72 {
		t.Errorf("heart_rate BPM: want 72, got %d", payload.HeartRate[0].BPM)
	}
	if payload.Steps[0].Count != 1500 {
		t.Errorf("steps count: want 1500, got %d", payload.Steps[0].Count)
	}
	if payload.Exercise[0].Type != "running" {
		t.Errorf("exercise type: want running, got %s", payload.Exercise[0].Type)
	}
	if payload.Sleep[0].Stages[0].Stage != "deep" {
		t.Errorf("sleep stage: want deep, got %s", payload.Sleep[0].Stages[0].Stage)
	}
	if payload.Speed[0].MetersPerSecond != 3.5 {
		t.Errorf("speed: want 3.5, got %v", payload.Speed[0].MetersPerSecond)
	}
	// 4184 J / 4184 = 1.0 kcal
	if payload.TotalCalories[0].Calories != 1.0 {
		t.Errorf("total_calories kcal: want 1.0, got %v", payload.TotalCalories[0].Calories)
	}
}

func TestRead_TimeConversion(t *testing.T) {
	dbPath := buildHCDB(t)

	payload, _, err := hcimport.Read(dbPath)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	want := "2026-06-24T10:00:00Z"
	if payload.HeartRate[0].Time != want {
		t.Errorf("heart_rate time: want %s, got %s", want, payload.HeartRate[0].Time)
	}
	if payload.Speed[0].Time != want {
		t.Errorf("speed time: want %s, got %s", want, payload.Speed[0].Time)
	}
}

func TestRead_SleepDuration(t *testing.T) {
	dbPath := buildHCDB(t)

	payload, _, err := hcimport.Read(dbPath)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	// startMs and endMs are 1h apart → 3600 seconds
	if payload.Sleep[0].DurationSeconds != 3600 {
		t.Errorf("sleep duration: want 3600, got %d", payload.Sleep[0].DurationSeconds)
	}
	if payload.Sleep[0].Stages[0].DurationSeconds != 3600 {
		t.Errorf("sleep stage duration: want 3600, got %d", payload.Sleep[0].Stages[0].DurationSeconds)
	}
}

func TestSleepStageEnum_KnownAndUnknown(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "hc-test-*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	f.Close()

	db, err := sql.Open("sqlite3", f.Name())
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	defer db.Close()

	epochMs := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC).UnixMilli()
	endMs := epochMs + 3600000

	stmts := []string{
		`CREATE TABLE sleep_session_record_table (row_id INTEGER PRIMARY KEY, start_time INTEGER, end_time INTEGER)`,
		fmt.Sprintf(`INSERT INTO sleep_session_record_table VALUES (1, %d, %d)`, epochMs, endMs),

		`CREATE TABLE sleep_stages_table (parent_key INTEGER, stage_start_time INTEGER, stage_end_time INTEGER, stage_type INTEGER)`,
		// stage_type 7 = rem, stage_type 999 = unknown
		fmt.Sprintf(`INSERT INTO sleep_stages_table VALUES (1, %d, %d, 7)`, epochMs, epochMs+1800000),
		fmt.Sprintf(`INSERT INTO sleep_stages_table VALUES (1, %d, %d, 999)`, epochMs+1800000, endMs),

		`CREATE TABLE heart_rate_record_series_table (beats_per_minute INTEGER, epoch_millis INTEGER)`,
		`CREATE TABLE steps_record_table (count INTEGER, start_time INTEGER, end_time INTEGER)`,
		`CREATE TABLE exercise_session_record_table (exercise_type INTEGER, start_time INTEGER, end_time INTEGER)`,
		`CREATE TABLE distance_record_table (distance REAL, start_time INTEGER, end_time INTEGER)`,
		`CREATE TABLE total_calories_burned_record_table (energy REAL, start_time INTEGER, end_time INTEGER)`,
		`CREATE TABLE oxygen_saturation_record_table (percentage REAL, time INTEGER)`,
		`CREATE TABLE speed_record_table (speed REAL, epoch_millis INTEGER)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("exec %q: %v", s, err)
		}
	}
	db.Close()

	payload, _, err := hcimport.Read(f.Name())
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	stages := payload.Sleep[0].Stages
	if len(stages) != 2 {
		t.Fatalf("want 2 stages, got %d", len(stages))
	}
	if stages[0].Stage != "rem" {
		t.Errorf("stage[0]: want rem, got %s", stages[0].Stage)
	}
	if stages[1].Stage != "unknown" {
		t.Errorf("stage[1]: want unknown, got %s", stages[1].Stage)
	}
}
