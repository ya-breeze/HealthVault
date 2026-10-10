package server_test

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/server"
)

func TestPhoneExerciseTypeNamesPreserveEvidence(t *testing.T) {
	st := newFoodTestStorage(t)
	owner, family := seedFoodUser(t, st)
	if err := st.UpsertUserSettings(owner, family, `{"timezone":"Europe/Prague"}`); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		raw, name, mapping string
		truncated          bool
	}{
		{"79", "Walking", "health_connect", false},
		{"56", "Running", "health_connect", false},
		{"0", "Other workout", "health_connect", false},
		{"37", "Hiking", "health_connect", false},
		{"59", "Scuba diving", "health_connect", false},
		{"82", "Wheelchair", "health_connect", false},
		{"83", "Yoga", "health_connect", false},
		{" 79 ", "Walking", "health_connect", false},
		{"9999", "", "unknown", false},
		{"-56", "", "unknown", false},
		{"-0", "", "unknown", false},
		{"+79", "", "unknown", false},
		{"079", "Walking", "health_connect", false},
		{"  ", "", "unknown", false},
		{"", "", "unknown", false},
		{"walking", "walking", "stored_text", false},
		{"Morning walk", "Morning walk", "stored_text", false},
		{strings.Repeat("ходьба", 100), strings.Repeat("ходьба", 100), "stored_text", true},
		{"79" + strings.Repeat("0", 600), "", "unknown", true},
		{strings.Repeat("0", 600) + "79", "", "unknown", true},
	}
	ids := map[string]int{}
	for i, c := range cases {
		start := time.Date(2025, 1, 15, 12, i, 0, 0, time.UTC)
		r := database.Exercise{UserID: owner, SourcePayloadID: uuid.New(), StartTime: start, EndTime: start.Add(time.Minute), DurationSeconds: 60, ExerciseType: c.raw}
		r.ID, r.FamilyID = uuid.New(), family
		if err := st.DB().Create(&r).Error; err != nil {
			t.Fatal(err)
		}
		ids[r.ID.String()] = i
	}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).PhoneMCPHandler("test-phone-token", owner.String(), nil)
	args := map[string]any{"period": "range", "start_date": "2025-01-15", "end_date": "2025-01-15", "limit": 20}
	out := phoneSuccess(t, phoneCall(t, h, "list_activity_exercises", args))
	rows := out["exercises"].([]any)
	if len(rows) != len(cases) || out["has_more"] != false {
		t.Fatalf("lost exercises: %+v", out)
	}
	for _, row := range rows {
		r := row.(map[string]any)
		id := r["exercise_id"].(string)
		index, ok := ids[id]
		if !ok {
			t.Fatal("unexpected exercise")
		}
		delete(ids, id)
		c := cases[index]
		expectedRaw := string([]rune(c.raw)[:min(utf8.RuneCountInString(c.raw), 512)])
		if r["exercise_type"] != expectedRaw || r["exercise_type_mapping"] != c.mapping || r["text_truncated"] != c.truncated {
			t.Fatalf("case %d: raw/mapping/bound mismatch: %+v", index, r)
		}
		if c.name == "" {
			if name, present := r["exercise_type_name"]; !present || name != nil {
				t.Fatalf("unknown type must have an explicit null name: %+v", r)
			}
		} else if r["exercise_type_name"] != string([]rune(c.name)[:min(utf8.RuneCountInString(c.name), 512)]) {
			t.Fatalf("case %d: wrong name: %+v", index, r)
		}
		var stored database.Exercise
		if err := st.DB().First(&stored, "id = ?", id).Error; err != nil || stored.ExerciseType != c.raw || stored.DurationSeconds != 60 {
			t.Fatal("name lookup changed persisted exercise")
		}
	}
	if len(ids) != 0 {
		t.Fatal("duplicate or missing exercise")
	}
}
