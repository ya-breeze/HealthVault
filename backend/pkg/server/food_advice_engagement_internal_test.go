package server

import (
	"io"
	"log/slog"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ya-breeze/healthvault/pkg/database"
)

func newFoodAdviceEngagementTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := database.Open(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		filepath.Join(t.TempDir(), "food-advice-engagement.db"),
	)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	return db
}

func TestRecordFoodAdviceEngagement_EventFamiliesAndBounds(t *testing.T) {
	db := newFoodAdviceEngagementTestDB(t)
	userID, familyID := uuid.New(), uuid.New()
	zone := time.FixedZone("UTC+05:30", 5*60*60+30*60)
	center := time.Date(2026, time.September, 9, 15, 30, 0, 123000000, zone)
	earlier := center.Add(-2 * time.Hour)
	later := center.Add(3 * time.Hour)
	bracketed := center.Add(time.Hour)

	tests := []struct {
		name      string
		day       string
		event     string
		seedFirst bool
	}{
		{name: "qualified view", day: "2026-09-07", event: qualifiedViewEvent},
		{name: "refresh request", day: "2026-09-08", event: "refresh_request"},
		{name: "refresh success", day: "2026-09-09", event: "refresh_success", seedFirst: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			requestAt := center.Add(-time.Minute)
			if tc.seedFirst {
				if err := recordFoodAdviceEngagement(
					db, userID, familyID, tc.day, "refresh_request", requestAt,
				); err != nil {
					t.Fatalf("seed refresh request: %v", err)
				}
			}

			for _, at := range []time.Time{center, earlier, later, bracketed} {
				if err := recordFoodAdviceEngagement(db, userID, familyID, tc.day, tc.event, at); err != nil {
					t.Fatalf("record %s at %v: %v", tc.event, at, err)
				}
			}

			var got database.FoodAdviceEngagement
			if err := db.Where("user_id = ? AND logged_day = ?", userID, tc.day).First(&got).Error; err != nil {
				t.Fatalf("load aggregate: %v", err)
			}
			if got.FamilyID != familyID {
				t.Errorf("family ID = %s, want %s", got.FamilyID, familyID)
			}

			assertBounds := func(count uint64, first, last *time.Time) {
				t.Helper()
				if count != 4 || first == nil || last == nil {
					t.Fatalf("selected aggregate = count %d, bounds %v/%v", count, first, last)
				}
				if !first.Equal(earlier.UTC()) || !last.Equal(later.UTC()) {
					t.Errorf("bounds = %v/%v, want %v/%v", first, last, earlier.UTC(), later.UTC())
				}
				if first.Location() != time.UTC || last.Location() != time.UTC {
					t.Errorf("bounds were not normalized to UTC: %v/%v", first.Location(), last.Location())
				}
			}

			switch tc.event {
			case qualifiedViewEvent:
				assertBounds(got.QualifiedViewCount, got.FirstQualifiedViewAt, got.LastQualifiedViewAt)
				if got.RefreshRequestCount != 0 || got.FirstRefreshRequestAt != nil || got.LastRefreshRequestAt != nil ||
					got.RefreshSuccessCount != 0 || got.FirstRefreshSuccessAt != nil || got.LastRefreshSuccessAt != nil {
					t.Errorf("qualified view changed unrelated fields: %+v", got)
				}
			case "refresh_request":
				assertBounds(got.RefreshRequestCount, got.FirstRefreshRequestAt, got.LastRefreshRequestAt)
				if got.QualifiedViewCount != 0 || got.FirstQualifiedViewAt != nil || got.LastQualifiedViewAt != nil ||
					got.RefreshSuccessCount != 0 || got.FirstRefreshSuccessAt != nil || got.LastRefreshSuccessAt != nil {
					t.Errorf("refresh request changed unrelated fields: %+v", got)
				}
			case "refresh_success":
				assertBounds(got.RefreshSuccessCount, got.FirstRefreshSuccessAt, got.LastRefreshSuccessAt)
				if got.QualifiedViewCount != 0 || got.FirstQualifiedViewAt != nil || got.LastQualifiedViewAt != nil ||
					got.RefreshRequestCount != 1 || got.FirstRefreshRequestAt == nil || got.LastRefreshRequestAt == nil ||
					!got.FirstRefreshRequestAt.Equal(requestAt.UTC()) || !got.LastRefreshRequestAt.Equal(requestAt.UTC()) {
					t.Errorf("refresh success changed unrelated fields: %+v", got)
				}
			}
		})
	}
}

func TestRecordFoodAdviceEngagement_UnknownEventDoesNotMutate(t *testing.T) {
	db := newFoodAdviceEngagementTestDB(t)
	userID, familyID := uuid.New(), uuid.New()
	at := time.Date(2026, time.September, 9, 12, 0, 0, 0, time.UTC)

	if err := recordFoodAdviceEngagement(db, userID, familyID, "2026-09-08", "unknown", at); err == nil {
		t.Fatal("unknown event error = nil")
	}
	var count int64
	if err := db.Model(&database.FoodAdviceEngagement{}).Count(&count).Error; err != nil {
		t.Fatalf("count aggregates: %v", err)
	}
	if count != 0 {
		t.Fatalf("aggregate rows = %d, want none", count)
	}

	const day = "2026-09-09"
	if err := recordFoodAdviceEngagement(db, userID, familyID, day, qualifiedViewEvent, at); err != nil {
		t.Fatalf("seed aggregate: %v", err)
	}
	var before database.FoodAdviceEngagement
	if err := db.Where("user_id = ? AND logged_day = ?", userID, day).First(&before).Error; err != nil {
		t.Fatalf("load aggregate before unknown event: %v", err)
	}
	if err := recordFoodAdviceEngagement(db, userID, familyID, day, "unknown", at.Add(time.Hour)); err == nil {
		t.Fatal("unknown event error = nil for existing aggregate")
	}
	var after database.FoodAdviceEngagement
	if err := db.First(&after, "id = ?", before.ID).Error; err != nil {
		t.Fatalf("load aggregate after unknown event: %v", err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Errorf("aggregate changed after unknown event:\n before: %+v\n  after: %+v", before, after)
	}
}
