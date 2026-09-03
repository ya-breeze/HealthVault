package database

import (
	"fmt"
	"time"
)

// LocalBucketKey converts t into loc and returns the calendar-date label
// bucket_start has always carried: the local day (BucketDay) or the first
// of the local month (BucketMonth) t falls on, serialized as a UTC-midnight
// RFC3339 string — e.g. "2026-08-25T00:00:00Z" for a day bucket,
// "2026-08-01T00:00:00Z" for a month bucket. That string names a calendar
// date, not the instant local midnight actually occurred: a real
// local-midnight-with-offset label ("2026-08-25T00:00:00+03:00") was
// rejected because string ordering breaks across offset changes, the
// frontend's day-offset arithmetic (toDayOffset in
// frontend/lib/dataTypeMeta.ts) stops being exact, and a browser in a
// different zone than the stored setting would still render the wrong
// label — see the spec's How section. Keeping a UTC-midnight label makes
// that arithmetic exact by construction and lets trailingStepsAverage keep
// its existing Truncate(24 * time.Hour) map keys unchanged.
//
// A nil loc is treated as time.UTC, matching every aggregate method's
// "unconverted caller keeps today's behavior" contract.
func LocalBucketKey(t time.Time, bucket Bucket, loc *time.Location) string {
	if loc == nil {
		loc = time.UTC
	}
	lt := t.In(loc)
	if bucket == BucketMonth {
		return fmt.Sprintf("%04d-%02d-01T00:00:00Z", lt.Year(), lt.Month())
	}
	return fmt.Sprintf("%04d-%02d-%02dT00:00:00Z", lt.Year(), lt.Month(), lt.Day())
}

// aggColumnKind is how one output column of a folded aggregate accumulates
// across the 15-minute slot rows that land in the same local bucket.
type aggColumnKind int

const (
	// colSum sums a slot's non-null value across every slot folded into the
	// bucket, emitting nil for the whole bucket only if no slot ever
	// contributed a non-null value. This is what keeps SQL SUM's "ignores
	// NULL rows, NULL result only when every contributing row was NULL"
	// semantics exact once several slots fold into one bucket — a plain
	// numeric zero-default would turn "no data" into a false 0.
	colSum aggColumnKind = iota
	// colMin/colMax track the running min/max across slots' non-null
	// MIN(...)/MAX(...) values.
	colMin
	colMax
	// colAvg emits the accumulated total of sumCol divided by the
	// accumulated total of countCol — the value-weighted average across
	// slots, never the average of per-slot averages, which would silently
	// misweight a slot with one reading against a slot with a dozen.
	colAvg
)

// aggColumn describes one key of the map QueryAggregate,
// QueryAggregateBloodPressure and QueryAggregateNutrition emit per bucket,
// and how it folds the corresponding column(s) of the raw 15-minute slot
// rows SQL returns.
type aggColumn struct {
	name string
	kind aggColumnKind
	// slotCol is the per-slot column colSum/colMin/colMax reads.
	slotCol string
	// sumCol/countCol are the per-slot columns colAvg divides.
	sumCol   string
	countCol string
}

// bucketAcc accumulates every aggColumn's contribution for one bucket.
type bucketAcc struct {
	bucketStart string
	totals      map[string]float64
	hasValue    map[string]bool
}

// foldSlotsToBuckets walks 15-minute slot rows in ascending slot order (the
// order QueryAggregate's SQL ORDER BY slot already guarantees), converts
// each slot index to its start instant, resolves the local bucket it falls
// in via LocalBucketKey, and accumulates cols across every slot row that
// resolves to the same bucket key. Because slots are walked in ascending
// order and a slot's local bucket only ever advances (never repeats once
// left — see slotSeconds' doc comment for why 15 minutes never straddles a
// bucket boundary), the first-seen order of bucket keys is already
// ascending bucket order, so no separate sort is needed.
func foldSlotsToBuckets(rows []map[string]any, bucket Bucket, loc *time.Location, cols []aggColumn) []map[string]any {
	order := make([]string, 0, len(rows))
	accs := make(map[string]*bucketAcc, len(rows))

	for _, row := range rows {
		slot := toInt64(row["slot"])
		start := time.Unix(slot*slotSeconds, 0).UTC()
		key := LocalBucketKey(start, bucket, loc)

		a, ok := accs[key]
		if !ok {
			a = &bucketAcc{bucketStart: key, totals: map[string]float64{}, hasValue: map[string]bool{}}
			accs[key] = a
			order = append(order, key)
		}

		for _, c := range cols {
			switch c.kind {
			case colSum:
				if v, ok := floatValue(row[c.slotCol]); ok {
					a.totals[c.name] += v
					a.hasValue[c.name] = true
				}
			case colMin:
				if v, ok := floatValue(row[c.slotCol]); ok {
					if !a.hasValue[c.name] || v < a.totals[c.name] {
						a.totals[c.name] = v
					}
					a.hasValue[c.name] = true
				}
			case colMax:
				if v, ok := floatValue(row[c.slotCol]); ok {
					if !a.hasValue[c.name] || v > a.totals[c.name] {
						a.totals[c.name] = v
					}
					a.hasValue[c.name] = true
				}
			case colAvg:
				if v, ok := floatValue(row[c.sumCol]); ok {
					a.totals[c.name+".sum"] += v
				}
				if v, ok := floatValue(row[c.countCol]); ok {
					a.totals[c.name+".count"] += v
				}
			}
		}
	}

	out := make([]map[string]any, 0, len(order))
	for _, key := range order {
		a := accs[key]
		m := map[string]any{"bucket_start": a.bucketStart}
		for _, c := range cols {
			switch c.kind {
			case colSum, colMin, colMax:
				if a.hasValue[c.name] {
					m[c.name] = a.totals[c.name]
				} else {
					m[c.name] = nil
				}
			case colAvg:
				if count := a.totals[c.name+".count"]; count > 0 {
					m[c.name] = a.totals[c.name+".sum"] / count
				} else {
					m[c.name] = nil
				}
			}
		}
		out = append(out, m)
	}
	return out
}

// derefAny unwraps the *interface{} GORM scans a computed (expression-
// aliased) column into — its map[string]any destination can't determine a
// static type for an expression column, so it scans defensively through a
// pointer-to-interface rather than the concrete type a real column gets.
func derefAny(v any) any {
	if p, ok := v.(*interface{}); ok {
		if p == nil {
			return nil
		}
		return *p
	}
	return v
}

// toInt64 normalizes a raw driver value (int64 or float64, depending on
// SQLite column affinity) into an int64, defaulting to 0 for anything else
// — used only for the slot index, which SQL always returns as an integer.
func toInt64(v any) int64 {
	switch n := derefAny(v).(type) {
	case int64:
		return n
	case float64:
		return int64(n)
	default:
		return 0
	}
}

// floatValue normalizes a raw driver value into a float64, returning
// ok=false for SQL NULL (a nil interface after unwrapping) so callers can
// distinguish "this slot contributed nothing to this column" from "this
// slot contributed zero".
func floatValue(v any) (float64, bool) {
	switch n := derefAny(v).(type) {
	case float64:
		return n, true
	case int64:
		return float64(n), true
	case nil:
		return 0, false
	default:
		return 0, false
	}
}
