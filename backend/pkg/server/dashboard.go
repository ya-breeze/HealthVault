package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/ya-breeze/healthvault/pkg/database"
)

// dashboardPrimaryMetrics is the complete set of Vital Cards whose daily
// aggregate data is part of the dashboard read model. Keep this registry
// fixed: the frontend contract and the response's eight aggregate entries
// both depend on these exact names.
var dashboardPrimaryMetrics = [...]string{
	"steps",
	"heart_rate",
	"sleep",
	"heart_rate_variability",
	"distance",
	"weight",
	"blood_pressure",
	"oxygen_saturation",
}

type dashboardSectionStatus string

const (
	dashboardStatusOK    dashboardSectionStatus = "ok"
	dashboardStatusError dashboardSectionStatus = "error"
)

// DashboardSettingsSection is the settings branch of the dashboard read
// model. Value deliberately remains raw JSON so settings keys unknown to this
// server survive the read-model boundary unchanged.
type DashboardSettingsSection struct {
	Status dashboardSectionStatus
	Value  json.RawMessage
}

func (s DashboardSettingsSection) MarshalJSON() ([]byte, error) {
	if s.Status != dashboardStatusOK {
		return json.Marshal(struct {
			Status dashboardSectionStatus `json:"status"`
		}{Status: s.Status})
	}
	value := s.Value
	if len(value) == 0 {
		value = json.RawMessage(`{}`)
	}
	return json.Marshal(struct {
		Status dashboardSectionStatus `json:"status"`
		Value  json.RawMessage        `json:"value"`
	}{Status: s.Status, Value: value})
}

// DashboardPresenceSection is the complete all-time data-type presence
// signal. A failed presence query has no value branch.
type DashboardPresenceSection struct {
	Status dashboardSectionStatus
	Value  map[string]bool
}

func (s DashboardPresenceSection) MarshalJSON() ([]byte, error) {
	if s.Status != dashboardStatusOK {
		return json.Marshal(struct {
			Status dashboardSectionStatus `json:"status"`
		}{Status: s.Status})
	}
	value := s.Value
	if value == nil {
		value = map[string]bool{}
	}
	return json.Marshal(struct {
		Status dashboardSectionStatus `json:"status"`
		Value  map[string]bool        `json:"value"`
	}{Status: s.Status, Value: value})
}

// DashboardAggregateSection is one independently-fetched daily aggregate.
// Its custom JSON representation is needed to distinguish a successful empty
// rows array from an error branch with no rows field.
type DashboardAggregateSection struct {
	Status dashboardSectionStatus
	Rows   []map[string]any
}

func (s DashboardAggregateSection) MarshalJSON() ([]byte, error) {
	if s.Status != dashboardStatusOK {
		return json.Marshal(struct {
			Status dashboardSectionStatus `json:"status"`
		}{Status: s.Status})
	}
	rows := s.Rows
	if rows == nil {
		rows = []map[string]any{}
	}
	return json.Marshal(struct {
		Status dashboardSectionStatus `json:"status"`
		Rows   []map[string]any       `json:"rows"`
	}{Status: s.Status, Rows: rows})
}

// DashboardNeedsAttentionSection is the Food Meal needs-attention count.
type DashboardNeedsAttentionSection struct {
	Status dashboardSectionStatus
	Count  int64
}

func (s DashboardNeedsAttentionSection) MarshalJSON() ([]byte, error) {
	if s.Status != dashboardStatusOK {
		return json.Marshal(struct {
			Status dashboardSectionStatus `json:"status"`
		}{Status: s.Status})
	}
	return json.Marshal(struct {
		Status dashboardSectionStatus `json:"status"`
		Count  int64                  `json:"count"`
	}{Status: s.Status, Count: s.Count})
}

// DashboardReadModel is the authenticated dashboard response envelope.
type DashboardReadModel struct {
	Settings       DashboardSettingsSection             `json:"settings"`
	Presence       DashboardPresenceSection             `json:"presence"`
	Aggregates     map[string]DashboardAggregateSection `json:"aggregates"`
	NeedsAttention DashboardNeedsAttentionSection       `json:"needs_attention"`
}

func newDashboardReadModel() DashboardReadModel {
	aggregates := make(map[string]DashboardAggregateSection, len(dashboardPrimaryMetrics))
	for _, metric := range dashboardPrimaryMetrics {
		aggregates[metric] = DashboardAggregateSection{Status: dashboardStatusError}
	}
	return DashboardReadModel{
		Settings:       DashboardSettingsSection{Status: dashboardStatusError},
		Presence:       DashboardPresenceSection{Status: dashboardStatusError},
		Aggregates:     aggregates,
		NeedsAttention: DashboardNeedsAttentionSection{Status: dashboardStatusError},
	}
}

// dashboardAggregateRange is the same eight-UTC-day over-fetch used by the
// dashboard's former browser request wave. The upper bound is the one instant
// captured by DashboardHandler; the lower bound is the UTC midnight eight
// calendar days before that instant's UTC date.
func dashboardAggregateRange(now time.Time) database.TimeRange {
	now = now.UTC()
	fromDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -8)
	return database.TimeRange{From: fromDate, To: now}
}

func dashboardLoggedDayCutoff(now time.Time, loc *time.Location) string {
	return database.LocalDate(now.UTC().In(loc).AddDate(0, 0, -6), loc)
}

func dashboardRowsFromLatestSevenLoggedDays(rows []map[string]any, cutoff string) []map[string]any {
	filtered := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		bucketStart, ok := row["bucket_start"].(string)
		if !ok || len(bucketStart) < len("2006-01-02") {
			continue
		}
		if bucketStart[:len("2006-01-02")] >= cutoff {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

// buildDashboardReadModel assembles all sections under one authenticated user
// and one caller-supplied instant. It deliberately returns no aggregate error:
// each source owns its own discriminated status in the envelope.
func buildDashboardReadModel(storage database.Storage, userID uuid.UUID, now time.Time) DashboardReadModel {
	model := newDashboardReadModel()

	presence, err := dataTypesPresence(storage.DB(), userID)
	if err != nil {
		slog.Error("dashboard: read presence", "err", err, "user_id", userID)
	} else {
		model.Presence = DashboardPresenceSection{Status: dashboardStatusOK, Value: presence}
	}

	count, err := needsAttentionCount(storage.DB(), userID)
	if err != nil {
		slog.Error("dashboard: read needs attention", "err", err, "user_id", userID)
	} else {
		model.NeedsAttention = DashboardNeedsAttentionSection{Status: dashboardStatusOK, Count: count}
	}

	settingsJSON, err := readUserSettingsJSON(storage, userID)
	if err != nil {
		slog.Error("dashboard: read settings", "err", err, "user_id", userID)
		return model
	}
	if settingsJSON == "" {
		settingsJSON = `{}`
	}
	var settingsObject map[string]json.RawMessage
	if err := json.Unmarshal([]byte(settingsJSON), &settingsObject); err != nil || settingsObject == nil {
		if err == nil {
			err = fmt.Errorf("settings JSON is not an object")
		}
		slog.Error("dashboard: decode settings", "err", err, "user_id", userID)
		return model
	}
	model.Settings = DashboardSettingsSection{
		Status: dashboardStatusOK,
		Value:  json.RawMessage(settingsJSON),
	}

	loc := database.ResolveTimezone(settingsJSON)
	tr := dashboardAggregateRange(now)
	cutoff := dashboardLoggedDayCutoff(now, loc)
	for _, metric := range dashboardPrimaryMetrics {
		rows, err := queryBucketed(storage, metric, typeRegistry[metric], database.BucketDay, loc, userID, tr)
		if err != nil {
			slog.Error("dashboard: read aggregate", "metric", metric, "err", err, "user_id", userID)
			continue
		}
		model.Aggregates[metric] = DashboardAggregateSection{
			Status: dashboardStatusOK,
			Rows:   dashboardRowsFromLatestSevenLoggedDays(rows, cutoff),
		}
	}

	return model
}

// DashboardHandler serves the authenticated dashboard read model. The
// claims user is the only possible target: this self-only endpoint never
// resolves a family member and ignores any user query parameter.
func DashboardHandler(storage database.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := ClaimsFromCtx(r)
		if claims == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		now := time.Now().UTC()
		writeJSON(w, buildDashboardReadModel(storage, claims.UserID, now))
	}
}
