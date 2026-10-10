package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ya-breeze/healthvault/pkg/database"
)

type phoneActivityDay struct {
	database.ActivityDay
	PartialToday bool `json:"partial_today"`
}

type phoneActivityResult struct {
	phonePeriod
	Days  []phoneActivityDay `json:"days"`
	Notes []string           `json:"notes"`
}

type phoneExercise struct {
	ExerciseID          string    `json:"exercise_id"`
	LocalDate           string    `json:"local_date"`
	StartTime           time.Time `json:"start_time"`
	EndTime             time.Time `json:"end_time"`
	ExerciseType        string    `json:"exercise_type"`
	ExerciseTypeName    *string   `json:"exercise_type_name"`
	ExerciseTypeMapping string    `json:"exercise_type_mapping"`
	TextTruncated       bool      `json:"text_truncated"`
	DurationSeconds     int       `json:"duration_seconds"`
	DistanceMeters      *float64  `json:"distance_meters"`
	Steps               *int      `json:"steps"`
	AvgCadenceSpm       *float64  `json:"avg_cadence_steps_per_minute"`
	MaxCadenceSpm       *float64  `json:"max_cadence_steps_per_minute"`
	StrideLengthM       *float64  `json:"stride_length_meters"`
}

type phoneExerciseResult struct {
	phonePeriod
	Exercises  []phoneExercise `json:"exercises"`
	HasMore    bool            `json:"has_more"`
	NextCursor string          `json:"next_cursor"`
	Notes      []string        `json:"notes"`
}

type phoneExerciseCursor struct {
	StartDate string    `json:"start_date"`
	EndDate   string    `json:"end_date"`
	Timezone  string    `json:"timezone"`
	StartTime time.Time `json:"start_time"`
	ID        uuid.UUID `json:"id"`
}

var phoneActivityNotes = []string{
	"Evidence is stored imported activity. Record counts do not establish complete 24-hour sensor coverage. Today is partial.",
	"Null totals mean no records, not zero activity. Zero with records is the recorded aggregate; check kept/dropped step counts when interpreting it.",
	"Whole intervals belong to their local start date, even across midnight. Intervals starting before the selected period are omitted; no proportional counts or duration are invented.",
	"Steps use Step Interval Collapse across the selected period: covered intervals drop; partial overlaps remain whole. Different period boundaries can change collapse. This is not complete deduplication or exact daily activity.",
	"Exercise durations and calorie intervals are summed as recorded and may overlap. Exercise steps are not added to daily steps. Do not add active and total calories together or infer energy balance from this evidence.",
	"exercise_type preserves the stored value. Use exercise_type_name for readable names. health_connect mapping interprets numeric values using Android Health Connect session constants, not device/source provenance. stored_text names preserve supplied text. unknown mapping has a null name; do not invent a workout type.",
	"Calories are kcal, duration is seconds, distance/stride are meters, cadence is steps per minute. Null exercise fields are unavailable; zero is a stored value. Exercise calories and device/origin provenance are unavailable.",
}

func phoneActivityError(err error) *mcp.CallToolResult {
	if errors.Is(err, database.ErrActivityLimit) {
		return phoneReadError(err)
	}
	return phoneReadError(errors.New("activity evidence unavailable"))
}

func phoneEvidenceFits(out any) bool {
	encoded, err := json.Marshal(out)
	return err == nil && len(encoded) <= 64*1024
}

func (h *foodHandlers) addPhoneActivityTools(s *mcp.Server, id uuid.UUID, barrier *sync.RWMutex) {
	closed := false
	read := &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closed}
	mcp.AddTool(s, &mcp.Tool{Name: "get_activity_daily_totals", Description: "Read connected user's recorded steps, exercise duration and separate active/total kcal for every local day. Default yesterday; last_7_days excludes today. Null means missing records. Read notes about interval allocation, overlap and incomplete sensor coverage; never add active and total calories or exercise steps to daily steps.", Annotations: read}, func(ctx context.Context, _ *mcp.CallToolRequest, in phonePeriodInput) (*mcp.CallToolResult, phoneActivityResult, error) {
		barrier.RLock()
		defer barrier.RUnlock()
		p, loc, _, err := h.phoneReadPeriod(id, in)
		if err != nil {
			return phoneReadError(err), phoneActivityResult{}, nil
		}
		days, err := database.ActivityDailyRange(h.storage.DB().WithContext(ctx), id, loc, p.StartDate, p.EndDate)
		if err != nil {
			return phoneActivityError(err), phoneActivityResult{}, nil
		}
		out := phoneActivityResult{phonePeriod: p, Days: []phoneActivityDay{}, Notes: phoneActivityNotes}
		for _, day := range days {
			out.Days = append(out.Days, phoneActivityDay{ActivityDay: day, PartialToday: day.Date == p.Today})
		}
		if !phoneEvidenceFits(out) {
			return phoneReadError(errors.New("activity result too large; request a shorter period")), phoneActivityResult{}, nil
		}
		return nil, out, nil
	})
	mcp.AddTool(s, &mcp.Tool{Name: "list_activity_exercises", Description: "List connected user's stored exercise sessions by local start date. Default yesterday. Follow next_cursor for the complete list. Returns stored type plus readable exercise_type_name and explicit mapping; unknown numeric codes have a null name. Returns interval, duration, nullable distance/steps/cadence/stride; no exercise kcal or device provenance. Use get_activity_daily_totals for complete aggregates independent of pagination.", Annotations: read}, func(ctx context.Context, _ *mcp.CallToolRequest, in phoneHistoryInput) (*mcp.CallToolResult, phoneExerciseResult, error) {
		barrier.RLock()
		defer barrier.RUnlock()
		p, loc, _, err := h.phoneReadPeriod(id, in.phonePeriodInput)
		if err != nil {
			return phoneReadError(err), phoneExerciseResult{}, nil
		}
		limit := in.Limit
		if limit == 0 {
			limit = 10
		}
		if limit < 1 || limit > 20 {
			return phoneReadError(errors.New("limit must be 1 to 20")), phoneExerciseResult{}, nil
		}
		var cursor phoneExerciseCursor
		if in.Cursor != "" {
			if len(in.Cursor) > 2048 {
				return phoneReadError(errors.New("invalid cursor; reuse next_cursor with the same dates and timezone")), phoneExerciseResult{}, nil
			}
			decoded, err := base64.RawURLEncoding.DecodeString(in.Cursor)
			if err != nil || json.Unmarshal(decoded, &cursor) != nil || cursor.ID == uuid.Nil || cursor.StartTime.IsZero() || cursor.StartDate != p.StartDate || cursor.EndDate != p.EndDate || cursor.Timezone != p.Timezone {
				return phoneReadError(errors.New("invalid cursor; reuse next_cursor with the same dates and timezone")), phoneExerciseResult{}, nil
			}
		}
		from, to := phoneWindow(p, loc)
		rows, err := database.ActivityExercises(h.storage.DB().WithContext(ctx), id, from, to)
		if err != nil {
			return phoneActivityError(err), phoneExerciseResult{}, nil
		}
		out := phoneExerciseResult{phonePeriod: p, Exercises: []phoneExercise{}, Notes: phoneActivityNotes}
		for _, row := range rows {
			if in.Cursor != "" && (row.StartTime.After(cursor.StartTime) || (row.StartTime.Equal(cursor.StartTime) && row.ID.String() >= cursor.ID.String())) {
				continue
			}
			if len(out.Exercises) == limit {
				out.HasMore = true
				break
			}
			kind, typeName, mapping, truncated := phoneExerciseType(row.ExerciseType)
			out.Exercises = append(out.Exercises, phoneExercise{ExerciseID: row.ID.String(), LocalDate: database.LocalDate(row.StartTime, loc), StartTime: row.StartTime, EndTime: row.EndTime, ExerciseType: kind, ExerciseTypeName: typeName, ExerciseTypeMapping: mapping, TextTruncated: truncated, DurationSeconds: row.DurationSeconds, DistanceMeters: row.DistanceMeters, Steps: row.Steps, AvgCadenceSpm: row.AvgCadenceSpm, MaxCadenceSpm: row.MaxCadenceSpm, StrideLengthM: row.StrideLengthM})
		}
		if out.HasMore {
			last := out.Exercises[len(out.Exercises)-1]
			encoded, _ := json.Marshal(phoneExerciseCursor{StartDate: p.StartDate, EndDate: p.EndDate, Timezone: p.Timezone, StartTime: last.StartTime, ID: uuid.MustParse(last.ExerciseID)})
			out.NextCursor = base64.RawURLEncoding.EncodeToString(encoded)
		}
		if !phoneEvidenceFits(out) {
			return phoneReadError(errors.New("exercise result too large; request a smaller limit")), phoneExerciseResult{}, nil
		}
		return nil, out, nil
	})
}
