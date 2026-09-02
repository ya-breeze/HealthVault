# ADR-012: Chart Buckets Resolve in the User's Timezone, via 15-Minute SQL Pre-Aggregation

## Status
Proposed

## Context and Problem Statement

Every bucketed chart (`GET /api/data/{type}?bucket=day|month`) defined a "day" as a UTC calendar day: `bucketExpr` truncated each record's timestamp with `strftime('%Y-%m-%dT00:00:00Z', <timeCol>)` directly in SQL. For a user whose stored `timezone` setting differs from UTC, the bar labelled "Aug 25" covered a window shifted by their UTC offset from the "Aug 25" their phone shows them — every per-day comparison against another app, or against the user's own memory of their day, disagreed by whatever activity fell in the shifted window. Food logging's Logged Day already resolved this correctly (`database.ResolveTimezone` + `LocalDate`), so the app had two different, disagreeing definitions of "day" depending which feature you looked at.

Fixing the boundary is not enough on its own: the backend's runtime image (`debian:bookworm-slim`) installs no `tzdata` package, and nothing in the backend imported `time/tzdata`. `time.LoadLocation` therefore failed for every IANA zone name in production, and `ResolveTimezone` silently fell back to UTC — meaning the existing Logged Day timezone support was already inert in the deployed stack, and any local-day bucketing built on it would have been too.

**Exactness across DST is a requirement, not a nice-to-have.** A regrouping that merely applies a fixed UTC offset is correct most of the year and wrong on the two DST transition days — a 23-hour spring-forward day or a 25-hour fall-back day would either lose a bucket's worth of records or split one calendar day into two. "Close enough except on two days a year" is exactly the kind of bug that's invisible in every demo and wrong for real users on those two real days.

## Decision Drivers

- Chart buckets must agree with Logged Day about what "day" means, in the same user-configurable zone
- The fix must actually take effect in the deployed container, not just in local development where the host OS supplies zone data
- Correctness across DST transitions and non-hour UTC offsets (`+05:45`, `+08:45`, `+12:45` exist in the IANA database) must be exact, not approximate
- SQL should keep doing the heavy reduction — pulling every raw record to the application layer to bucket in Go doesn't scale even at this project's single-user scale, and throws away SQLite's indexes
- The wire format (`bucket_start`) is read by existing frontend day-offset arithmetic (`toDayOffset` in `dataTypeMeta.ts`) that must stay exact

## Considered Options

- **Bucket entirely in SQL with a timezone-aware `strftime`** — SQLite's `strftime`/`datetime` have no IANA timezone support, only fixed numeric offsets, so this can't itself be DST-correct without the caller supplying the right offset per row, which is circular.
- **Pull raw records to Go and bucket there** — trivially exact (Go's `time.Location` is DST-correct), but discards the whole point of aggregating in the database: a Year-zoom query over dense step data would pull tens of thousands of raw rows instead of at most 96-per-day pre-aggregated slots.
- **Pre-aggregate in SQL at a fixed sub-day granularity, then fold into local buckets in Go** — SQL still does the heavy reduction (one row per populated 15-minute slot, not per raw record), and the DST-correct part (converting a slot's start instant into a local calendar day) happens in Go using the real embedded zone database. 15 minutes is the coarsest width that provably never straddles a local-day or local-month boundary: every offset in the modern IANA database is a whole number of 15-minute steps, and every DST transition lands on a slot edge.
- **Emit real local-midnight-with-offset labels (`2026-08-25T00:00:00+03:00`)** — considered for `bucket_start` itself. Rejected: string ordering breaks across an offset change (e.g. a DST transition mid-range), the frontend's `toDayOffset` (`Math.floor(ms / MS_PER_DAY)`) stops being exact once the label isn't UTC-anchored, and a browser in a different zone than the account's stored setting would still render the wrong label from a raw `Date` parse.

## Decision Outcome

Chosen: **pre-aggregate at 15-minute slots in SQL, then fold slots into local calendar buckets in Go**, plus embedding `time/tzdata` in the binary so the deployed container has real zone data.

- `slotExpr(timeCol)` replaces `bucketExpr`, grouping SQL rows by `CAST(strftime('%s', <timeCol>) AS INTEGER) / 900` — the 15-minute slot index since the epoch. `QueryAggregate`/`QueryAggregateBloodPressure`/`QueryAggregateNutrition` select per-slot `COUNT`/`SUM`/`MIN`/`MAX` (and, for the point family and blood pressure, a per-slot non-null `COUNT` alongside `SUM`, since a bucket's average must be the value-weighted total-sum-over-total-count across every slot it folds, never the average of each slot's own average).
- `database.LocalBucketKey(t, bucket, loc)` (`bucket_regroup.go`) converts a slot's start instant with `t.In(loc)` and derives its local calendar day or month. A generic fold (`foldSlotsToBuckets`), described per output column rather than duplicated three times, walks slot rows in ascending order and accumulates each bucket's sums/counts/minima/maxima — preserving nutrition's existing "SUM ignores NULL rows, NULL bucket only when every contributing row was NULL" semantics by tracking, per column, whether any slot contributed a non-null value.
- The timezone reaches storage as an explicit `loc *time.Location` parameter on the three aggregate methods, resolved by the server from the **target** user's stored settings (`resolveViewerTimezone`, `readUserSettingsJSON` + `database.ResolveTimezone`) — not the caller's, so a family member's chart uses their own zone. A nil `loc` is treated as `time.UTC` rather than panicking.
- `bucket_start` keeps its existing wire shape, `YYYY-MM-DDT00:00:00Z` — still a calendar-date label serialized at UTC midnight, now naming the **local** date rather than a UTC one. The frontend's `bucketLabel`/`crossingDate` formatting was updated to read it back with `timeZone: 'UTC'`, fixing a pre-existing off-by-one where a viewer behind UTC already saw the wrong day.
- `backend/cmd/main.go` blank-imports `time/tzdata`, so `time.LoadLocation` and `database.ResolveTimezone` work in the deployed container without an OS-level `tzdata` package.

### Consequences

- Every bucketed chart, the dashboard sparklines, and the Activity Level 28-day step window now agree with Logged Day about what "day" means, in the same per-user zone.
- A 15-minute pre-aggregation returns at most 96 rows per populated day instead of one row per populated day (the old bucketed-in-SQL shape) — a Year-zoom query over dense step data is now bounded around 35,000 rows worst case, an acceptable cost at this project's single-user scale, still far short of pulling every raw record.
- The Logging Gap card needed no change: it already reads raw records and keys them with `loggedDayKey`, not `bucket_start`.
- No stored or materialized per-day rollup exists; buckets are computed per request, so a timezone change takes effect on the next read with no migration or backfill — the same property Logged Day already had.
- Pre-1970 timestamps are out of scope: integer division of a negative epoch second truncates toward zero rather than flooring, which would mis-slot such a row. Health data is post-1970, so this is accepted rather than handled.
