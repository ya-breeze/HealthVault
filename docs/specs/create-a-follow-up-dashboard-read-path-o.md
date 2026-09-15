# Profile and optimize dashboard Presence queries
Idea: ya-breeze/idea-forge#478

## Why

The duplicate-fetch fix now present on this branch gives the remaining dashboard read path a stable shape: `Dashboard` in `frontend/app/page.tsx` performs one settings read, one Presence read, eight daily aggregate reads, and one needs-attention read, while `LoggingGapCard` independently loads its four inputs. Before those reads are consolidated behind a new dashboard endpoint, the database work inside each source needs a reproducible baseline so the next change optimizes measured work rather than merely reducing browser request count.

`DataTypesPresenceHandler` in `backend/pkg/server/api.go` is already a concrete hotspot by query cardinality. It walks all 27 entries in `typeRegistry` serially and performs `COUNT` against each table. This consumes 27 database round trips on every fresh dashboard load, and each count processes every matching index entry even though the response needs only whether the first row exists. The registered tables already have per-user indexes, and the table names come from the server-owned allowlist rather than request input, so one allowlisted `UNION ALL` statement containing bounded `EXISTS` probes can retain the endpoint contract while reducing that component to one database statement.

This should land first because Presence optimization is independently reviewable, benefits the current dashboard immediately, and becomes reusable groundwork for the later dashboard read model. Combining it with the new response envelope, frontend lifecycle migration, `LoggingGapCard` degradation analysis, and extensive Playwright fixture changes would cross too many failure boundaries for one review.

## How

Add a reproducible local profile of the post-duplicate-fix fresh-load database path. An internal-package benchmark under `backend/pkg/server` should seed representative settings, primary vital records, and Food Meals, then measure the settings read, the current Presence computation, the eight `queryBucketed` daily aggregates used by `PRIMARY_METRICS`, and the needs-attention count both separately and as one legacy dashboard-read sequence. Pair elapsed-time and allocation output with a test-only GORM statement counter so the investigation records deterministic query cardinality even when benchmark timings vary by machine. Keep the existing browser request-budget case in `e2e/tests/dashboard.spec.ts` unchanged; it already establishes the network side of the baseline.

Replace only the Presence implementation in this split. Build one SQL statement from a stable, sorted traversal of `typeRegistry`; interpolate only its trusted table identifiers, parameterize every returned type name and user ID, and use one `EXISTS(SELECT 1 ... WHERE user_id = ? LIMIT 1)` arm per registered table. Scan the rows into the same complete `map[string]bool` returned today. Keep `DataTypesPresenceHandler` authenticated, retain `resolveUser` and its family-member `?user=` behavior, and continue returning HTTP 500 with no partial map if any constituent probe fails. Do not add a `database.Storage` method merely to move the server registry across package boundaries; the existing handler already uses `storage.DB()` because the allowlist lives in the server layer.

Record the before-and-after statement counts, representative benchmark results, and `EXPLAIN QUERY PLAN` evidence in `docs/investigations/idea-478-dashboard-read-path.md`. The decision is based primarily on eliminating 26 of 27 serial Presence statements and bounding each table lookup, not on a brittle wall-clock threshold. No cache, materialized table, HTTP cache header, or precomputation belongs in this split, so imports, manual writes/deletes, Food Meal mutations, profile and Nutrition Target changes, and timezone/settings changes need no invalidation mechanism. No `dogfood` or `prod` profiling, deployment, hostname, Access-policy, or credential work is included; those environments are owner-controlled and are unnecessary for validating this query-shape improvement.

The authenticated dashboard read-model endpoint, its TypeScript contract, dashboard cutover, and any inclusion of `LoggingGapCard` inputs are deliberately deferred to the child change. They require a separately reviewed partial-result contract so a settings failure still blocks and remains retryable, Presence still fails open, each vital still degrades independently to no result, needs-attention still falls back to zero, and the Nutrition card can retain its row-level behavior.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e`

## Ground rules
This spec is implemented by an automated pass running unattended. **There is no approval step and nothing is waiting for one** — do not look for a tick, a marker, or a sign-off anywhere, and do not wait for one.

Tick the boxes in this file as the work is completed; they are the record of progress, and the pipeline reads them to decide whether the change is finished.

Out of scope, deliberately: do NOT mark the pull request ready for review and do NOT call a forge merge API. Implementation marks the pull request ready only after the task list is complete. Afterward Completion may ask the Store to perform Automatic Merge only when the planner and final implementation agent authorized the exact result. Leave the pull request in a state worth reading.

### Task 1: Establish the post-fix database baseline
- [ ] Add an internal-package benchmark in `backend/pkg/server/data_types_presence_benchmark_test.go` that seeds an empty user and a representative populated user, then exercises the settings read, the existing serial Presence computation, the eight dashboard daily aggregates (`steps`, `heart_rate`, `sleep`, `heart_rate_variability`, `distance`, `weight`, `blood_pressure`, and `oxygen_saturation`), and the `needsAttentionStatuses` count separately and as one legacy fresh-load sequence.
- [ ] Add test-only GORM tracing or equivalent statement counting around the measured operations, excluding seed/setup work, so the benchmark can report deterministic SQL-statement counts alongside `ns/op` and allocations without changing production logging.
- [ ] Include both empty and high-cardinality Presence cases so the profile distinguishes fixed round-trip amplification from the additional work `COUNT` performs when a user has many rows.
- [ ] Mark completed

### Task 2: Collapse Presence into bounded probes
- [ ] In `backend/pkg/server/api.go`, extract the Presence query work used by `DataTypesPresenceHandler` into a helper that accepts the resolved user ID and returns a complete `map[string]bool` or an error.
- [ ] Have the helper sort the `typeRegistry` keys for deterministic SQL construction and issue one allowlisted `UNION ALL` query whose arms use bounded `EXISTS` probes; concatenate only registry-owned table identifiers and bind type names and user IDs as parameters.
- [ ] Normalize SQLite's boolean result for every row, verify the result covers every registered type exactly once, and treat a malformed or incomplete scan as an error rather than returning a partial map.
- [ ] Route `DataTypesPresenceHandler` through the helper while preserving its claims check, `FamilyIDFromCtx` plus `resolveUser` behavior, response shape, and all-or-nothing HTTP 500 error contract.
- [ ] Keep the optimization read-through only: do not add process memory, cache tables, schema migrations, write-path invalidation, or changes to any health-data mutation.
- [ ] Mark completed

### Task 3: Pin compatibility and query cardinality
- [ ] Extend `backend/pkg/server/data_types_presence_handler_test.go` so populated and absent types still produce the same booleans and every `typeRegistry` member remains present in the response.
- [ ] Retain coverage for unauthenticated requests, family-member selection through `?user=`, and rejection of a user outside the caller's family.
- [ ] Replace the old per-table `COUNT` error injection with a failure that reaches the combined query, and assert the handler returns HTTP 500 without a partial JSON response when any referenced table cannot be probed.
- [ ] Add an internal helper-level assertion that Presence executes exactly one SQL statement after user resolution, preventing a later refactor from silently restoring a per-table loop.
- [ ] Cover both an empty account and an account with multiple rows in more than one registered table, proving `EXISTS` reports presence without changing the complete response shape.
- [ ] Mark completed

### Task 4: Record the profiling decision
- [ ] Add `docs/investigations/idea-478-dashboard-read-path.md` describing the measured fresh-load components, dataset sizes, benchmark invocation, before-and-after statement counts, and representative benchmark results.
- [ ] Include `EXPLAIN QUERY PLAN` output or a concise interpretation showing that each `EXISTS` arm uses the registered table's per-user index and terminates after finding a row rather than counting all matches.
- [ ] State the resulting seam explicitly: this change optimizes the existing Presence endpoint, while the child change will define and consume the authenticated dashboard read-model response.
- [ ] Document why no caching or precomputation was introduced and list the import, manual record, Food Meal, profile/Nutrition Target, and settings/timezone invalidation sources that any later cache design must cover.
- [ ] Mark completed
