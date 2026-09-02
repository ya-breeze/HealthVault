# ADR-012: Collapse Overlapping Step Intervals on Read, Not on Write

## Status
Proposed

## Context and Problem Statement

Idea #63 reported daily step totals inflated 10-25% against a known ~10k-step day.
Health Connect's `steps_record_table` holds one set of step records per data origin that writes
steps (the phone's own sensor, a watch, Google Fit, Samsung Health, a fitness app), and Health
Connect's own aggregation API deduplicates those origins by priority. `readSteps`
(`backend/pkg/hcimport/reader.go`) reads the raw table with no origin filter, so every origin's
copy of a walk is stored, and both `SummarySteps` and the generic `QueryAggregate` cumulative path
(`backend/pkg/database/storage_impl.go`) summed every row with a plain `SUM(count)`. Two origins
whose intervals overlap for part of a day inflate that day by exactly the overlapping fraction —
the shape of the reported gap. Separately, the phone webhook and the Health Connect zip import
both land in `ingest.Process`, which upserts on `(user_id, start_time)`: two records with
identical `start_time` collapse, but two that merely overlap have different `start_time` values
and both survive. How should the app remove this double-count, and where?

## Decision Drivers

- The raw rows are the only record of what was actually received; `webhook_payloads.raw` already
  exists as the ingestion-time safety net, and the diagnostic endpoint this idea also asks for
  (`GET /api/data/steps/diagnostics`) needs the raw rows intact to show what was actually stored.
- A step record is a count over an interval; a count cannot be split without inventing a number
  that was never recorded, so any correction has to decide between keeping a record whole or
  dropping it, never trimming it to a partial count.
- Steps already has one generic path (`QueryAggregate`) and two special-cased ones
  (`QueryAggregateBloodPressure`, `QueryAggregateNutrition`) for types the single-`valueCol` shape
  can't express — precedent existed for a fourth, steps-specific query rather than growing
  `QueryAggregate` a branch it wasn't designed for.
- `fetchDailySteps` (`backend/pkg/server/nutrition_target.go`) feeds the same per-day step sums
  into Activity Level (ADR-006), so an uncorrected read path leaves the Nutrition Target's calorie
  budget inflated even after the steps chart itself is fixed.
- The day-bucket boundary is UTC for every aggregated type (`bucketExpr`); changing it to the
  caller's local day would change the meaning of the weight chart, the trend projection, the
  Logging Gap window, and the Activity Level window all at once — a separate decision with its own
  blast radius, not something to fold into a steps-only fix.

## Considered Options

- **Deduplicate on write** (reject or merge overlapping records at ingest time) — rejected. It
  would discard the raw rows the diagnostic endpoint needs to show *what was received*, and it
  would have to guess a rule before it's clear the rule is right; a wrong on-write rule needs a
  re-import to correct, while a wrong on-read rule is fixed by changing a query.
- **Apportion overlapping counts proportionally to their non-overlapping share** — rejected. A step
  count is atomic; splitting it invents a number no origin ever reported. Under-removing (keeping
  a partially-overlapping record whole) was chosen instead, specifically because every kept count
  still traces back to a real row.
- **Collapse on read via a watermark walk, credited to each kept record's own start-time bucket** —
  chosen. Sort by `(start_time, end_time)`, walk once keeping a watermark of the latest `end_time`
  counted so far, drop any record whose `end_time` doesn't extend past it, keep every other record
  whole and advance the watermark. The watermark is monotone across bucket boundaries because the
  scan is ordered by `start_time`, so it can run as a single streaming pass over an arbitrarily
  wide range without buffering more than one bucket's accumulator at a time.
- **Special-case steps inside the generic `QueryAggregate`** — rejected in favor of a dedicated
  `QueryAggregateSteps`, matching the existing `QueryAggregateBloodPressure`/
  `QueryAggregateNutrition` precedent for types the single-`valueCol` shape can't serve.

## Decision Outcome

Chosen: **collapse overlapping step intervals on read, via a shared watermark rule, applied
everywhere steps are summed.**

- `CollapseOverlappingSteps` (`backend/pkg/database/steps_overlap.go`) is the rule itself, over a
  pre-sorted, pre-loaded slice: drop a record whose `EndTime` doesn't extend past the running
  watermark, keep every other record whole and advance the watermark to its `EndTime`.
- `QueryAggregateSteps` and the rewritten `SummarySteps` apply the identical rule while streaming
  rows off a `*sql.Rows` cursor, so memory stays flat regardless of the queried range's width — a
  small `stepWatermark` type factors the keep/drop decision itself out of `CollapseOverlappingSteps`
  so the slice-based and cursor-based callers can't drift into two different rules.
- `fetchDailySteps` moved onto `QueryAggregateSteps`, so Activity Level's trailing-average
  inference reads the corrected series, not the raw one.
- `GET /api/data/steps/diagnostics` re-derives the same watermark decision over the raw rows
  directly (not via `CollapseOverlappingSteps`, whose parameter type is unexported) to report, per
  UTC day: the raw count/sum, the collapsed sum, how many records the collapse dropped, how many
  distinct `source_payload_id`s contributed, and the same day's collapsed total recomputed against
  the caller's stored timezone instead of UTC — separating "duplicate rows in the database" from
  "two syncs wrote the same day" from "the chart's UTC day boundary isn't the caller's day
  boundary" as three independently falsifiable signals, rather than asking the user to guess which
  applies.
- UTC day bucketing is left unchanged everywhere. The diagnostic's `local_day_sum` column measures
  the timezone boundary's contribution so a future change to `bucketExpr` can be judged on
  evidence; it does not itself change what any chart renders.

### Consequences

- Raw step rows are never deleted or rewritten by this change — every number the collapse produces
  is still reconstructable from what's in the database, and a wrong collapse rule is a query
  change, not a re-import.
- `SummarySteps` and `QueryAggregateSteps` are O(rows in range) time and O(1) extra memory beyond
  the current bucket's accumulator, same asymptotic shape as the `SUM(count)` they replaced, just
  computed in Go instead of SQL.
- A record that partially overlaps another is kept whole rather than trimmed, so the corrected
  total can still slightly over-count a day with genuinely staggered, partially-overlapping
  sources. This is accepted as the safer failure mode: it never invents a count, only under-removes
  relative to a proportional split.
- The UTC/local day-boundary mismatch the diagnostic surfaces is not fixed by this change — see
  `bucketExpr`'s own note on why changing it is out of scope here. A user whose reported gap turns
  out to be purely a day-boundary artifact still sees an inflated-looking UTC bucket next to an
  accurate `local_day_sum`, until that follow-up decision is made.
