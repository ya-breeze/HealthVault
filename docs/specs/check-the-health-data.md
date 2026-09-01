# Steps: find and fix the inflated daily totals
Idea: ya-breeze/idea-forge#63

## Why

The owner walked roughly 10k steps a day for several days. HealthVault reported 12.5k for Aug 25 and just under 11k for Tue-Fri. The reported totals are consistently high by 10-25%. The idea asks which layer is wrong: the data in the database, the sync from the phone, or the GUI.

Three defects in this repository can each produce that gap, and nothing in the app today lets you tell them apart.

First, step totals are summed with no overlap handling. `SummarySteps` (`backend/pkg/database/storage_impl.go:236`) and the generic `QueryAggregate` cumulative path (`backend/pkg/database/storage_impl.go:163`, reached for `steps` through `queryBucketed` in `backend/pkg/server/api.go:187`) both run a plain `SUM(count)` over every row whose `start_time` falls in the bucket. Health Connect's `steps_record_table` holds one set of records per data origin that writes steps — the phone's own sensor, a watch, Google Fit, Samsung Health, a fitness app. Health Connect's aggregation API deduplicates those origins by priority. `readSteps` (`backend/pkg/hcimport/reader.go:94`) reads the raw table with no origin filter, so every origin's copy of the same walk is stored, and every copy is summed. Two origins that overlap for part of the day inflate that day by exactly the overlapping fraction, which is the shape of the reported gap.

Second, the two ingest paths can disagree about interval boundaries. The phone webhook (`backend/pkg/server/webhook.go`) and the Health Connect zip import (`backend/pkg/server/handler_import_hc.go`) both land in `ingest.Process`, which upserts steps on the `(user_id, start_time)` unique key (`backend/pkg/database/models.go:40-47`). Two records with the same `start_time` collapse, so re-sending the same payload is idempotent. Two records that merely overlap have different `start_time` values, so both survive and both get summed. A user who syncs from the phone and also uploads an export accumulates two overlapping representations of the same steps, and no read path notices.

Third, the dashboard's Steps card can show a value that is not today's. `extractVital` takes `series[series.length - 1]` (`frontend/lib/vitals.ts:176-179`), and the backend's bucket query is a plain `GROUP BY` with no zero-fill. When today has no step records yet, the last bucket is yesterday, and the card renders yesterday's full-day total with nothing saying so. A user who checks the dashboard in the morning sees a full day's steps they have not walked.

The inflation matters beyond the chart. `fetchDailySteps` (`backend/pkg/server/nutrition_target.go:221`) feeds the same inflated per-day sums into Activity Level, so an over-counted step history pushes the inferred tier up a step and inflates the Nutrition Target's calorie budget through the multiplier (see ADR-006).

## How

Collapse overlapping step records on read, expose a diagnostic that attributes any remaining gap to a specific layer, and stop the dashboard from presenting a stale bucket as the current value.

**Collapse on read, not on write.** The raw rows stay in the database. `webhook_payloads.raw` is already the safety net for every ingested payload, and keeping the rows means the collapse rule can change later without a re-import, and means the diagnostic can still show what was actually received. This is a decision worth recording, so it gets an ADR.

**The collapse rule.** A step record is a count over an interval, and a count cannot be split, so the rule never apportions. Sort the user's records by `start_time`, then `end_time`. Walk them keeping a watermark of the latest `end_time` covered so far. Drop a record whose interval lies entirely at or before the watermark as a duplicate of already-counted time. Keep a record that extends past the watermark in full, and advance the watermark to its `end_time`. A record that only partly overlaps is therefore kept whole, so the collapse removes less than a proportional split would. That is deliberate: under-removing leaves a number that is still explainable from the raw rows, whereas apportioning invents counts that were never recorded. The watermark is monotone across bucket boundaries because the scan is ordered by `start_time`; each kept record's count is credited to the bucket of its own `start_time`, matching the existing bucketing convention.

**Where it applies.** Steps gets a dedicated aggregate query rather than special-casing the generic one. `QueryAggregateBloodPressure` and `QueryAggregateNutrition` already establish that pattern for types the single-`valueCol` path cannot serve, and `queryBucketed` already dispatches to them by type name. A new `QueryAggregateSteps` joins them there, `SummarySteps` uses the same collapse, and `fetchDailySteps` moves onto the new query so Activity Level reads the corrected series. Both queries stream their rows in a single ordered pass and fold as they go, so memory stays flat regardless of how wide a range the caller asks for.

**The diagnostic.** `GET /api/data/steps/diagnostics` returns, per day in the range: the raw record count, the raw sum, the collapsed sum, how many records the collapse dropped, how many distinct `source_payload_id` values contributed, and the same day's total recomputed against the caller's stored `timezone` instead of UTC. Those six numbers separate the layers the idea asks about. A raw sum above the collapsed sum means duplicate intervals in the database. More than one contributing payload on a day means two syncs wrote the same window. A local-day total that differs from the UTC-day total means the chart's day boundary is not the phone's day boundary. All six agreeing while the number is still high means the phone app is sending inflated counts, which is outside this repository — and the diagnostic is what makes that conclusion defensible rather than a guess.

The endpoint is self-only and read-only. It reuses `SourcePayloadID`, which every `Steps` row already carries, so no schema change and no migration.

**Deliberately excluded.** Changing the day-bucket boundary from UTC to the user's timezone is not in this change. `bucketExpr` (`backend/pkg/database/storage_impl.go:152`) truncates on the stored UTC timestamp for every type, so moving it would change the meaning of the weight chart, the trend projection, the Logging Gap window, and the Activity Level window all at once. That is its own decision with its own record. This change measures the timezone contribution in the diagnostic so the follow-up can be judged on evidence instead of assumption.

Also excluded: attributing records to the source app that wrote them. Health Connect's export schema would have to be probed for a data-origin column that this repository has never read, and `SourcePayloadID` already answers the narrower question the idea asks — which sync wrote this row. Source-app attribution earns its own change if the diagnostic shows it is needed.

No per-record deletion or repair tool. The collapse is on read, so nothing needs deleting.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e`

## Ground rules
This spec is implemented by an automated pass running unattended. **There is no approval step and nothing is waiting for one** — do not look for a tick, a marker, or a sign-off anywhere, and do not wait for one.

Tick the boxes in this file as the work is completed; they are the record of progress, and the pipeline reads them to decide whether the change is finished.

Out of scope, deliberately: do NOT mark the pull request ready for review and do NOT merge it. Those are the pipeline's own final steps, run once the task list is complete. The operator reviews the pull request and merges it themselves; that is the only gate this work passes through, so leave it in a state worth reading.

### Task 1: Collapse overlapping step intervals
- [ ] Add `backend/pkg/database/steps_overlap.go` with a `stepInterval` struct (`StartTime`, `EndTime`, `Count`) and an exported `CollapseOverlappingSteps` that applies the watermark rule from `How`: input sorted by `StartTime` then `EndTime`, drop any record whose `EndTime` is at or before the running watermark, keep every other record whole and advance the watermark to its `EndTime`.
- [ ] Return both the kept records and the number dropped, so the diagnostic in Task 3 can report the drop count without repeating the walk.
- [ ] Document in the function's doc comment that a partial overlap keeps the whole record, and why apportioning a count is not an option.
- [ ] Add `backend/pkg/database/steps_overlap_test.go` covering: no overlap (nothing dropped); exact duplicate intervals; one record fully containing several smaller ones; a chain of partial overlaps; two records sharing a `StartTime` with different `EndTime`; adjacent records that touch but do not overlap (nothing dropped); a zero-length interval; and an empty input.
- [ ] Mark completed

### Task 2: Route every step read through the collapse
- [ ] Add `QueryAggregateSteps(bucket Bucket, userID uuid.UUID, tr TimeRange) ([]map[string]any, error)` to the `Storage` interface in `backend/pkg/database/storage.go`, next to `QueryAggregateBloodPressure` and `QueryAggregateNutrition`.
- [ ] Implement it in `backend/pkg/database/storage_impl.go`: select `start_time, end_time, count` for the user and range ordered by `start_time, end_time`, stream the rows with `Rows()`, fold them through the Task 1 rule in a single pass, and credit each kept record's count to the bucket its own `start_time` falls in. Return rows shaped exactly like the existing cumulative path — `bucket_start`, `count`, `sum` — so no caller or frontend has to change shape. Reuse `bucketExpr`'s day and month formats for `bucket_start` so the values stay byte-identical to what `QueryAggregate` produced.
- [ ] Dispatch `steps` to it from `queryBucketed` in `backend/pkg/server/api.go`, alongside the `blood_pressure` and `nutrition` cases, and update that function's doc comment to say why steps now needs its own query.
- [ ] Rewrite `SummarySteps` in `backend/pkg/database/storage_impl.go` to use the same streaming collapse instead of `COALESCE(SUM(count), 0)`.
- [ ] Point `fetchDailySteps` in `backend/pkg/server/nutrition_target.go` at `QueryAggregateSteps`, and update its doc comment, which currently states it reuses the generic `?bucket=day` aggregation.
- [ ] Add tests in `backend/pkg/database/aggregate_test.go` proving that two overlapping step records in one day sum once, that two non-overlapping records in one day sum to their total, and that a record straddling midnight lands wholly in its start day's bucket.
- [ ] Add a test in `backend/pkg/server/nutrition_target_test.go` proving that duplicated step records no longer push the inferred Activity Level tier up.
- [ ] Mark completed

### Task 3: Step diagnostics endpoint
- [ ] Add `backend/pkg/server/steps_diagnostics.go` with `StepsDiagnosticsHandler(storage database.Storage) http.HandlerFunc`, following the auth and user-resolution pattern of `DataHandler`: reject a missing claims context with 401, resolve the target user with `resolveUser`, and parse the range with `parseTimeRange`.
- [ ] Return a JSON array, one object per UTC day in the range that has at least one record: `bucket_start`, `raw_count`, `raw_sum`, `collapsed_sum`, `dropped_records`, `payload_count` (distinct `source_payload_id`), and `local_day_sum` (the collapsed total for the same calendar date resolved in the caller's stored timezone).
- [ ] Resolve the timezone with `database.ResolveTimezone` over the caller's `UserSettings` blob, which already falls back to UTC on a missing or invalid zone. When the resolved zone is UTC, `local_day_sum` equals `collapsed_sum`; state that in the handler's doc comment so a reader does not mistake the equality for a bug.
- [ ] Register the route in `backend/pkg/server/server.go` as `api.HandleFunc("/data/steps/diagnostics", StepsDiagnosticsHandler(storage)).Methods("GET")`, above the `/data/{type}` registrations.
- [ ] Add `backend/pkg/server/steps_diagnostics_test.go` covering: a day with overlapping records reports `raw_sum` above `collapsed_sum` and a nonzero `dropped_records`; a day whose records came from two payloads reports `payload_count` 2; a non-UTC stored timezone shifts `local_day_sum` away from `collapsed_sum`; an unauthenticated request returns 401; and another family member's data is never returned.
- [ ] Mark completed

### Task 4: Show the diagnostic on the steps page
- [ ] Add a `stepsDiagnostics(from, to)` call to `frontend/lib/api.ts` alongside the existing `data` call.
- [ ] In `frontend/app/data/[type]/DataTypeClient.tsx`, render the diagnostic for `steps` only, behind a disclosure control that is collapsed by default — the same hint-then-detail pattern the nutrition card's footnotes use. Show one row per day with the raw total, the counted total, the records dropped, the contributing payload count, and the local-day total.
- [ ] Show a short plain-language reading beneath the table that names the layer: duplicates in the database when `raw_sum` exceeds `collapsed_sum`, more than one sync writing the same day when `payload_count` is above 1, a day-boundary difference when `local_day_sum` differs from `collapsed_sum`, and nothing to report when all three agree.
- [ ] Add the new keys to both `frontend/lib/i18n/en.ts` and `frontend/lib/i18n/ru.ts`. Neither file may be left with a key the other lacks.
- [ ] Fail quietly: a failed diagnostics fetch leaves the chart and table untouched and hides the disclosure, matching how the page already isolates its raw and chart fetches from each other.
- [ ] Mark completed

### Task 5: Label a Vital Card value that is not today's
- [ ] Add an optional `asOf` field (the source bucket's `bucket_start`) to `VitalResult` in `frontend/lib/vitals.ts`, and set it in every `extractVital` branch from the last row's `bucket_start`.
- [ ] In `frontend/components/VitalCard.tsx`, render a short "as of <date>" line under the value when `asOf` is not today, and render nothing extra when it is. Compare calendar dates, not timestamps.
- [ ] Add the label keys to `frontend/lib/i18n/en.ts` and `frontend/lib/i18n/ru.ts`.
- [ ] Extend `frontend/lib/vitals.test.ts` with cases proving `asOf` is set from the last bucket for a cumulative type and for a point type, and that it is absent when the row carries no `bucket_start`.
- [ ] Mark completed

### Task 6: ADR, terminology, and end-to-end coverage
- [ ] Add `docs/adr/ADR-012-steps-collapse-overlapping-intervals-on-read.md` with Status: Proposed. Record the decision to collapse on read rather than deduplicate on write, the watermark rule and its refusal to apportion counts, the choice of a dedicated `QueryAggregateSteps` over special-casing the generic aggregate, and that UTC day bucketing is left unchanged and measured rather than fixed here.
- [ ] Add a **Step Interval Collapse** entry to `CONTEXT.md` under a Steps heading, naming the rule and the terms to avoid ("deduplication", which suggests exact-match rows; "step merging", which suggests counts are combined).
- [ ] Add an e2e test to `e2e/tests/data-types.spec.ts` that posts two overlapping step records to `/webhook/alice` for a date far enough in the past that no other spec's window covers it, then asserts `GET /api/data/steps?bucket=day` with an explicit `from`/`to` counts them once.
- [ ] Add an e2e test asserting `GET /api/data/steps/diagnostics` over that same range reports `raw_sum` above `collapsed_sum` and a nonzero `dropped_records`.
- [ ] Add an e2e test asserting the steps page's diagnostic disclosure is collapsed on load and reveals the table when activated.
- [ ] Run `make lint`, `make test` and `make test-e2e`, and fix every failure.
- [ ] Flip ADR-012 from Proposed to Accepted as the last commit.
- [ ] Mark completed
