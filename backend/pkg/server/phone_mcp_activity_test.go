package server_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/server"
)

func TestPhoneActivityLocalIntervalsAndMissingEvidence(t *testing.T) {
	st := newFoodTestStorage(t)
	owner, family := seedFoodUser(t, st)
	if err := st.UpsertUserSettings(owner, family, `{"timezone":"Europe/Prague"}`); err != nil {
		t.Fatal(err)
	}
	stamp := func(s string) time.Time {
		x, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			t.Fatal(err)
		}
		return x
	}
	step := func(user uuid.UUID, start, end string, count int) database.Steps {
		r := database.Steps{UserID: user, SourcePayloadID: uuid.New(), StartTime: stamp(start), EndTime: stamp(end), Count: count}
		r.ID, r.FamilyID = uuid.New(), family
		if err := st.DB().Create(&r).Error; err != nil {
			t.Fatal(err)
		}
		return r
	}
	step(owner, "2026-03-28T09:00:00Z", "2026-03-28T09:01:00Z", 0)
	step(owner, "2026-03-29T00:30:00+01:00", "2026-03-30T00:30:00+02:00", 100)
	step(owner, "2026-03-29T12:00:00Z", "2026-03-29T13:00:00Z", 999)          // contained
	step(owner, "2026-03-30T00:10:00+02:00", "2026-03-30T00:20:00+02:00", 33) // covered across midnight
	step(owner, "2026-03-29T22:15:00Z", "2026-03-29T23:00:00Z", 200)          // partial overlap remains whole
	step(owner, "2026-03-30T00:20:00+02:00", "2026-03-30T00:25:00+02:00", 888)
	step(owner, "2026-03-31T00:00:00+02:00", "2026-03-31T00:01:00+02:00", 777) // exclusive end
	step(uuid.New(), "2026-03-29T01:00:00+01:00", "2026-03-29T02:00:00+01:00", 10000)
	deleted := step(owner, "2026-03-29T21:00:00Z", "2026-03-31T00:00:00Z", 12345)
	if err := st.DB().Delete(&deleted).Error; err != nil {
		t.Fatal(err)
	}
	active := database.ActiveCalories{UserID: owner, StartTime: stamp("2026-03-29T03:30:00+02:00"), EndTime: stamp("2026-03-29T04:30:00+02:00"), Calories: 50}
	active.ID, active.FamilyID = uuid.New(), family
	if err := st.DB().Create(&active).Error; err != nil {
		t.Fatal(err)
	}
	total := database.TotalCalories{UserID: owner, StartTime: active.StartTime, EndTime: active.EndTime, Calories: 250}
	total.ID, total.FamilyID = uuid.New(), family
	if err := st.DB().Create(&total).Error; err != nil {
		t.Fatal(err)
	}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).PhoneMCPHandler("test-phone-token", owner.String(), nil)
	args := map[string]any{"period": "range", "start_date": "2026-03-27", "end_date": "2026-03-30"}
	out := phoneSuccess(t, phoneCall(t, h, "get_activity_daily_totals", args))
	days := out["days"].([]any)
	empty, zero, spring, next := days[0].(map[string]any), days[1].(map[string]any), days[2].(map[string]any), days[3].(map[string]any)
	if empty["steps"] != nil || empty["exercise_duration_seconds"] != nil || empty["active_calories_kcal"] != nil {
		t.Fatalf("missing interpreted as zero: %+v", empty)
	}
	if zero["steps"] != float64(0) || zero["step_records"] != float64(1) {
		t.Fatalf("recorded zero lost: %+v", zero)
	}
	if spring["steps"] != float64(100) || spring["step_records"] != float64(2) || spring["dropped_step_records"] != float64(1) {
		t.Fatalf("local collapse failed: %+v", spring)
	}
	if next["steps"] != float64(200) || next["step_records"] != float64(3) || next["dropped_step_records"] != float64(2) {
		t.Fatalf("watermark reset at midnight: %+v", next)
	}
	if spring["active_calories_kcal"] != float64(50) || spring["total_calories_kcal"] != float64(250) {
		t.Fatalf("calories combined or wrong date: %+v", spring)
	}
	single := phoneSuccess(t, phoneCall(t, h, "get_activity_daily_totals", map[string]any{"period": "range", "start_date": "2026-03-30", "end_date": "2026-03-30"}))["days"].([]any)[0].(map[string]any)
	if single["steps"] != float64(233) {
		t.Fatal("interval starting before selected window incorrectly counted")
	}
	if !strings.Contains(out["notes"].([]any)[3].(string), "partial overlaps") {
		t.Fatal("collapse limitation missing")
	}
	var count int64
	if err := st.DB().Model(&database.Steps{}).Where("user_id = ?", owner).Count(&count).Error; err != nil || count != 7 {
		t.Fatalf("read changed step records: %d %v", count, err)
	}
}

func TestPhoneActivityExercisesExactInstantsPaginationAndReadOnly(t *testing.T) {
	st := newFoodTestStorage(t)
	owner, family := seedFoodUser(t, st)
	if err := st.UpsertUserSettings(owner, family, `{"timezone":"Europe/Prague"}`); err != nil {
		t.Fatal(err)
	}
	stamp := func(s string) time.Time {
		x, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			t.Fatal(err)
		}
		return x
	}
	exercise := func(user uuid.UUID, start string) database.Exercise {
		r := database.Exercise{UserID: user, SourcePayloadID: uuid.New(), StartTime: stamp(start), EndTime: stamp(start).Add(time.Minute), DurationSeconds: 60, ExerciseType: "walking"}
		r.ID, r.FamilyID = uuid.New(), family
		if err := st.DB().Create(&r).Error; err != nil {
			t.Fatal(err)
		}
		return r
	}
	first := exercise(owner, "2025-10-26T02:30:00+02:00")  // repeated hour, earlier instant
	second := exercise(owner, "2025-10-26T02:30:00+01:00") // repeated hour, later instant
	tied := exercise(owner, "2025-10-26T01:30:00Z")        // same later instant, different persisted offset
	zero := 0.0
	if err := st.DB().Model(&tied).Updates(map[string]any{"distance_meters": zero, "exercise_type": strings.Repeat("walk", 200)}).Error; err != nil {
		t.Fatal(err)
	}
	exercise(uuid.New(), "2025-10-26T02:31:00+01:00")
	exercise(owner, "2025-10-26T00:00:00+02:00")      // included lower bound
	exercise(owner, "2025-10-25T21:59:59.999999999Z") // widened candidate before boundary, excluded exactly
	exercise(owner, "2025-10-26T22:59:59.999999999Z") // submillisecond before upper bound, included
	exercise(owner, "2025-10-27T00:00:00+01:00")      // exclusive upper bound
	deleted := exercise(owner, "2025-10-26T12:00:00Z")
	if err := st.DB().Delete(&deleted).Error; err != nil {
		t.Fatal(err)
	}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).PhoneMCPHandler("test-phone-token", owner.String(), nil)
	args := map[string]any{"period": "range", "start_date": "2025-10-26", "end_date": "2025-10-26", "limit": 1}
	seen := map[string]bool{}
	var previous time.Time
	var previousID string
	for {
		out := phoneSuccess(t, phoneCall(t, h, "list_activity_exercises", args))
		rows := out["exercises"].([]any)
		if len(rows) != 1 {
			t.Fatalf("wrong page %+v", out)
		}
		r := rows[0].(map[string]any)
		id := r["exercise_id"].(string)
		start := stamp(r["start_time"].(string))
		if seen[id] || (!previous.IsZero() && (start.After(previous) || (start.Equal(previous) && id >= previousID))) {
			t.Fatal("lost ordering or duplicate")
		}
		seen[id] = true
		previous, previousID = start, id
		if id == tied.ID.String() && (r["distance_meters"] != float64(0) || r["text_truncated"] != true) {
			t.Fatal("stored optional zero/text bound lost")
		}
		if id == first.ID.String() && r["distance_meters"] != nil {
			t.Fatal("unavailable metric fabricated")
		}
		if _, ok := r["source_payload_id"]; ok {
			t.Fatal("payload provenance leaked")
		}
		if out["has_more"] == false {
			if out["next_cursor"] != "" {
				t.Fatal("terminal cursor")
			}
			break
		}
		args["cursor"] = out["next_cursor"]
	}
	if len(seen) != 5 || !seen[first.ID.String()] || !seen[second.ID.String()] || !seen[tied.ID.String()] {
		t.Fatalf("lost or foreign exercises %+v", seen)
	}
	daily := phoneSuccess(t, phoneCall(t, h, "get_activity_daily_totals", map[string]any{"period": "range", "start_date": "2025-10-26", "end_date": "2025-10-26"}))["days"].([]any)[0].(map[string]any)
	if daily["exercise_records"] != float64(5) || daily["exercise_duration_seconds"] != float64(300) || daily["steps"] != nil {
		t.Fatalf("exercise steps or pagination affected total %+v", daily)
	}
	args["end_date"] = "2025-10-27"
	if phoneCall(t, h, "list_activity_exercises", args)["isError"] != true {
		t.Fatal("changed-period cursor accepted")
	}
	for _, tool := range []string{"list_activity_exercises", "get_activity_daily_totals"} {
		if phoneCall(t, h, tool, map[string]any{"period": "range", "start_date": "2099-01-01", "end_date": "2099-01-01"})["isError"] != true {
			t.Fatal("future accepted")
		}
	}
	if phoneCall(t, h, "list_activity_exercises", map[string]any{"limit": 21})["isError"] != true {
		t.Fatal("unbounded page accepted")
	}
	if phoneCall(t, h, "list_activity_exercises", map[string]any{"cursor": "bad"})["isError"] != true {
		t.Fatal("bad cursor accepted")
	}
	today := phoneSuccess(t, phoneCall(t, h, "get_activity_daily_totals", map[string]any{"period": "today"}))["days"].([]any)[0].(map[string]any)
	if today["partial_today"] != true {
		t.Fatal("today marked complete")
	}
	var stored database.Exercise
	if err := st.DB().First(&stored, "id = ?", tied.ID).Error; err != nil || len(stored.ExerciseType) != 800 || stored.DistanceMeters == nil || *stored.DistanceMeters != 0 {
		t.Fatal("read mutated exercise")
	}
}
