# Stop loading dashboard vitals twice
Idea: ya-breeze/idea-forge#413

## Why

The dashboard has a deterministic request-amplification problem before any database caching question is reached. `Dashboard` in `frontend/app/page.tsx` calls `api.me()` and waits for it, while `AuthenticatedShell` independently calls the same method. `LanguageProvider` loads `/users/me/settings`, and after its own authentication request the dashboard loads the same settings again.

The largest duplication is the primary-vitals effect. Once `ready` becomes true it requests daily aggregates for the eight DataType-backed entries in `PRIMARY_METRICS`. The settings response then writes `timezone`, which is a dependency of that effect; for an account with a saved timezone, all eight `/api/data/{type}?bucket=day` requests run a second time. A normal opening can therefore issue 22 core GETs before the four requests owned by `LoggingGapCard`, and the grid is withheld until settings and Presence have settled. The repeated wait reported by the only real user is consistent with work visible directly in the source, so eliminating this known duplication is the first improvement to land.

This should happen now because every dashboard visit pays the cost and the number of requests grows with the fixed Dashboard Card registry rather than with any action by the user. It is also a safer first step than introducing stale data: request cardinality is measurable in an automated test, whereas no production timing evidence currently identifies a particular aggregate query that warrants persistent caching or precomputation.

## How

Add narrowly scoped, in-flight request coalescing for `api.me()` and `api.getSettings()` in `frontend/lib/api.ts`. Concurrent callers share one promise, but the slot is cleared after either success or failure; this is request deduplication, not a value cache. A later navigation must still revalidate the session and reload settings. Keep the fresh read inside `api.updateSettings()` outside this coalescing path so the existing whole-document GET-merge-PUT protection cannot reuse a UI bootstrap read or overwrite newer settings.

In `Dashboard`, begin the settings read on mount instead of placing it behind the component's duplicate `ready` gate. This lets it overlap and coalesce with `LanguageProvider` while the existing session checks continue to control authentication and redirection. Move the primary-vitals request wave into the successful settings-load path and calculate its `from`, `to`, and seven-Logged-Day cutoff from the returned `s.timezone` directly. Remove the separate effect keyed by `[ready, timezone]`; setting `timezone` must continue to feed `LoggingGapCard`, but must no longer trigger another eight aggregate requests. Preserve the current UI contracts: settings failure keeps the grid hidden and retryable, Presence failure fails open, each failed vital becomes an empty result without failing its siblings, needs-attention failure remains non-blocking, and unauthenticated visitors still reach `/login` through the existing session checks.

The performance contract for this slice is deterministic rather than duration-based: on a fresh dashboard navigation with a non-UTC stored timezone, there is one network GET for `/users/me`, one for `/users/me/settings`, and exactly one daily-bucket request for each of the eight DataType-backed primary metrics. The raw weight request made by `LoggingGapCard` is separate and must not be mistaken for a duplicate daily-bucket request.

Persistent browser caching, HTTP cache headers, service-worker storage, database cache tables, and import-time precomputation are deliberately excluded. They require invalidation across health imports, manual record creation/deletion, Food Meal lifecycle changes, profile and Nutrition Target changes, and settings updates; adding that machinery before the confirmed duplicate work is removed would risk showing stale health data without evidence that aggregation remains the bottleneck. Consolidating the remaining dashboard reads into a backend read-model endpoint and optimizing `DataTypesPresenceHandler` are deferred to the follow-up scope. No `dogfood` or `prod` deployment, hostname, Access-policy, or credential work is part of this change.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e`

## Ground rules
This spec is implemented by an automated pass running unattended. **There is no approval step and nothing is waiting for one** — do not look for a tick, a marker, or a sign-off anywhere, and do not wait for one.

Tick the boxes in this file as the work is completed; they are the record of progress, and the pipeline reads them to decide whether the change is finished.

Out of scope, deliberately: do NOT mark the pull request ready for review and do NOT call a forge merge API. Implementation marks the pull request ready only after the task list is complete. Afterward Completion may ask the Store to perform Automatic Merge only when the planner and final implementation agent authorized the exact result. Leave the pull request in a state worth reading.

### Task 1: Coalesce concurrent bootstrap reads
- [x] In `frontend/lib/api.ts`, introduce separate module-level in-flight promise slots for the parameterless `/users/me` and `/users/me/settings` GETs, and route `api.me()` and `api.getSettings()` through them.
- [x] Return the same promise to concurrent callers and clear only the matching slot in `finally`, on both fulfillment and rejection, so a settled request is never retained as cached data and an old promise cannot clear a newer one.
- [x] Give `api.updateSettings()` a private uncached settings read before its merge and PUT; retain its rejection behavior when that fresh read fails and update the surrounding comments to distinguish bootstrap coalescing from write-path freshness.
- [x] Do not coalesce parameterized data calls, mutation calls, authentication exchanges, or the four independent sources used by `LoggingGapCard`.
- [x] Mark completed

### Task 2: Make settings drive one primary-vitals load
- [x] In `frontend/app/page.tsx`, start the dashboard settings effect without waiting for `ready`, while retaining the current `settingsStatus`, toast, retry, strict `more_data_hidden === true` normalization, and saved-order reconciliation behavior.
- [x] Extract or locally encapsulate the existing primary-vitals loading logic and invoke it once from a successful settings result, passing `s.timezone` directly and using one captured `now` for the request upper bound and cutoff calculation.
- [x] Preserve the existing eight-day over-fetch followed by `loggedDayKey` filtering to seven local calendar days, `Promise.all` parallelism, per-metric `.catch(() => [])` degradation, and `extractVital` response transformation.
- [x] Remove the standalone `[ready, timezone]` vitals effect so `setTimezone(s.timezone)` updates `LoggingGapCard` without issuing a second daily-bucket wave.
- [x] Prevent a settled request from updating state after the effect is cleaned up, including during unmount and a settings retry, without weakening `dashboardReady` or the existing Presence and authentication behavior.
- [x] Mark completed

### Task 3: Unit-test the in-flight-only contract
- [x] Extend `frontend/lib/api.test.ts` with controlled fetch promises proving that two overlapping `api.me()` calls issue one `/users/me` request and both receive its result.
- [x] Add the equivalent overlapping-call case for `api.getSettings()` and assert both successful and rejected requests clear their slot so a later sequential call performs a new GET.
- [x] Cover `api.updateSettings()` independently: while a bootstrap settings GET is in flight, the update path must issue its own fresh GET, merge that response, and then PUT the complete settings document.
- [x] Keep the existing transparent-refresh tests intact; coalescing must continue to use `apiFetch` and therefore preserve refresh and Cloudflare Access exchange behavior.
- [x] Mark completed

### Task 4: Lock the dashboard request budget in E2E coverage
- [ ] Extend `e2e/tests/dashboard.spec.ts` with a fresh-navigation case whose settings GET returns a non-UTC timezone, ensuring the historical `undefined`-to-saved-timezone transition is exercised.
- [ ] Count browser requests and assert exactly one GET reaches `/api/users/me`, exactly one GET reaches `/api/users/me/settings`, and each DataType-backed member of `PRIMARY_METRIC_TYPES` receives exactly one request with `bucket=day`.
- [ ] Match the bucket query explicitly so `LoggingGapCard`'s intentional raw `/api/data/weight` request is excluded from the primary-vitals count.
- [ ] Wait for the grid and all expected daily-bucket responses before asserting counts, avoiding timing-based sleeps or a count taken while requests can still start.
- [ ] Confirm the existing settings-error retry, Presence fail-open, card visibility/order persistence, and needs-attention cases still pass with the new request lifecycle.
- [ ] Mark completed
