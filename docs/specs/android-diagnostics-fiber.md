# Record Android sync diagnostics and show fiber progress

## Why
Intermittent red refresh errors disappear without a durable report. The phone must retain safe technical evidence while offline and send it to HealthVault when connectivity returns. Fiber currently has an amount but no target, so its widget rail cannot show progress.

## How
Keep one bounded encrypted journal for the current signed-in session, with pending event IDs and successful summary time. Record request attempts, failures and recovery using closed operation and failure categories, timestamp, duration, HTTP status, app version, Android API level and UUID request ID. Never store raw exception text, response bodies, URLs, credentials, medical data or location. Clear the journal and outbox on account changes/sign-out. Upload batches after successful sync and from a Diagnostics screen; acknowledge exact IDs and retain unsent entries on failure. The server accepts authenticated self-only batches, deduplicates retries, stores at most 1000 events per user for 30 days and exposes self-only report reads. Correlate API requests with safe request IDs and structured server logs. Preserve existing sync retry behavior. Exclude alerts, additional automated reactions, crash SDKs and the dedicated full-chain fault-injection/E2E project (owner excluded plan items 5 and 6). Run ordinary focused correctness tests, static checks, build and required WIP validation. Repair the existing logging-gap reorder test's stale move bound when it blocks validation; derive it from the rendered card count without changing application behavior.

Provide an adult reference target of 25g/day independent of weight or calorie target availability, with a native Settings override/reset persisted on the server. Do not auto-assign the adult reference to known under-18 profiles; a manual target remains available. Older servers/caches without a fiber target keep the neutral rail. Fiber reaches green at target and stays green above it. Keep macro overrun behavior. Expose the target in the Android summary API model and widget accessibility. Server age retention uses receipt time, so phone clock skew does not block reports. Hide expired reports on reads and prune on ingestion; inactive records may remain on disk until the next ingestion.

## Validation Commands
`make test-backend`

`make lint`

`/data/android-build.py HealthVault-worktrees/android-diagnostics-fiber/android "testDebugUnitTest lintDebug assembleDebug"`

`git diff --check`

### Task 1: Store and correlate safe diagnostics
- [x] Add bounded authenticated ingestion/read APIs, request correlation and database migration.
- [x] Add encrypted session-scoped Android journal, safe classification and acknowledged upload.
- [x] Cover auth, isolation, validation, deduplication, retention and local session/outbox behavior with focused tests.

### Task 2: Expose diagnostics and fiber settings
- [x] Add the native Diagnostics screen with recent history, current failure, last success and manual sending.
- [x] Add persisted fiber target override/reset, summary contract and green fiber progress.
- [x] Cover missing/legacy targets, overrides and fiber-specific overrun behavior.

### Task 3: Validate and hand off
- [x] Repair the existing reorder test's stale fixed move limit and validate its unchanged assertions.
- [x] Complete ordinary build/static checks and Review Gate.
- [x] Validate the reviewed server on WIP, record Android launcher/device limits, and open a PR.

No production deployment, merge or Play publication is included without owner approval. Native device rendering and offline phone acceptance cannot be proved by the control host's JVM tests or APK build.

## Validation evidence
Backend tests and `make lint` pass. The shared Android container passes 90 JVM tests, lint and the debug APK build. Native correctness, standards and spec/tests reviews pass; both Claude system peer runs completed, and all verified findings are resolved.

WIP `hcw-wip` ran server commit `4f1c091dc80ecdccab967094eb59967489d7ec75`. API smoke checks confirm fiber default/override/reset, acknowledged and deduplicated diagnostics, self-only reads and request-ID correlation in actual server logs. The existing web suite passes 325 distinct tests with one configured skip across the full run and focused reruns. Eight initial failures required the existing WIP webhook token; one existing reorder test required its stale nine-move limit to follow the rendered card count. Both corrected edit-mode tests pass. No application behavior changed for that test repair.

Android has no instrumented device tests. Launcher rendering and phone lifecycle/offline acceptance remain unverified. Owner-excluded alerts and dedicated full-chain fault injection remain outside this change. PR: https://github.com/ya-breeze/HealthVault/pull/108.
