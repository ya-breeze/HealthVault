package database

import (
	"testing"
	"time"
)

func mustInterval(startOffset, endOffset time.Duration, count int) stepInterval {
	base := time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC)
	return stepInterval{
		StartTime: base.Add(startOffset),
		EndTime:   base.Add(endOffset),
		Count:     count,
	}
}

func TestCollapseOverlappingSteps_NoOverlap(t *testing.T) {
	in := []stepInterval{
		mustInterval(0, time.Hour, 100),
		mustInterval(2*time.Hour, 3*time.Hour, 200),
	}
	kept, dropped := CollapseOverlappingSteps(in)
	if dropped != 0 {
		t.Errorf("dropped = %d, want 0", dropped)
	}
	if len(kept) != 2 {
		t.Fatalf("kept = %d records, want 2", len(kept))
	}
}

func TestCollapseOverlappingSteps_ExactDuplicateIntervals(t *testing.T) {
	in := []stepInterval{
		mustInterval(0, time.Hour, 100),
		mustInterval(0, time.Hour, 100),
	}
	kept, dropped := CollapseOverlappingSteps(in)
	if dropped != 1 {
		t.Errorf("dropped = %d, want 1", dropped)
	}
	if len(kept) != 1 {
		t.Fatalf("kept = %d records, want 1", len(kept))
	}
}

func TestCollapseOverlappingSteps_OneRecordContainsSeveralSmaller(t *testing.T) {
	in := []stepInterval{
		mustInterval(0, 4*time.Hour, 1000),
		mustInterval(time.Hour, 2*time.Hour, 50),
		mustInterval(2*time.Hour, 3*time.Hour, 60),
		mustInterval(3*time.Hour, 3*time.Hour+30*time.Minute, 30),
	}
	kept, dropped := CollapseOverlappingSteps(in)
	if dropped != 3 {
		t.Errorf("dropped = %d, want 3", dropped)
	}
	if len(kept) != 1 {
		t.Fatalf("kept = %d records, want 1", len(kept))
	}
	if kept[0].Count != 1000 {
		t.Errorf("kept[0].Count = %d, want 1000", kept[0].Count)
	}
}

func TestCollapseOverlappingSteps_ChainOfPartialOverlaps(t *testing.T) {
	in := []stepInterval{
		mustInterval(0, 2*time.Hour, 100),
		mustInterval(time.Hour, 3*time.Hour, 100),
		mustInterval(2*time.Hour+30*time.Minute, 4*time.Hour, 100),
	}
	kept, dropped := CollapseOverlappingSteps(in)
	if dropped != 0 {
		t.Errorf("dropped = %d, want 0 (each record extends past the watermark)", dropped)
	}
	if len(kept) != 3 {
		t.Fatalf("kept = %d records, want 3", len(kept))
	}
}

func TestCollapseOverlappingSteps_SharedStartTimeDifferentEndTime(t *testing.T) {
	in := []stepInterval{
		mustInterval(0, time.Hour, 100),
		mustInterval(0, 2*time.Hour, 200),
	}
	kept, dropped := CollapseOverlappingSteps(in)
	if dropped != 0 {
		t.Errorf("dropped = %d, want 0 (second record extends past the watermark)", dropped)
	}
	if len(kept) != 2 {
		t.Fatalf("kept = %d records, want 2", len(kept))
	}
}

func TestCollapseOverlappingSteps_AdjacentTouchingNotOverlapping(t *testing.T) {
	in := []stepInterval{
		mustInterval(0, time.Hour, 100),
		mustInterval(time.Hour, 2*time.Hour, 200),
	}
	kept, dropped := CollapseOverlappingSteps(in)
	if dropped != 0 {
		t.Errorf("dropped = %d, want 0 (touching, not overlapping)", dropped)
	}
	if len(kept) != 2 {
		t.Fatalf("kept = %d records, want 2", len(kept))
	}
}

func TestCollapseOverlappingSteps_ZeroLengthInterval(t *testing.T) {
	in := []stepInterval{
		mustInterval(time.Hour, time.Hour, 0),
	}
	kept, dropped := CollapseOverlappingSteps(in)
	if dropped != 0 {
		t.Errorf("dropped = %d, want 0", dropped)
	}
	if len(kept) != 1 {
		t.Fatalf("kept = %d records, want 1", len(kept))
	}
}

func TestCollapseOverlappingSteps_ZeroLengthIntervalCoveredByPriorRecord(t *testing.T) {
	in := []stepInterval{
		mustInterval(0, 2*time.Hour, 500),
		mustInterval(time.Hour, time.Hour, 0),
	}
	kept, dropped := CollapseOverlappingSteps(in)
	if dropped != 1 {
		t.Errorf("dropped = %d, want 1 (zero-length record's EndTime is at the watermark)", dropped)
	}
	if len(kept) != 1 {
		t.Fatalf("kept = %d records, want 1", len(kept))
	}
}

func TestCollapseOverlappingSteps_EmptyInput(t *testing.T) {
	kept, dropped := CollapseOverlappingSteps(nil)
	if dropped != 0 {
		t.Errorf("dropped = %d, want 0", dropped)
	}
	if len(kept) != 0 {
		t.Fatalf("kept = %d records, want 0", len(kept))
	}
}
