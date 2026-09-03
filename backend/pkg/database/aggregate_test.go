package database_test

import (
	"math"
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
	results, err := s.QueryAggregate("steps", "start_time", "count", database.AggFamilyCumulative, database.BucketDay, time.UTC, userID, tr)
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
	results, err := s.QueryAggregate("heart_rates", "time", "bpm", database.AggFamilyPoint, database.BucketDay, time.UTC, userID, tr)
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
	results, err := s.QueryAggregate("steps", "start_time", "count", database.AggFamilyCumulative, database.BucketMonth, time.UTC, userID, tr)
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
	results, err := s.QueryAggregateBloodPressure(database.BucketDay, time.UTC, userID, tr)
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
	results, err := s.QueryAggregateNutrition(database.BucketDay, time.UTC, userID, tr)
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

// Asia/Tokyo (+09:00, no DST) local midnight (2026-03-16 00:00 JST) is UTC
// 2026-03-15 15:00. Two records either side of that instant fall on the
// same UTC calendar day but different Tokyo calendar days.
func TestQueryAggregate_LocalDayBoundaryAheadOfUTC(t *testing.T) {
	s := newTestStorage(t)
	userID, familyID := seedUserAndFamily(t, s)

	loc, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}

	beforeMidnight := time.Date(2026, time.March, 15, 14, 45, 0, 0, time.UTC) // 23:45 JST, Mar 15
	afterMidnight := time.Date(2026, time.March, 15, 15, 15, 0, 0, time.UTC)  // 00:15 JST, Mar 16

	for _, ts := range []time.Time{beforeMidnight, afterMidnight} {
		rec := database.Steps{UserID: userID, SourcePayloadID: uuid.New(), StartTime: ts, EndTime: ts, Count: 100}
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		if err := s.DB().Create(&rec).Error; err != nil {
			t.Fatalf("create steps: %v", err)
		}
	}
	tr := database.TimeRange{From: beforeMidnight.Add(-time.Hour), To: afterMidnight.Add(time.Hour)}

	utcResults, err := s.QueryAggregate(
		"steps", "start_time", "count", database.AggFamilyCumulative, database.BucketDay, time.UTC, userID, tr)
	if err != nil {
		t.Fatalf("QueryAggregate(UTC): %v", err)
	}
	if len(utcResults) != 1 {
		t.Fatalf("under UTC both records fall on the same calendar day: expected 1 bucket, got %d: %+v", len(utcResults), utcResults)
	}

	tokyoResults, err := s.QueryAggregate(
		"steps", "start_time", "count", database.AggFamilyCumulative, database.BucketDay, loc, userID, tr)
	if err != nil {
		t.Fatalf("QueryAggregate(Tokyo): %v", err)
	}
	if len(tokyoResults) != 2 {
		t.Fatalf("under Asia/Tokyo the two records straddle local midnight: expected 2 buckets, got %d: %+v", len(tokyoResults), tokyoResults)
	}
	if got := toStr(tokyoResults[0]["bucket_start"]); got != "2026-03-15T00:00:00Z" {
		t.Errorf("bucket 0 = %v, want 2026-03-15T00:00:00Z", got)
	}
	if got := toStr(tokyoResults[1]["bucket_start"]); got != "2026-03-16T00:00:00Z" {
		t.Errorf("bucket 1 = %v, want 2026-03-16T00:00:00Z", got)
	}
}

// America/Los_Angeles (-08:00 PST in January, clear of any DST transition)
// local midnight (2026-01-15 00:00 PST) is UTC 2026-01-15 08:00 — the
// boundary moves the opposite direction from the Asia/Tokyo case above.
func TestQueryAggregate_LocalDayBoundaryBehindUTC(t *testing.T) {
	s := newTestStorage(t)
	userID, familyID := seedUserAndFamily(t, s)

	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}

	beforeMidnight := time.Date(2026, time.January, 15, 7, 45, 0, 0, time.UTC) // 23:45 PST, Jan 14
	afterMidnight := time.Date(2026, time.January, 15, 8, 15, 0, 0, time.UTC)  // 00:15 PST, Jan 15

	for _, ts := range []time.Time{beforeMidnight, afterMidnight} {
		rec := database.Steps{UserID: userID, SourcePayloadID: uuid.New(), StartTime: ts, EndTime: ts, Count: 100}
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		if err := s.DB().Create(&rec).Error; err != nil {
			t.Fatalf("create steps: %v", err)
		}
	}
	tr := database.TimeRange{From: beforeMidnight.Add(-time.Hour), To: afterMidnight.Add(time.Hour)}

	utcResults, err := s.QueryAggregate(
		"steps", "start_time", "count", database.AggFamilyCumulative, database.BucketDay, time.UTC, userID, tr)
	if err != nil {
		t.Fatalf("QueryAggregate(UTC): %v", err)
	}
	if len(utcResults) != 1 {
		t.Fatalf("under UTC both records fall on the same calendar day: expected 1 bucket, got %d: %+v", len(utcResults), utcResults)
	}

	laResults, err := s.QueryAggregate(
		"steps", "start_time", "count", database.AggFamilyCumulative, database.BucketDay, loc, userID, tr)
	if err != nil {
		t.Fatalf("QueryAggregate(LA): %v", err)
	}
	if len(laResults) != 2 {
		t.Fatalf(
			"under America/Los_Angeles the two records straddle local midnight: expected 2 buckets, got %d: %+v",
			len(laResults), laResults,
		)
	}
	if got := toStr(laResults[0]["bucket_start"]); got != "2026-01-14T00:00:00Z" {
		t.Errorf("bucket 0 = %v, want 2026-01-14T00:00:00Z", got)
	}
	if got := toStr(laResults[1]["bucket_start"]); got != "2026-01-15T00:00:00Z" {
		t.Errorf("bucket 1 = %v, want 2026-01-15T00:00:00Z", got)
	}
}

// Asia/Kathmandu (+05:45) has the finest offset in the modern IANA database.
// Its local midnight (2026-03-16 00:00 +05:45) is UTC 2026-03-15 18:15 — not
// on an hour boundary — which only a 15-minute (not hourly) SQL slot can
// resolve exactly.
func TestQueryAggregate_FortyFiveMinuteOffsetResolvesExactly(t *testing.T) {
	s := newTestStorage(t)
	userID, familyID := seedUserAndFamily(t, s)

	loc, err := time.LoadLocation("Asia/Kathmandu")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}

	beforeMidnight := time.Date(2026, time.March, 15, 18, 0, 0, 0, time.UTC) // 23:45 Kathmandu, Mar 15
	afterMidnight := time.Date(2026, time.March, 15, 18, 30, 0, 0, time.UTC) // 00:15 Kathmandu, Mar 16

	for _, ts := range []time.Time{beforeMidnight, afterMidnight} {
		rec := database.Steps{UserID: userID, SourcePayloadID: uuid.New(), StartTime: ts, EndTime: ts, Count: 100}
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		if err := s.DB().Create(&rec).Error; err != nil {
			t.Fatalf("create steps: %v", err)
		}
	}
	tr := database.TimeRange{From: beforeMidnight.Add(-time.Hour), To: afterMidnight.Add(time.Hour)}

	results, err := s.QueryAggregate(
		"steps", "start_time", "count", database.AggFamilyCumulative, database.BucketDay, loc, userID, tr)
	if err != nil {
		t.Fatalf("QueryAggregate: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf(
			"the two records straddle Kathmandu's 45-minute-offset midnight: expected 2 buckets, got %d: %+v",
			len(results), results,
		)
	}
	if got := toStr(results[0]["bucket_start"]); got != "2026-03-15T00:00:00Z" {
		t.Errorf("bucket 0 = %v, want 2026-03-15T00:00:00Z", got)
	}
	if got := toStr(results[1]["bucket_start"]); got != "2026-03-16T00:00:00Z" {
		t.Errorf("bucket 1 = %v, want 2026-03-16T00:00:00Z", got)
	}
}

// The exactness criterion: every record must land in exactly one bucket, and
// each bucket's SQL-side sum must equal what grouping the same records
// record-by-record via database.LocalBucketKey would produce — even across
// a spring-forward day (23 real hours) or a fall-back day (25 real hours).
// A regrouping that merely applied a fixed UTC offset would agree with this
// on an ordinary day and disagree on exactly these two.
func TestQueryAggregate_DSTTransitionsFoldExactly(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}

	cases := []struct {
		name string
		from time.Time
		to   time.Time
	}{
		// 2026-03-08: America/Los_Angeles springs forward (2am -> 3am),
		// giving that local day 23 real hours.
		{"spring-forward", time.Date(2026, 3, 7, 0, 0, 0, 0, time.UTC), time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)},
		// 2026-11-01: America/Los_Angeles falls back (2am -> 1am), giving
		// that local day 25 real hours.
		{"fall-back", time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStorage(t)
			userID, familyID := seedUserAndFamily(t, s)

			want := map[string]int64{}
			var total int64
			for ts := tc.from; ts.Before(tc.to); ts = ts.Add(20 * time.Minute) {
				rec := database.Steps{UserID: userID, SourcePayloadID: uuid.New(), StartTime: ts, EndTime: ts, Count: 1}
				rec.ID = uuid.New()
				rec.FamilyID = familyID
				if err := s.DB().Create(&rec).Error; err != nil {
					t.Fatalf("create steps: %v", err)
				}
				want[database.LocalBucketKey(ts, database.BucketDay, loc)]++
				total++
			}

			tr := database.TimeRange{From: tc.from.Add(-time.Hour), To: tc.to.Add(time.Hour)}
			results, err := s.QueryAggregate(
				"steps", "start_time", "count", database.AggFamilyCumulative, database.BucketDay, loc, userID, tr)
			if err != nil {
				t.Fatalf("QueryAggregate: %v", err)
			}

			got := map[string]int64{}
			var totalGot int64
			for _, row := range results {
				sum := toInt64(row["sum"])
				got[toStr(row["bucket_start"])] = sum
				totalGot += sum
			}
			if totalGot != total {
				t.Errorf("total across all buckets = %d, want %d (every record must land in exactly one bucket)", totalGot, total)
			}
			if len(got) != len(want) {
				t.Fatalf("bucket count = %d, want %d\n got=%v\nwant=%v", len(got), len(want), got, want)
			}
			for key, wantSum := range want {
				if got[key] != wantSum {
					t.Errorf("bucket %s sum = %d, want %d (record-by-record LocalBucketKey fold)", key, got[key], wantSum)
				}
			}
		})
	}
}

// A record near a month boundary can move from one local month bucket to
// another: UTC 2026-02-28 23:00 is Tokyo (+09:00) 2026-03-01 08:00.
func TestQueryAggregate_MonthBucketLocalZoneShiftsMonth(t *testing.T) {
	s := newTestStorage(t)
	userID, familyID := seedUserAndFamily(t, s)

	loc, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}

	monthShift := time.Date(2026, time.February, 28, 23, 0, 0, 0, time.UTC)
	clearlyFeb := time.Date(2026, time.February, 1, 1, 0, 0, 0, time.UTC) // February under either zone

	for _, ts := range []time.Time{clearlyFeb, monthShift} {
		rec := database.Steps{UserID: userID, SourcePayloadID: uuid.New(), StartTime: ts, EndTime: ts, Count: 100}
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		if err := s.DB().Create(&rec).Error; err != nil {
			t.Fatalf("create steps: %v", err)
		}
	}

	tr := database.TimeRange{
		From: time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, time.March, 2, 0, 0, 0, 0, time.UTC),
	}
	results, err := s.QueryAggregate(
		"steps", "start_time", "count", database.AggFamilyCumulative, database.BucketMonth, loc, userID, tr)
	if err != nil {
		t.Fatalf("QueryAggregate: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 month buckets (Feb, Mar), got %d: %+v", len(results), results)
	}
	if got := toStr(results[0]["bucket_start"]); got != "2026-02-01T00:00:00Z" {
		t.Errorf("bucket 0 = %v, want 2026-02-01T00:00:00Z", got)
	}
	if got := toInt64(results[0]["sum"]); got != 100 {
		t.Errorf("Feb sum = %v, want 100 (only clearlyFeb)", got)
	}
	if got := toStr(results[1]["bucket_start"]); got != "2026-03-01T00:00:00Z" {
		t.Errorf("bucket 1 = %v, want 2026-03-01T00:00:00Z", got)
	}
	if got := toInt64(results[1]["sum"]); got != 100 {
		t.Errorf("Mar sum = %v, want 100 (monthShift moved into March under Asia/Tokyo)", got)
	}
}

// avg must be the value-weighted average across every slot's contribution,
// not the average of each slot's own average: two readings in one 15-minute
// slot and one reading in the next slot would average (65+100)/2 = 82.5 the
// wrong way, versus the correct (60+70+100)/3 ≈ 76.67. min/max must survive
// the fold across slots too.
func TestQueryAggregate_PointAvgIsValueWeightedAcrossSlots(t *testing.T) {
	s := newTestStorage(t)
	userID, familyID := seedUserAndFamily(t, s)

	day1 := time.Date(2026, time.March, 15, 8, 0, 0, 0, time.UTC)
	for _, r := range []struct {
		offset time.Duration
		bpm    int
	}{
		{0, 60},                 // slot [08:00, 08:15)
		{5 * time.Minute, 70},   // slot [08:00, 08:15)
		{20 * time.Minute, 100}, // slot [08:15, 08:30)
	} {
		rec := database.HeartRate{UserID: userID, SourcePayloadID: uuid.New(), Time: day1.Add(r.offset), BPM: r.bpm}
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		if err := s.DB().Create(&rec).Error; err != nil {
			t.Fatalf("create heart_rate: %v", err)
		}
	}

	tr := database.TimeRange{From: day1.Add(-time.Hour), To: day1.Add(time.Hour)}
	results, err := s.QueryAggregate(
		"heart_rates", "time", "bpm", database.AggFamilyPoint, database.BucketDay, time.UTC, userID, tr)
	if err != nil {
		t.Fatalf("QueryAggregate: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 bucket, got %d: %+v", len(results), results)
	}
	const wantAvg = (60.0 + 70.0 + 100.0) / 3
	if got := toFloat64(results[0]["avg"]); math.Abs(got-wantAvg) > 1e-9 {
		t.Errorf("avg = %v, want %v (value-weighted, not average of the two slots' own averages)", got, wantAvg)
	}
	if got := toFloat64(results[0]["min"]); got != 60 {
		t.Errorf("min = %v, want 60", got)
	}
	if got := toFloat64(results[0]["max"]); got != 100 {
		t.Errorf("max = %v, want 100", got)
	}
}

// Same value-weighted-average and min/max-survives-the-fold proof as
// TestQueryAggregate_PointAvgIsValueWeightedAcrossSlots, for the
// blood_pressure dual-column path.
func TestQueryAggregateBloodPressure_AvgIsValueWeightedAcrossSlots(t *testing.T) {
	s := newTestStorage(t)
	userID, familyID := seedUserAndFamily(t, s)

	day1 := time.Date(2026, time.March, 15, 8, 0, 0, 0, time.UTC)
	for _, r := range []struct {
		offset   time.Duration
		sys, dia float64
	}{
		{0, 110, 70},                 // slot [08:00, 08:15)
		{5 * time.Minute, 130, 90},   // slot [08:00, 08:15)
		{20 * time.Minute, 150, 100}, // slot [08:15, 08:30)
	} {
		rec := database.BloodPressure{
			UserID: userID, SourcePayloadID: uuid.New(), Time: day1.Add(r.offset),
			Systolic: r.sys, Diastolic: r.dia,
		}
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		if err := s.DB().Create(&rec).Error; err != nil {
			t.Fatalf("create blood_pressure: %v", err)
		}
	}

	tr := database.TimeRange{From: day1.Add(-time.Hour), To: day1.Add(time.Hour)}
	results, err := s.QueryAggregateBloodPressure(database.BucketDay, time.UTC, userID, tr)
	if err != nil {
		t.Fatalf("QueryAggregateBloodPressure: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 bucket, got %d: %+v", len(results), results)
	}
	const wantSysAvg = (110.0 + 130.0 + 150.0) / 3
	if got := toFloat64(results[0]["systolic_avg"]); math.Abs(got-wantSysAvg) > 1e-9 {
		t.Errorf("systolic_avg = %v, want %v", got, wantSysAvg)
	}
	if got := toFloat64(results[0]["systolic_min"]); got != 110 {
		t.Errorf("systolic_min = %v, want 110", got)
	}
	if got := toFloat64(results[0]["systolic_max"]); got != 150 {
		t.Errorf("systolic_max = %v, want 150", got)
	}
	const wantDiaAvg = (70.0 + 90.0 + 100.0) / 3
	if got := toFloat64(results[0]["diastolic_avg"]); math.Abs(got-wantDiaAvg) > 1e-9 {
		t.Errorf("diastolic_avg = %v, want %v", got, wantDiaAvg)
	}
	if got := toFloat64(results[0]["diastolic_min"]); got != 70 {
		t.Errorf("diastolic_min = %v, want 70", got)
	}
	if got := toFloat64(results[0]["diastolic_max"]); got != 100 {
		t.Errorf("diastolic_max = %v, want 100", got)
	}
}

// An unknown or empty timezone setting resolves to time.UTC
// (database.ResolveTimezone), and a nil loc must produce the exact same
// buckets as that resolved UTC location — QueryAggregate's own nil-loc
// fallback (task 4) must agree with ResolveTimezone's, not just also
// happen to default to UTC.
func TestQueryAggregate_NilLocationMatchesResolvedEmptySetting(t *testing.T) {
	s := newTestStorage(t)
	userID, familyID := seedUserAndFamily(t, s)

	ts := time.Date(2026, time.March, 15, 23, 30, 0, 0, time.UTC)
	rec := database.Steps{UserID: userID, SourcePayloadID: uuid.New(), StartTime: ts, EndTime: ts, Count: 500}
	rec.ID = uuid.New()
	rec.FamilyID = familyID
	if err := s.DB().Create(&rec).Error; err != nil {
		t.Fatalf("create steps: %v", err)
	}

	tr := database.TimeRange{From: ts.Add(-time.Hour), To: ts.Add(time.Hour)}
	viaEmptySetting := database.ResolveTimezone("")

	resultsNil, err := s.QueryAggregate(
		"steps", "start_time", "count", database.AggFamilyCumulative, database.BucketDay, nil, userID, tr)
	if err != nil {
		t.Fatalf("QueryAggregate(nil): %v", err)
	}
	resultsUTC, err := s.QueryAggregate(
		"steps", "start_time", "count", database.AggFamilyCumulative, database.BucketDay, viaEmptySetting, userID, tr)
	if err != nil {
		t.Fatalf("QueryAggregate(resolved UTC): %v", err)
	}
	if len(resultsNil) != 1 || len(resultsUTC) != 1 {
		t.Fatalf("expected 1 bucket each, got nil=%d resolved=%d", len(resultsNil), len(resultsUTC))
	}
	if got := toStr(resultsNil[0]["bucket_start"]); got != "2026-03-15T00:00:00Z" {
		t.Errorf("nil-loc bucket_start = %v, want 2026-03-15T00:00:00Z (today's UTC bucket)", got)
	}
	if toStr(resultsNil[0]["bucket_start"]) != toStr(resultsUTC[0]["bucket_start"]) {
		t.Errorf(
			"nil loc and an explicitly-resolved empty-setting location disagree: %v vs %v",
			resultsNil[0]["bucket_start"], resultsUTC[0]["bucket_start"],
		)
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
