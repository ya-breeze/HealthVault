# Add the authenticated dashboard read-model contract
Idea: ya-breeze/idea-forge#570

## Why

`Dashboard` in `frontend/app/page.tsx` currently composes its core state from one settings request, the complete Presence request, eight `bucket=day` aggregate requests for the DataType-backed entries in `PRIMARY_METRICS`, and the Food Meal needs-attention count. The prerequisite work in `docs/specs/create-a-follow-up-dashboard-read-path-o.md` removed the worst database amplification by changing `dataTypesPresence` in `backend/pkg/server/api.go` from 27 serial counts to one indexed `UNION ALL` query, but the browser still coordinates eleven independent requests and their different failure policies.

The completed profile in `docs/investigations/idea-478-dashboard-read-path.md` measured 19 SQL statements for that post-Presence-optimization source sequence: one settings read, one Presence statement, eight repeated timezone/settings reads, eight aggregates, and one needs-attention count. A dashboard read model can reduce this to one HTTP request and one settings read while retaining the existing optimized Presence helper, aggregate implementations, and Food Meal status definition. The representative local benchmark was roughly 1.57 ms for the current sequence, so it supports request consolidation and removal of redundant settings reads, not caching or precomputation.

This first split establishes the backend and TypeScript contract before changing the dashboard lifecycle. That gives the partial-result and authorization behavior a reviewable boundary of its own; the later UI cutover can then focus on preserving the dashboard's retry, cancellation, ordering, visibility, and Playwright-observed request behavior.

## How

Add a self-only `GET /api/dashboard` route in `backend/pkg/server/server.go`, implemented by a new `DashboardHandler` in `backend/pkg/server/dashboard.go`. Authentication remains an overall request boundary: missing claims return 401 and no read-model body. The handler must always use `claims.UserID`, must not call `resolveUser`, and must not allow a `user` query parameter to select a family member. Once authenticated, source failures return HTTP 200 with explicit per-section status rather than turning unrelated data into an overall 500.

Define a stable JSON envelope with `settings`, `presence`, `aggregates`, and `needs_attention` sections. Settings is `{status: "ok", value: <the opaque settings object>}` or `{status: "error"}`; a missing settings row remains the ordinary successful `{}` value, and arbitrary unknown keys must round-trip. Presence is `{status: "ok", value: <the complete map>}` or `{status: "error"}`. `aggregates` always contains exactly `steps`, `heart_rate`, `sleep`, `heart_rate_variability`, `distance`, `weight`, `blood_pressure`, and `oxygen_saturation`, with each entry independently shaped as `{status: "ok", rows: [...]}` or `{status: "error"}`. `needs_attention` is `{status: "ok", count: <number>}` or `{status: "error"}`. Successful empty aggregate results serialize as `[]`. Do not expose database error text in the response.

Capture `now := time.Now().UTC()` exactly once at the handler boundary and pass it into an internal read-model builder so tests can supply a fixed instant. Read settings once with `readUserSettingsJSON`, resolve the saved timezone through `database.ResolveTimezone`, use the same eight-day UTC over-fetch and `to = now` range as `fetchPrimaryVitals`, and retain only bucket rows whose `bucket_start` date is on or after the saved-timezone Logged Day six calendar days before today. This preserves the current seven-Logged-Day card window, including timezone-aware bucketing and the extra-day protection at the leading boundary. Call the existing `queryBucketed` with `database.BucketDay` for every metric; do not reproduce its special handling for steps or blood pressure. If settings cannot be read, report the settings error and mark all eight aggregates as errors because their timezone prerequisite is unavailable, while still attempting Presence and needs-attention.

Call the optimized `dataTypesPresence` helper directly so its complete all-time map and bounded single-statement behavior remain authoritative. Extract the Food Meal count query currently embedded in `foodHandlers.NeedsAttentionCount` into a shared helper based on `needsAttentionStatuses`, and use that helper from both the existing endpoint and the dashboard read model. A Presence failure affects only `presence`; an aggregate failure affects only that metric; and a needs-attention failure affects only `needs_attention`. The later dashboard consumer will map those states to the existing fail-open Presence behavior, per-card no-result behavior, and zero-count fallback.

Add the matching discriminated TypeScript types and `api.getDashboardReadModel()` in `frontend/lib/api.ts`. Keep `api.getSettings`, `api.dataTypesPresence`, `api.data`, and `api.needsAttentionCount` because other screens and the deferred migration still use them. Do not add value caching, request coalescing, a materialized table, a schema migration, or write-path invalidation. All sections remain fresh on read and are not wrapped in a transaction that would couple their failure domains. The measured latency does not justify invalidation across imports, manual record writes/deletes, Food Meal lifecycle mutations, profile or Nutrition Target inputs, and settings/timezone changes.

The dashboard component migration, Playwright fixture rewrite, and `LoggingGapCard` decision are deliberately deferred. No `dogfood` or `prod` deployment, hostname, Access-policy, credential, or production profiling work is part of this change.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e`

## Ground rules
This spec is implemented by an automated pass running unattended. **There is no approval step and nothing is waiting for one** — do not look for a tick, a marker, or a sign-off anywhere, and do not wait for one.

Tick the boxes in this file as the work is completed; they are the record of progress, and the pipeline reads them to decide whether the change is finished.

Out of scope, deliberately: do NOT mark the pull request ready for review and do NOT call a forge merge API. Implementation marks the pull request ready only after the task list is complete. Afterward Completion may ask the Store to perform Automatic Merge only when the planner and final implementation agent authorized the exact result. Leave the pull request in a state worth reading.

### Task 1: Define the shared backend read-model primitives
- [x] Add `backend/pkg/server/dashboard.go` with the fixed eight-metric dashboard registry and response types for the four explicitly discriminated sections, including an aggregate entry for every metric even when settings or queries fail.
- [x] Represent successful settings with `json.RawMessage` or an equivalent opaque-object mechanism so unknown keys survive unchanged, while normalizing the no-row case from `readUserSettingsJSON` to `{}`.
- [x] Extract a `needsAttentionCount` helper from `backend/pkg/server/food_meal_detail.go` that uses `needsAttentionStatuses`, route `foodHandlers.NeedsAttentionCount` through it, and update `backend/pkg/server/data_types_presence_benchmark_test.go` to use the production helper instead of its duplicate count query.
- [x] Keep `dataTypesPresence` and `queryBucketed` as the sole Presence and daily-aggregate implementations rather than adding dashboard-specific calculations.
- [x] Mark completed

### Task 2: Implement the authenticated dashboard endpoint
- [ ] Implement `DashboardHandler` and an internal builder that accepts the authenticated user ID and one captured `now`, then register `GET /api/dashboard` on the protected router in `backend/pkg/server/server.go`.
- [ ] Scope every section directly to `claims.UserID`; prove by construction that `?user=` cannot switch the target, while a request without claims returns 401 instead of a partial envelope.
- [ ] Read settings once, resolve its timezone, construct the current eight-day over-fetch range, and filter each successful daily result to the seven saved-timezone Logged Days before placing it in the response.
- [ ] Attempt Presence and needs-attention independently; after successful settings, attempt all eight aggregates independently so one query error cannot suppress a sibling section.
- [ ] On a settings read error, emit `settings.status = "error"`, mark all aggregates as errors without guessing a timezone, and still return the independently obtained Presence and needs-attention states.
- [ ] Log internal section errors with enough section/type context to diagnose them, but return only the stable status contract and HTTP 200 for authenticated partial failures.
- [ ] Mark completed

### Task 3: Pin the backend contract and failure isolation
- [ ] Add handler contract tests in `backend/pkg/server/dashboard_handler_test.go` covering unauthenticated rejection, self-only scoping despite a `user` query parameter, missing settings as `{}`, opaque settings keys, a complete Presence map, all eight aggregate keys and row shapes, successful empty arrays, and the needs-attention count.
- [ ] Add fixed-clock internal tests in `backend/pkg/server/dashboard_internal_test.go` with records around UTC and non-UTC day boundaries, proving the handler uses one instant, saved-timezone bucket labels, the extra-day over-fetch, and exactly the latest seven Logged Days.
- [ ] Inject a settings read failure and assert a 200 response with settings and all aggregates in error while Presence and needs-attention still succeed.
- [ ] Inject a combined Presence query failure and assert only the Presence section is in error, with the settings, aggregates, and needs-attention sections intact.
- [ ] Inject failures for the generic aggregate path and the special steps and blood-pressure paths, asserting each time that only the named Vital Card's aggregate is in error and every sibling aggregate remains usable.
- [ ] Inject a Food Meal count failure and assert only `needs_attention` is in error; retain the existing standalone needs-attention endpoint tests to prove the extracted helper did not change its status set or caller scoping.
- [ ] Mark completed

### Task 4: Add the typed frontend client contract
- [ ] In `frontend/lib/api.ts`, add a `DashboardPrimaryMetric` literal union, reusable `{status: "ok", ...} | {status: "error"}` section types, and a `DashboardReadModel` interface matching the backend envelope without weakening aggregate rows or settings to `any`.
- [ ] Add `api.getDashboardReadModel()` using the existing authenticated `apiFetch` path so transparent refresh and Cf-Access recovery continue to apply to the endpoint's overall 401 response.
- [ ] Do not coalesce or cache the read model and do not remove the existing settings, Presence, aggregate, or needs-attention methods before their remaining callers are migrated.
- [ ] Extend `frontend/lib/api.test.ts` to assert the exact `GET /api/dashboard` path and successful decoding of both success and error section variants, including opaque settings keys and a single failed aggregate alongside successful siblings.
- [ ] Mark completed
