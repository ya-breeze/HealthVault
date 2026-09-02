package database

import "time"

// stepInterval is one step record's interval and count, the shape
// CollapseOverlappingSteps walks. It carries no identity (no ID,
// UserID, SourcePayloadID) because the collapse rule never needs one —
// only StartTime, EndTime and Count decide what to keep or drop.
type stepInterval struct {
	StartTime time.Time
	EndTime   time.Time
	Count     int
}

// CollapseOverlappingSteps applies the watermark rule from the
// check-the-health-data spec's "How" section to a caller-sorted run of step
// intervals: input must already be sorted by StartTime, then EndTime.
//
// It walks the intervals keeping a watermark of the latest EndTime covered
// so far. A record whose EndTime is at or before the watermark is dropped
// as a duplicate of already-counted time. Every other record is kept whole
// and the watermark advances to its EndTime.
//
// A partially-overlapping record is kept whole rather than trimmed to the
// uncovered remainder: a step record is a count over its interval, and a
// count cannot be split without inventing a number that was never
// recorded. Keeping the whole record under-removes relative to a
// proportional split, but every kept count still traces back to a raw row,
// which a synthesized partial count would not.
//
// Returns the kept intervals (in the same order) and the number dropped.
func CollapseOverlappingSteps(intervals []stepInterval) (kept []stepInterval, dropped int) {
	kept = make([]stepInterval, 0, len(intervals))
	var watermark time.Time // zero value predates every real record, so the first record is always kept
	for _, iv := range intervals {
		if !iv.EndTime.After(watermark) {
			dropped++
			continue
		}
		kept = append(kept, iv)
		watermark = iv.EndTime
	}
	return kept, dropped
}
