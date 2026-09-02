# Bucket every chart's day in the user's timezone
Idea: ya-breeze/idea-forge#185

## Why

Every bucketed chart in the app defines a "day" as a UTC calendar day. `bucketExpr` in `backend/pkg/database/storage_impl.go` truncates each record's stored timestamp with `strftime('%Y-%m-%dT00:00:00Z', <timeCol>)`, and `QueryAggregate`, `QueryAggregateBloodPressure` and `QueryAggregateNutrition` all group and order by that expression. Everything downstream inherits it: every `GET /api/data/{type}?bucket=day` response, the dashboard sparklines built by `extractVital` in `frontend/lib/vitals.ts`, and every chart on `/data/<type>`.

For a user three hours ahead of UTC, the bar labelled "Aug 25" covers 03:00 on Aug 25 through 03:00 on Aug 26 in their own time. It is not the Aug 25 their phone shows them, so any per-day comparison against another app disagrees by whatever activity falls in the shifted window.

The app already stores the right answer. Each user has a `timezone` setting in their settings blob, and the food-logging side already honours it: `database.ResolveTimezone` in `backend/pkg/database/food_completeness.go` turns it into a `*time.Location`, falling back to UTC when the key is missing, empty, or names a zone `time.LoadLocation` rejects. `SummaryTodayHandler` and `foodHandlers.callerTimezone` both use it. The general chart path never picked it up, so the two halves of the app disagree about what a day is. `CONTEXT.md`'s "Logged Day" entry documents that disagreement as a known quirk instead of fixing it.

The blast radius is the whole app, which is why this was left out of the step-count work. Four surfaces move when the boundary moves:

- The `/data/<type>` charts for every registered type, at Week, Month and Year zoom, plus the all-time weight fetches.
- The weight chart's smoothed trend line and its dashed goal-weight projection, both fed by `projectionBucketRows` (a dedicated `?bucket=day` fetch) in `frontend/app/data/[type]/DataTypeClient.tsx`.
- The dashboard vitals sparklines.
- The inferred Activity Level's 28-day step window: `fetchDailySteps` in `backend/pkg/server/nutrition_target.go` reads `?bucket=day` step sums, and `trailingStepsAverage` in `backend/pkg/server/activity_level.go` walks UTC calendar days over them.

One surface named in the idea turns out not to be affected. The Logging Gap card's 28-day window already resolves days in the stored zone: `frontend/components/LoggingGapCard.tsx` fetches **raw** weigh-ins, not buckets, and keys them with `loggedDayKey` from `frontend/lib/loggedDay.ts`. It needs no change here.

There is a second, quieter defect this change has to fix first, or the rest of it is a no-op in production. The backend runtime image is `debian:bookworm-slim` with only `ca-certificates` and `sqlite3` installed (`backend/Dockerfile`), and nothing in the backend imports `time/tzdata`. There is no zone database in the container, so `time.LoadLocation` fails for every IANA name and `ResolveTimezone` silently falls back to UTC. That means the existing food-day-completeness timezone support is already inert in the deployed stack, and any local-day bucketing built on top of it would be inert too.

The idea originally proposed shipping a diagnostic first, to quantify the discrepancy before fixing it. The owner has dropped that gate: the diagnostic never shipped, and building it first turns one change into two for no added certainty.

## How

The day and month buckets resolve in the user's stored timezone, falling back to UTC when the setting is missing or names a zone the server cannot load. The mechanism the owner confirmed is: **pre-aggregate in SQL at 15-minute granularity, then regroup those slots into local days and months in Go.**

SQL keeps doing the heavy reduction. `bucketExpr` is replaced by a `slotExpr(timeCol)` that groups on `CAST(strftime('%s', <timeCol>) AS INTEGER) / 900` — the 15-minute slot index since the Unix epoch, emitted as an integer `slot` column. Go then folds consecutive slots into local buckets by converting each slot's start instant with `t.In(loc)` and reading its calendar date.

**Exactness across DST is an acceptance criterion, not an implementation note.** A regrouping that adds a single fixed UTC offset is wrong on the two transition days a year, and "close enough" on those days is the failure mode worth testing. Two properties make the slot approach exact rather than approximate:

- 15 minutes is the coarsest slot that never straddles a local-day or local-month boundary. Every offset in the modern IANA database is a whole number of 15-minute steps — `Asia/Kathmandu` at +05:45, `Australia/Eucla` at +08:45 and `Pacific/Chatham` at +12:45 are the finest — and every DST transition lands on a slot edge.
- Each slot's local date is derived per slot through `time.Location`, so a 23-hour spring-forward day and a 25-hour fall-back day are each folded into exactly one bucket, and every record lands in exactly one bucket.

The wire format does not change. `bucket_start` stays a string of the form `YYYY-MM-DDT00:00:00Z`, but it now names the **local** calendar date rather than a UTC one. Read it as a calendar-date label serialized at UTC midnight, not as the instant local midnight actually happened. The alternative — emitting real local midnight with its offset, `2026-08-25T00:00:00+03:00` — was rejected: string ordering breaks across offset changes, `toDayOffset`'s `Math.floor(ms / MS_PER_DAY)` arithmetic in `frontend/lib/dataTypeMeta.ts` stops being exact, and a browser in a different zone than the stored setting still renders the wrong label. Keeping a UTC-midnight label makes the existing day-offset arithmetic exact by construction and lets `trailingStepsAverage` keep its current `Truncate(24 * time.Hour)` map keys unchanged.

That label choice does force one client-side correction, and it fixes an existing off-by-one. `bucketLabel` in `DataTypeClient.tsx` currently formats `bucket_start` with `toLocaleDateString(undefined, ...)` in the **browser's** zone, so a bucket at `2026-08-25T00:00:00Z` already renders as "Aug 24" for a viewer behind UTC. Every render of a `bucket_start` or a day-offset-derived date must format with `timeZone: 'UTC'`, the same trick `loggedDayLabel` already uses.

Aggregate combination has to be exact too, which changes what SQL selects:

- Cumulative types combine by summing slot sums and slot counts.
- Point types cannot combine slot averages by averaging them. SQL returns `SUM(valueCol)`, `COUNT(valueCol)`, `MIN` and `MAX` per slot; Go emits `avg` as total sum over total non-null count, and `min`/`max` as the min/max across slots. The response keeps exactly today's keys — `bucket_start`, `count`, `avg`, `min`, `max` — with the helper columns folded away.
- Nutrition's seven `SUM(...)` columns must keep today's NULL semantics: `SUM` ignores NULL rows but returns NULL when every contributing row is NULL. Go tracks, per output column, whether any slot contributed a non-null value, and emits `nil` when none did. `TestQueryAggregateNutrition_SevenColumnsIgnoreNulls` must still pass unchanged.

The timezone reaches the storage layer as an explicit `loc *time.Location` parameter on the three aggregate methods in `database.Storage`, rather than by having the database package read settings itself. `DataHandler` resolves it from the **target** user — the one `resolveUser` returned, which may be a family member named by `?user=` — using `readUserSettingsJSON` plus `database.ResolveTimezone`. A family member's chart is their data, so it uses their zone, matching how per-user settings already work for food-day-completeness. A missing settings row is the ordinary case and yields UTC; any other read error is a 500, matching `SummaryTodayHandler`.

The Activity Level window follows: `resolveActivityTier` and `fetchDailySteps` take the location, the 28-day window is 28 **local** calendar days ending the user's local yesterday, and the SQL range's lower bound becomes the UTC instant of that window's first local midnight.

The dashboard sparkline window needs one small correction. `frontend/app/page.tsx` currently asks for the 7 days before the current UTC midnight. Under local buckets that clips the earliest local day for anyone ahead of UTC, showing an artificially low first point. Over-fetch by one day and drop any bucket whose date string sorts before the local date 6 days ago, computed with `loggedDayKey` against the already-loaded `timezone` setting.

Cost. A 15-minute pre-aggregation returns at most one row per populated slot instead of one per populated day, bounded by 96 rows per day and by the record count itself. The worst realistic case is a Year zoom over dense step data, roughly 35,000 rows for a full year. At this project's single-user scale that is an acceptable price for exactness, and the `(user_id, timeCol)` filter and its indexes are unchanged.

The change carries an ADR, `docs/adr/ADR-012-local-day-chart-buckets.md`, because it is a cross-cutting shift in what a stored aggregate means and it introduces the embedded zone database as a build-time dependency.

Deliberately excluded:

- **The diagnostic.** Dropped on the owner's instruction; see Why.
- **The Logging Gap card.** Already local-day correct through raw records and `loggedDayKey`.
- **Unbucketed `GET /api/data/{type}`.** It returns raw records with real UTC instants, which are already unambiguous.
- **Any stored or materialized per-day rollup.** Buckets stay computed per request, so a timezone change takes effect on the next read with no migration and no backfill.
- **A `?tz=` query override.** The stored setting is the single source, exactly as it is for Logged Day.
- **A `week` bucket.** Week zoom still uses day buckets over a shorter range.
- **Pre-1970 timestamps.** Integer division of a negative epoch second truncates toward zero and would mis-slot such a row. Health data is post-1970; note the limit in the code comment rather than handling it.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e`

## Ground rules
This spec is implemented by an automated pass running unattended. **There is no approval step and nothing is waiting for one** — do not look for a tick, a marker, or a sign-off anywhere, and do not wait for one.

Tick the boxes in this file as the work is completed; they are the record of progress, and the pipeline reads them to decide whether the change is finished.

Out of scope, deliberately: do NOT mark the pull request ready for review and do NOT merge it. Those are the pipeline's own final steps, run once the task list is complete. The operator reviews the pull request and merges it themselves; that is the only gate this work passes through, so leave it in a state worth reading.

### Task 1: Put a zone database in the deployed binary

- [x] Add a blank import of `time/tzdata` to `backend/cmd/main.go`, with a comment explaining that the runtime image (`debian:bookworm-slim`, `backend/Dockerfile`) installs no `tzdata` package, so without the embedded database every `time.LoadLocation` call fails and `database.ResolveTimezone` silently degrades every user to UTC.
- [x] Add a test in `backend/pkg/database/food_completeness_test.go` asserting that `ResolveTimezone` returns a non-UTC location for a real IANA name such as `Asia/Tokyo`, so a future build that drops the embedded database fails the suite instead of silently reverting the feature.
- [x] Mark completed

### Task 2: Pre-aggregate at 15-minute slots in SQL

- [x] In `backend/pkg/database/storage_impl.go`, replace `bucketExpr` with `slotExpr(timeCol string) string` returning `CAST(strftime('%s', <timeCol>) AS INTEGER) / 900`, and a `slotSeconds = 900` constant.
- [x] Document on `slotSeconds` why 15 minutes is the right width: every modern IANA offset is a whole number of 15-minute steps (+05:45, +08:45, +12:45 are the finest) and every DST transition lands on a slot edge, so no slot ever straddles a local-day or local-month boundary. Note the pre-1970 truncation limit.
- [x] Change `QueryAggregate` to select `slot`, `COUNT(*) AS count` and `SUM(<valueCol>) AS sum` for the cumulative family, and `slot`, `COUNT(*) AS count`, `COUNT(<valueCol>) AS value_count`, `SUM(<valueCol>) AS sum`, `MIN(<valueCol>) AS min`, `MAX(<valueCol>) AS max` for the point family, grouped and ordered by the slot expression.
- [x] Change `QueryAggregateBloodPressure` to select per-slot `COUNT(*)`, plus `COUNT`/`SUM`/`MIN`/`MAX` for `systolic` and `diastolic`.
- [x] Change `QueryAggregateNutrition` to select per-slot `COUNT(*)` plus the same seven `SUM(...)` columns it selects today.
- [x] Leave the `WHERE user_id = ? AND <timeCol> >= ? AND <timeCol> <= ?` clause and the existing indexes untouched.
- [x] Mark completed

### Task 3: Regroup slots into local buckets in Go

- [x] Add `backend/pkg/database/bucket_regroup.go` with `LocalBucketKey(t time.Time, bucket Bucket, loc *time.Location) string`: convert with `t.In(loc)`, then format the local calendar date as `YYYY-MM-DDT00:00:00Z` for `BucketDay` and the first of the local month as `YYYY-MM-01T00:00:00Z` for `BucketMonth`.
- [x] Document on `LocalBucketKey` that `bucket_start` is a calendar-date label serialized at UTC midnight, not the instant local midnight occurred, and why that representation was chosen over a real local-midnight offset (see the spec's `How`).
- [x] Add the fold that walks slot rows in ascending slot order, converts each slot index to its start instant (`time.Unix(slot*slotSeconds, 0).UTC()`), resolves its bucket key, and accumulates sums, counts, minima and maxima per bucket. Emit `[]map[string]any` in ascending bucket order.
- [x] Make the fold generic enough to serve all three query methods — a per-output-column description of which slot column it folds and how — rather than writing the accumulation three times.
- [x] Compute the point family's `avg` as total sum over total non-null count, and drop the `value_count` helper column from the emitted maps, so the response keys stay exactly `bucket_start`, `count`, `avg`, `min`, `max`.
- [x] Preserve nutrition's NULL semantics: track per output column whether any slot contributed a non-null value, and emit `nil` for a column where none did.
- [x] Normalize the raw driver values the same way `derefAny`/`toFloat64` already do, since GORM scans computed columns through `*interface{}`.
- [x] Mark completed

### Task 4: Thread the location through the storage interface

- [x] Add a `loc *time.Location` parameter to `QueryAggregate`, `QueryAggregateBloodPressure` and `QueryAggregateNutrition` in `backend/pkg/database/storage.go`, and update their doc comments to say the bucket resolves in `loc`.
- [x] Update the `Bucket` type's doc comment: day and month buckets are local calendar days and months, and `bucket_start` labels a local calendar date.
- [x] Update the implementations in `storage_impl.go` and the `mockStorage` in `backend/pkg/server/delete_handler_test.go`.
- [x] Treat a nil `loc` as `time.UTC` rather than panicking, so an unconverted caller degrades to today's behaviour.
- [x] Mark completed

### Task 5: Resolve the viewer's zone in the data handler

- [x] Add a helper in `backend/pkg/server/api.go` that resolves a user's `*time.Location` from `readUserSettingsJSON` plus `database.ResolveTimezone`, with a comment stating that the zone comes from the **target** user (the one `resolveUser` returned), not the caller, because a family member's chart is their data.
- [x] Add `loc` to `queryBucketed` and pass it to the three storage calls. Leave `errInvalidBucket` and the `food_meal` rejection unchanged.
- [x] Call the helper from `DataHandler` on the bucketed branch only, and return a 500 on a genuine settings read error while treating a missing settings row as UTC.
- [x] Mark completed

### Task 6: Move the Activity Level window to local days

- [x] Add a `loc *time.Location` parameter to `fetchDailySteps` and `resolveActivityTier` in `backend/pkg/server/nutrition_target.go`, and thread it from `computeUserNutritionTarget` and `computeNutritionTargetForProfile`.
- [x] Have `computeUserNutritionTarget` read the zone itself, and have `SummaryTodayHandler` pass the `loc` it already resolved so one settings read still serves the whole response.
- [x] In `fetchDailySteps`, derive the user's local calendar today from `now.In(loc)`, express it as that date at UTC midnight, and set the range's lower bound to the UTC instant of the first local midnight in the 28-day window. Update the doc comment.
- [x] Update `dailySteps`' comment in `backend/pkg/server/activity_level.go`: `Date` is now a local calendar day carried as a UTC-midnight label, which is exactly why `trailingStepsAverage`'s date arithmetic keeps working unchanged across DST.
- [x] Mark completed

### Task 7: Label and window the frontend against local dates

- [x] Format `bucket_start` with `timeZone: 'UTC'` in `bucketLabel` (`frontend/app/data/[type]/DataTypeClient.tsx`), with a comment that `bucket_start` is a local calendar date carried at UTC midnight, so browser-local formatting would shift the label by a day.
- [x] Format the projection's `crossingDate` with `timeZone: 'UTC'` for the same reason, since it is derived from a day offset.
- [x] In `frontend/app/page.tsx`, over-fetch the sparkline range by one extra day and drop any returned bucket whose date sorts before the local date 6 days ago, computed with `loggedDayKey` against the `timezone` already held in state. Explain in a comment that a UTC-midnight lower bound clips the earliest local day for a viewer ahead of UTC.
- [x] Confirm `frontend/lib/vitals.ts` needs no change — `extractVital` reads only value columns, never `bucket_start` — and leave it alone.
- [x] Confirm `frontend/components/LoggingGapCard.tsx` needs no change — it keys raw weigh-ins with `loggedDayKey` and never reads `bucket_start` — and leave it alone.
- [x] Mark completed

### Task 8: Test the boundary, including both DST days

- [x] In `backend/pkg/database/aggregate_test.go`, keep the existing UTC-location cases passing unchanged, then add a case where records either side of local midnight in a zone ahead of UTC (for example `Asia/Tokyo`) land in different local day buckets than they do under UTC.
- [x] Add a case for a zone behind UTC (for example `America/Los_Angeles`) showing the boundary moves the other way.
- [x] Add a case for a 45-minute offset zone (for example `Asia/Kathmandu`), proving 15-minute slots resolve a non-hour boundary exactly.
- [x] Add spring-forward and fall-back cases in a DST zone: seed records through the 23-hour and the 25-hour local day, and assert every record lands in exactly one bucket and each bucket's total equals the total from grouping the same records by `LocalBucketKey` record by record. This is the exactness criterion; a fixed-offset regrouping must fail it.
- [x] Add a month-bucket case in a non-UTC zone where a record near the month boundary moves months.
- [x] Add point-family and blood-pressure cases proving `avg` is the value-weighted average across slots, not the average of slot averages, and that `min`/`max` survive the fold.
- [x] Add a case proving an unknown or empty timezone setting produces exactly today's UTC buckets.
- [x] Add or extend a test in `backend/pkg/server` covering `trailingStepsAverage` over a window that crosses a DST transition in the user's zone.
- [x] Add an e2e spec at `e2e/tests/chart-day-boundary.spec.ts`, following `e2e/tests/completeness.spec.ts`'s pattern: default `BASE_URL` to the WIP stack because it mutates the `timezone` setting, capture the account's original settings first, and restore them in a `finally`.
- [x] Have that spec POST one `weight` record at a fixed past instant late in the UTC day, read `GET /api/data/weight?bucket=day` under `UTC` and again under a zone ahead of UTC, assert the record's `bucket_start` differs by one day between the two, and delete the record in the `finally`.
- [x] Mark completed

### Task 9: Record the decision

- [ ] Amend `CONTEXT.md`'s **Logged Day** entry: drop the clause claiming the general `/api/data/{type}` charts use a different, UTC bucketing, and drop the `_Avoid_` note that reserves "Day" because of it.
- [ ] Add a **Bucket Start** term to `CONTEXT.md` defining it as the local calendar date (or first of the local month) a bucket covers, resolved in the user's stored `timezone` and serialized at UTC midnight, with `_Avoid_`: bucket date, bucket timestamp.
- [ ] Write `docs/adr/ADR-012-local-day-chart-buckets.md` with `Status: Proposed`, covering the decision to resolve chart buckets in the user's stored zone, the 15-minute SQL pre-aggregation plus Go regrouping mechanism and why exactness across DST ruled out a fixed-offset shift, the choice to keep `bucket_start` as a UTC-midnight calendar-date label, and the embedded `time/tzdata` dependency.
- [ ] As the last change before finishing, flip ADR-012 from `Proposed` to `Accepted`.
- [ ] Mark completed
