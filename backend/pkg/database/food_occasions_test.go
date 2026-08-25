package database_test

import (
	"testing"
	"time"

	"github.com/ya-breeze/healthvault/pkg/database"
)

func mustParse(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse("2006-01-02T15:04:05Z", s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return ts
}

func TestCollapseOccasions(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		if got := database.CollapseOccasions(nil); got != 0 {
			t.Errorf("CollapseOccasions(nil) = %d, want 0", got)
		}
	})

	t.Run("single timestamp", func(t *testing.T) {
		ts := []time.Time{mustParse(t, "2026-08-21T09:10:00Z")}
		if got := database.CollapseOccasions(ts); got != 1 {
			t.Errorf("CollapseOccasions(1 ts) = %d, want 1", got)
		}
	})

	t.Run("two timestamps 3 minutes apart", func(t *testing.T) {
		ts := []time.Time{
			mustParse(t, "2026-08-21T09:00:00Z"),
			mustParse(t, "2026-08-21T09:03:00Z"),
		}
		if got := database.CollapseOccasions(ts); got != 1 {
			t.Errorf("CollapseOccasions(3min apart) = %d, want 1", got)
		}
	})

	t.Run("two timestamps exactly 10 minutes apart (inclusive boundary)", func(t *testing.T) {
		ts := []time.Time{
			mustParse(t, "2026-08-21T09:00:00Z"),
			mustParse(t, "2026-08-21T09:10:00Z"),
		}
		if got := database.CollapseOccasions(ts); got != 1 {
			t.Errorf("CollapseOccasions(exactly 10min apart) = %d, want 1", got)
		}
	})

	t.Run("two timestamps 10 minutes and 1 second apart", func(t *testing.T) {
		ts := []time.Time{
			mustParse(t, "2026-08-21T09:00:00Z"),
			mustParse(t, "2026-08-21T09:10:01Z"),
		}
		if got := database.CollapseOccasions(ts); got != 2 {
			t.Errorf("CollapseOccasions(10min1s apart) = %d, want 2", got)
		}
	})

	// The doc's own trap case (design.md §1): a single sitting logged as
	// multiple rows a few minutes apart must not inflate the count.
	t.Run("doc trap case 09:1x/14:39/14:42", func(t *testing.T) {
		ts := []time.Time{
			mustParse(t, "2026-08-21T09:10:00Z"),
			mustParse(t, "2026-08-21T14:39:00Z"),
			mustParse(t, "2026-08-21T14:42:00Z"),
		}
		if got := database.CollapseOccasions(ts); got != 2 {
			t.Errorf("CollapseOccasions(trap case) = %d, want 2", got)
		}
	})

	t.Run("unsorted input is sorted internally", func(t *testing.T) {
		ts := []time.Time{
			mustParse(t, "2026-08-21T14:42:00Z"),
			mustParse(t, "2026-08-21T09:10:00Z"),
			mustParse(t, "2026-08-21T14:39:00Z"),
		}
		if got := database.CollapseOccasions(ts); got != 2 {
			t.Errorf("CollapseOccasions(unsorted trap case) = %d, want 2", got)
		}
	})
}
