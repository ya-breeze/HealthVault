package database_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ya-breeze/healthvault/pkg/database"
)

func TestQueryAggregate_CumulativeSumsPerDayBucket(t *testing.T) {
	s := newTestStorage(t)
	userID, familyID := seedUserAndFamily(t, s)

	day1 := time.Date(2026, time.March, 15, 8, 0, 0, 0, time.UTC)
	day1Later := time.Date(2026, time.March, 15, 20, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, time.March, 16, 9, 0, 0, 0, time.UTC)

	for _, rec := range []database.Steps{
		{UserID: userID, SourcePayloadID: uuid.New(), StartTime: day1, EndTime: day1, Count: 1000},
		{UserID: userID, SourcePayloadID: uuid.New(), StartTime: day1Later, EndTime: day1Later, Count: 500},
		{UserID: userID, SourcePayloadID: uuid.New(), StartTime: day2, EndTime: day2, Count: 2000},
	} {
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		if err := s.DB().Create(&rec).Error; err != nil {
			t.Fatalf("create steps: %v", err)
		}
	}

	tr := database.TimeRange{From: day1.Add(-time.Hour), To: day2.Add(time.Hour)}
	results, err := s.QueryAggregate("steps", "start_time", "count", database.AggFamilyCumulative, database.BucketDay, userID, tr)
	if err != nil {
		t.Fatalf("QueryAggregate: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 day buckets, got %d: %+v", len(results), results)
	}
	if got := toStr(results[0]["bucket_start"]); got != "2026-03-15T00:00:00Z" {
		t.Errorf("bucket 0 start = %v, want 2026-03-15T00:00:00Z", got)
	}
	if got := toInt64(results[0]["sum"]); got != 1500 {
		t.Errorf("bucket 0 sum = %v, want 1500", got)
	}
	if got := toInt64(results[0]["count"]); got != 2 {
		t.Errorf("bucket 0 count = %v, want 2", got)
	}
	if got := toStr(results[1]["bucket_start"]); got != "2026-03-16T00:00:00Z" {
		t.Errorf("bucket 1 start = %v, want 2026-03-16T00:00:00Z", got)
	}
	if got := toInt64(results[1]["sum"]); got != 2000 {
		t.Errorf("bucket 1 sum = %v, want 2000", got)
	}
}

func TestQueryAggregate_PointAvgMinMaxPerBucket(t *testing.T) {
	s := newTestStorage(t)
	userID, familyID := seedUserAndFamily(t, s)

	day1 := time.Date(2026, time.March, 15, 8, 0, 0, 0, time.UTC)

	for _, bpm := range []int{60, 70, 80} {
		rec := database.HeartRate{
			UserID: userID, SourcePayloadID: uuid.New(),
			Time: day1.Add(time.Duration(bpm) * time.Minute), BPM: bpm,
		}
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		if err := s.DB().Create(&rec).Error; err != nil {
			t.Fatalf("create heart_rate: %v", err)
		}
	}

	tr := database.TimeRange{From: day1.Add(-time.Hour), To: day1.Add(24 * time.Hour)}
	results, err := s.QueryAggregate("heart_rates", "time", "bpm", database.AggFamilyPoint, database.BucketDay, userID, tr)
	if err != nil {
		t.Fatalf("QueryAggregate: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 bucket, got %d: %+v", len(results), results)
	}
	if got := toFloat64(results[0]["avg"]); got != 70 {
		t.Errorf("avg = %v, want 70", got)
	}
	if got := toFloat64(results[0]["min"]); got != 60 {
		t.Errorf("min = %v, want 60", got)
	}
	if got := toFloat64(results[0]["max"]); got != 80 {
		t.Errorf("max = %v, want 80", got)
	}
}

func TestQueryAggregate_MonthBucketGroupsAcrossDays(t *testing.T) {
	s := newTestStorage(t)
	userID, familyID := seedUserAndFamily(t, s)

	for _, ts := range []time.Time{
		time.Date(2026, time.March, 1, 8, 0, 0, 0, time.UTC),
		time.Date(2026, time.March, 30, 8, 0, 0, 0, time.UTC),
		time.Date(2026, time.April, 2, 8, 0, 0, 0, time.UTC),
	} {
		rec := database.Steps{UserID: userID, SourcePayloadID: uuid.New(), StartTime: ts, EndTime: ts, Count: 100}
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		if err := s.DB().Create(&rec).Error; err != nil {
			t.Fatalf("create steps: %v", err)
		}
	}

	tr := database.TimeRange{
		From: time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, time.April, 30, 0, 0, 0, 0, time.UTC),
	}
	results, err := s.QueryAggregate("steps", "start_time", "count", database.AggFamilyCumulative, database.BucketMonth, userID, tr)
	if err != nil {
		t.Fatalf("QueryAggregate: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 month buckets, got %d: %+v", len(results), results)
	}
	if got := toStr(results[0]["bucket_start"]); got != "2026-03-01T00:00:00Z" {
		t.Errorf("bucket 0 start = %v, want 2026-03-01T00:00:00Z", got)
	}
	if got := toInt64(results[0]["sum"]); got != 200 {
		t.Errorf("March sum = %v, want 200 (two records)", got)
	}
	if got := toStr(results[1]["bucket_start"]); got != "2026-04-01T00:00:00Z" {
		t.Errorf("bucket 1 start = %v, want 2026-04-01T00:00:00Z", got)
	}
}

func TestQueryAggregateSteps_OverlappingRecordsSumOnce(t *testing.T) {
	s := newTestStorage(t)
	userID, familyID := seedUserAndFamily(t, s)

	day1 := time.Date(2026, time.March, 15, 8, 0, 0, 0, time.UTC)
	for _, rec := range []database.Steps{
		{UserID: userID, SourcePayloadID: uuid.New(), StartTime: day1, EndTime: day1.Add(2 * time.Hour), Count: 3000},
		// Nested inside the first record's interval: a duplicate copy of the
		// same walk from a second sync source.
		{UserID: userID, SourcePayloadID: uuid.New(), StartTime: day1.Add(time.Minute), EndTime: day1.Add(time.Hour), Count: 2900},
	} {
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		if err := s.DB().Create(&rec).Error; err != nil {
			t.Fatalf("create steps: %v", err)
		}
	}

	tr := database.TimeRange{From: day1.Add(-time.Hour), To: day1.Add(3 * time.Hour)}
	results, err := s.QueryAggregateSteps(database.BucketDay, userID, tr)
	if err != nil {
		t.Fatalf("QueryAggregateSteps: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 day bucket, got %d: %+v", len(results), results)
	}
	if got := toInt64(results[0]["sum"]); got != 3000 {
		t.Errorf("sum = %v, want 3000 (the overlapping duplicate must be dropped, not summed)", got)
	}
	if got := toInt64(results[0]["count"]); got != 1 {
		t.Errorf("count = %v, want 1 kept record", got)
	}
}

func TestQueryAggregateSteps_NonOverlappingRecordsSumToTotal(t *testing.T) {
	s := newTestStorage(t)
	userID, familyID := seedUserAndFamily(t, s)

	day1 := time.Date(2026, time.March, 15, 8, 0, 0, 0, time.UTC)
	for _, rec := range []database.Steps{
		{UserID: userID, SourcePayloadID: uuid.New(), StartTime: day1, EndTime: day1.Add(time.Hour), Count: 3000},
		{UserID: userID, SourcePayloadID: uuid.New(), StartTime: day1.Add(2 * time.Hour), EndTime: day1.Add(3 * time.Hour), Count: 2000},
	} {
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		if err := s.DB().Create(&rec).Error; err != nil {
			t.Fatalf("create steps: %v", err)
		}
	}

	tr := database.TimeRange{From: day1.Add(-time.Hour), To: day1.Add(4 * time.Hour)}
	results, err := s.QueryAggregateSteps(database.BucketDay, userID, tr)
	if err != nil {
		t.Fatalf("QueryAggregateSteps: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 day bucket, got %d: %+v", len(results), results)
	}
	if got := toInt64(results[0]["sum"]); got != 5000 {
		t.Errorf("sum = %v, want 5000", got)
	}
	if got := toInt64(results[0]["count"]); got != 2 {
		t.Errorf("count = %v, want 2 kept records", got)
	}
}

func TestQueryAggregateSteps_RecordStraddlingMidnightLandsInStartDayBucket(t *testing.T) {
	s := newTestStorage(t)
	userID, familyID := seedUserAndFamily(t, s)

	start := time.Date(2026, time.March, 15, 23, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.March, 16, 1, 0, 0, 0, time.UTC)
	rec := database.Steps{UserID: userID, SourcePayloadID: uuid.New(), StartTime: start, EndTime: end, Count: 1500}
	rec.ID = uuid.New()
	rec.FamilyID = familyID
	if err := s.DB().Create(&rec).Error; err != nil {
		t.Fatalf("create steps: %v", err)
	}

	tr := database.TimeRange{From: start.Add(-time.Hour), To: end.Add(time.Hour)}
	results, err := s.QueryAggregateSteps(database.BucketDay, userID, tr)
	if err != nil {
		t.Fatalf("QueryAggregateSteps: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 day bucket (the record's start day), got %d: %+v", len(results), results)
	}
	if got := toStr(results[0]["bucket_start"]); got != "2026-03-15T00:00:00Z" {
		t.Errorf("bucket_start = %v, want 2026-03-15T00:00:00Z (the record's own start day)", got)
	}
	if got := toInt64(results[0]["sum"]); got != 1500 {
		t.Errorf("sum = %v, want 1500", got)
	}
}

func TestQueryAggregateBloodPressure_DualColumns(t *testing.T) {
	s := newTestStorage(t)
	userID, familyID := seedUserAndFamily(t, s)

	day1 := time.Date(2026, time.March, 15, 8, 0, 0, 0, time.UTC)
	for i, v := range []struct{ sys, dia float64 }{{120, 80}, {130, 85}} {
		rec := database.BloodPressure{
			UserID: userID, SourcePayloadID: uuid.New(), Time: day1.Add(time.Duration(i) * time.Hour),
			Systolic: v.sys, Diastolic: v.dia,
		}
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		if err := s.DB().Create(&rec).Error; err != nil {
			t.Fatalf("create blood_pressure: %v", err)
		}
	}

	tr := database.TimeRange{From: day1.Add(-time.Hour), To: day1.Add(3 * time.Hour)}
	results, err := s.QueryAggregateBloodPressure(database.BucketDay, userID, tr)
	if err != nil {
		t.Fatalf("QueryAggregateBloodPressure: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 bucket, got %d: %+v", len(results), results)
	}
	if got := toFloat64(results[0]["systolic_avg"]); got != 125 {
		t.Errorf("systolic_avg = %v, want 125", got)
	}
	if got := toFloat64(results[0]["diastolic_avg"]); got != 82.5 {
		t.Errorf("diastolic_avg = %v, want 82.5", got)
	}
	if got := toFloat64(results[0]["systolic_min"]); got != 120 {
		t.Errorf("systolic_min = %v, want 120", got)
	}
	if got := toFloat64(results[0]["diastolic_max"]); got != 85 {
		t.Errorf("diastolic_max = %v, want 85", got)
	}
}

func TestQueryAggregateNutrition_SevenColumnsIgnoreNulls(t *testing.T) {
	s := newTestStorage(t)
	userID, familyID := seedUserAndFamily(t, s)

	day1 := time.Date(2026, time.March, 15, 8, 0, 0, 0, time.UTC)
	cal1, protein1 := 500.0, 30.0
	cal2 := 300.0 // protein deliberately nil, to confirm SUM ignores it rather than nulling the bucket

	for _, rec := range []database.Nutrition{
		{UserID: userID, SourcePayloadID: uuid.New(), StartTime: day1, EndTime: day1, Calories: &cal1, ProteinGrams: &protein1},
		{UserID: userID, SourcePayloadID: uuid.New(), StartTime: day1.Add(time.Hour), EndTime: day1.Add(time.Hour), Calories: &cal2},
	} {
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		if err := s.DB().Create(&rec).Error; err != nil {
			t.Fatalf("create nutrition: %v", err)
		}
	}

	tr := database.TimeRange{From: day1.Add(-time.Hour), To: day1.Add(3 * time.Hour)}
	results, err := s.QueryAggregateNutrition(database.BucketDay, userID, tr)
	if err != nil {
		t.Fatalf("QueryAggregateNutrition: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 bucket, got %d: %+v", len(results), results)
	}
	if got := toFloat64(results[0]["sum_calories"]); got != 800 {
		t.Errorf("sum_calories = %v, want 800", got)
	}
	if got := toFloat64(results[0]["sum_protein_grams"]); got != 30 {
		t.Errorf("sum_protein_grams = %v, want 30 (one NULL row should be ignored, not zero the bucket)", got)
	}
}

// derefAny unwraps the *interface{} the driver scans a computed (aliased
// expression) column into — GORM's generic map destination can't determine
// a static type for an expression column, so it scans defensively through
// a pointer-to-interface rather than the concrete type a real column gets.
func derefAny(v any) any {
	if p, ok := v.(*interface{}); ok {
		if p == nil {
			return nil
		}
		return *p
	}
	return v
}

// toInt64/toFloat64/toStr normalize the unwrapped driver value (int64,
// float64, string, or []byte depending on the SQLite column affinity) into
// a single comparable form for assertions.
func toInt64(v any) int64 {
	switch n := derefAny(v).(type) {
	case int64:
		return n
	case float64:
		return int64(n)
	default:
		return -1
	}
}

func toFloat64(v any) float64 {
	switch n := derefAny(v).(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	default:
		return -1
	}
}

func toStr(v any) string {
	switch s := derefAny(v).(type) {
	case string:
		return s
	case []byte:
		return string(s)
	default:
		return ""
	}
}
