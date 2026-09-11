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

func TestRecordFoodAdviceEngagement_BoundsAndUTCNormalization(t *testing.T) {
	db := newFoodAdviceEngagementTestDB(t)
	userID, familyID := uuid.New(), uuid.New()
	// A non-UTC zone on the way in, so normalization is actually exercised.
	zone := time.FixedZone("UTC+05:30", 5*60*60+30*60)
	center := time.Date(2026, time.September, 9, 15, 30, 0, 123000000, zone)
	earlier := center.Add(-2 * time.Hour)
	later := center.Add(3 * time.Hour)
	bracketed := center.Add(time.Hour)
	const day = "2026-09-07"

	// Deliberately out of order: the first bound must end up at the earliest
	// event and the last at the latest, whatever order they arrive in, and a
	// bracketed event must move neither.
	for _, at := range []time.Time{center, earlier, later, bracketed} {
		if err := recordFoodAdviceEngagement(db, userID, familyID, day, qualifiedViewEvent, at); err != nil {
			t.Fatalf("record qualified view at %v: %v", at, err)
		}
	}

	var got database.FoodAdviceEngagement
	if err := db.Where("user_id = ? AND logged_day = ?", userID, day).First(&got).Error; err != nil {
		t.Fatalf("load aggregate: %v", err)
	}
	if got.FamilyID != familyID {
		t.Errorf("family ID = %s, want %s", got.FamilyID, familyID)
	}
	if got.QualifiedViewCount != 4 || got.FirstQualifiedViewAt == nil || got.LastQualifiedViewAt == nil {
		t.Fatalf("aggregate = count %d, bounds %v/%v",
			got.QualifiedViewCount, got.FirstQualifiedViewAt, got.LastQualifiedViewAt)
	}
	if !got.FirstQualifiedViewAt.Equal(earlier.UTC()) || !got.LastQualifiedViewAt.Equal(later.UTC()) {
		t.Errorf("bounds = %v/%v, want %v/%v",
			got.FirstQualifiedViewAt, got.LastQualifiedViewAt, earlier.UTC(), later.UTC())
	}
	if got.FirstQualifiedViewAt.Location() != time.UTC || got.LastQualifiedViewAt.Location() != time.UTC {
		t.Errorf("bounds were not normalized to UTC: %v/%v",
			got.FirstQualifiedViewAt.Location(), got.LastQualifiedViewAt.Location())
	}
}

// The refresh control is gone, and with it the two refresh event families.
// A caller naming one must be rejected exactly like any other unknown event,
// rather than silently recording nothing.
func TestRecordFoodAdviceEngagement_RetiredRefreshEventsAreUnknown(t *testing.T) {
	db := newFoodAdviceEngagementTestDB(t)
	userID, familyID := uuid.New(), uuid.New()
	at := time.Date(2026, time.September, 9, 12, 0, 0, 0, time.UTC)

	for _, event := range []string{"refresh_request", "refresh_success"} {
		if err := recordFoodAdviceEngagement(db, userID, familyID, "2026-09-09", event, at); err == nil {
			t.Fatalf("%s was accepted after the refresh control was removed", event)
		}
	}
	var count int64
	if err := db.Model(&database.FoodAdviceEngagement{}).Count(&count).Error; err != nil {
		t.Fatalf("count aggregates: %v", err)
	}
	if count != 0 {
		t.Fatalf("aggregate rows = %d, want none", count)
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
