package server

import (
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ya-breeze/healthvault/pkg/database"
	kinmodels "github.com/ya-breeze/kin-core/models"
	"gorm.io/gorm"
)

func newDashboardInternalStorage(t *testing.T) (database.Storage, uuid.UUID, uuid.UUID) {
	t.Helper()
	db, err := database.Open(slog.New(slog.NewTextHandler(io.Discard, nil)), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	storage := database.NewStorage(db)
	familyID := uuid.New()
	userID := uuid.New()
	if err := db.Create(&kinmodels.Family{ID: familyID, Name: "DashboardFamily"}).Error; err != nil {
		t.Fatalf("create family: %v", err)
	}
	if err := db.Create(&kinmodels.User{ID: userID, Username: "dashboard-user", PasswordHash: "x", FamilyID: familyID}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return storage, userID, familyID
}

func seedDashboardSteps(t *testing.T, storage database.Storage, userID, familyID uuid.UUID, at time.Time, count int) {
	t.Helper()
	row := database.Steps{
		UserID: userID, SourcePayloadID: uuid.New(), StartTime: at.UTC(), EndTime: at.UTC().Add(time.Minute), Count: count,
	}
	row.ID = uuid.New()
	row.FamilyID = familyID
	if err := storage.DB().Create(&row).Error; err != nil {
		t.Fatalf("create steps: %v", err)
	}
}

func seedDashboardHeartRate(t *testing.T, storage database.Storage, userID, familyID uuid.UUID, at time.Time, bpm int) {
	t.Helper()
	row := database.HeartRate{UserID: userID, SourcePayloadID: uuid.New(), Time: at.UTC(), BPM: bpm}
	row.ID = uuid.New()
	row.FamilyID = familyID
	if err := storage.DB().Create(&row).Error; err != nil {
		t.Fatalf("create heart rate: %v", err)
	}
}

func TestBuildDashboardReadModel_UsesSavedTimezoneAndLatestSevenLoggedDays(t *testing.T) {
	storage, userID, familyID := newDashboardInternalStorage(t)
	if err := storage.UpsertUserSettings(userID, familyID, `{"timezone":"Asia/Tokyo","unknown_key":{"survives":true}}`); err != nil {
		t.Fatalf("settings: %v", err)
	}

	now := time.Date(2026, time.September, 19, 0, 30, 0, 0, time.UTC)
	loc, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	// The first pair straddles UTC midnight but belongs to two different
	// Tokyo Logged Days. The remaining rows cover eight consecutive Logged
	// Days so the cutoff must retain exactly the newest seven.
	seedDashboardHeartRate(t, storage, userID, familyID, time.Date(2026, time.September, 12, 14, 30, 0, 0, time.UTC), 61)
	seedDashboardHeartRate(t, storage, userID, familyID, time.Date(2026, time.September, 12, 15, 30, 0, 0, time.UTC), 62)
	for day := 14; day <= 19; day++ {
		hour := 12
		if day == 19 {
			hour = 8 // before fixed now's 09:30 Tokyo local time
		}
		local := time.Date(2026, time.September, day, hour, 0, 0, 0, loc)
		seedDashboardHeartRate(t, storage, userID, familyID, local, 60+day)
	}

	model := buildDashboardReadModel(storage, userID, now)
	if model.Settings.Status != dashboardStatusOK {
		t.Fatalf("settings status = %q, want ok", model.Settings.Status)
	}
	rows := model.Aggregates["heart_rate"]
	if rows.Status != dashboardStatusOK {
		t.Fatalf("heart_rate status = %q, want ok", rows.Status)
	}
	if len(rows.Rows) != 7 {
		t.Fatalf("heart_rate rows = %d, want exactly seven Logged Days: %+v", len(rows.Rows), rows.Rows)
	}
	if got := rows.Rows[0]["bucket_start"]; got != "2026-09-13T00:00:00Z" {
		t.Errorf("first bucket = %v, want Tokyo Logged Day 2026-09-13", got)
	}
	if got := rows.Rows[len(rows.Rows)-1]["bucket_start"]; got != "2026-09-19T00:00:00Z" {
		t.Errorf("last bucket = %v, want Tokyo Logged Day 2026-09-19", got)
	}
	range_ := dashboardAggregateRange(now)
	if !range_.From.Equal(time.Date(2026, time.September, 11, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("aggregate from = %s, want 2026-09-11T00:00:00Z extra-day over-fetch", range_.From)
	}
	if !range_.To.Equal(now) {
		t.Errorf("aggregate to = %s, want captured now %s", range_.To, now)
	}
	if got := dashboardLoggedDayCutoff(now, loc); got != "2026-09-13" {
		t.Errorf("cutoff = %q, want 2026-09-13", got)
	}
}

func TestBuildDashboardReadModel_SettingsFailureLeavesIndependentSections(t *testing.T) {
	storage, userID, _ := newDashboardInternalStorage(t)
	if err := storage.DB().Exec("DROP TABLE user_settings").Error; err != nil {
		t.Fatalf("drop settings table: %v", err)
	}

	model := buildDashboardReadModel(storage, userID, time.Now().UTC())
	if model.Settings.Status != dashboardStatusError {
		t.Errorf("settings status = %q, want error", model.Settings.Status)
	}
	if model.Presence.Status != dashboardStatusOK {
		t.Errorf("presence status = %q, want ok", model.Presence.Status)
	}
	if model.NeedsAttention.Status != dashboardStatusOK {
		t.Errorf("needs_attention status = %q, want ok", model.NeedsAttention.Status)
	}
	for _, metric := range dashboardPrimaryMetrics {
		if model.Aggregates[metric].Status != dashboardStatusError {
			t.Errorf("aggregate %q status = %q, want error", metric, model.Aggregates[metric].Status)
		}
	}
}

func TestBuildDashboardReadModel_PresenceFailureIsIsolated(t *testing.T) {
	storage, userID, familyID := newDashboardInternalStorage(t)
	if err := storage.UpsertUserSettings(userID, familyID, `{}`); err != nil {
		t.Fatalf("settings: %v", err)
	}
	db := storage.DB()
	const callbackName = "test:dashboard:presence-failure"
	db.Callback().Row().Before("gorm:row").Register(callbackName, func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "UNION ALL") {
			tx.AddError(errors.New("injected presence failure"))
		}
	})
	t.Cleanup(func() { _ = db.Callback().Row().Remove(callbackName) })

	model := buildDashboardReadModel(storage, userID, time.Now().UTC())
	if model.Presence.Status != dashboardStatusError {
		t.Errorf("presence status = %q, want error", model.Presence.Status)
	}
	if model.Settings.Status != dashboardStatusOK || model.NeedsAttention.Status != dashboardStatusOK {
		t.Errorf("independent sections changed: settings=%q needs=%q", model.Settings.Status, model.NeedsAttention.Status)
	}
	for _, metric := range dashboardPrimaryMetrics {
		if model.Aggregates[metric].Status != dashboardStatusOK {
			t.Errorf("aggregate %q status = %q, want ok", metric, model.Aggregates[metric].Status)
		}
	}
}

func TestBuildDashboardReadModel_AggregateFailuresAreMetricIsolated(t *testing.T) {
	for _, tc := range []struct {
		metric string
		table  string
	}{
		{metric: "heart_rate", table: "heart_rates"},
		{metric: "steps", table: "steps"},
		{metric: "blood_pressure", table: "blood_pressures"},
	} {
		t.Run(tc.metric, func(t *testing.T) {
			storage, userID, familyID := newDashboardInternalStorage(t)
			if err := storage.UpsertUserSettings(userID, familyID, `{}`); err != nil {
				t.Fatalf("settings: %v", err)
			}
			if err := storage.DB().Exec("DROP TABLE " + tc.table).Error; err != nil {
				t.Fatalf("drop %s: %v", tc.table, err)
			}

			model := buildDashboardReadModel(storage, userID, time.Now().UTC())
			if model.Aggregates[tc.metric].Status != dashboardStatusError {
				t.Errorf("aggregate %q status = %q, want error", tc.metric, model.Aggregates[tc.metric].Status)
			}
			for _, metric := range dashboardPrimaryMetrics {
				if metric != tc.metric && model.Aggregates[metric].Status != dashboardStatusOK {
					t.Errorf("sibling aggregate %q status = %q, want ok", metric, model.Aggregates[metric].Status)
				}
			}
		})
	}
}

func TestBuildDashboardReadModel_NeedsAttentionFailureIsIsolated(t *testing.T) {
	storage, userID, familyID := newDashboardInternalStorage(t)
	if err := storage.UpsertUserSettings(userID, familyID, `{}`); err != nil {
		t.Fatalf("settings: %v", err)
	}
	db := storage.DB()
	const callbackName = "test:dashboard:needs-attention-failure"
	db.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		sql := strings.ToUpper(tx.Statement.SQL.String())
		if strings.Contains(sql, "FOOD_MEALS") && strings.Contains(sql, "COUNT(") {
			tx.AddError(errors.New("injected needs-attention failure"))
		}
	})
	t.Cleanup(func() { _ = db.Callback().Query().Remove(callbackName) })

	model := buildDashboardReadModel(storage, userID, time.Now().UTC())
	if model.NeedsAttention.Status != dashboardStatusError {
		t.Errorf("needs_attention status = %q, want error", model.NeedsAttention.Status)
	}
	if model.Settings.Status != dashboardStatusOK || model.Presence.Status != dashboardStatusOK {
		t.Errorf("independent sections changed: settings=%q presence=%q", model.Settings.Status, model.Presence.Status)
	}
	for _, metric := range dashboardPrimaryMetrics {
		if model.Aggregates[metric].Status != dashboardStatusOK {
			t.Errorf("aggregate %q status = %q, want ok", metric, model.Aggregates[metric].Status)
		}
	}
}
