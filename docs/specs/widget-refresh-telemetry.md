# Trace Android widget refresh after food entry

## Why
The owner adds food from a widget, closes the browser, and intermittently sees old widget data. HTTP diagnostics exist, but do not show whether the browser return was received, refresh work started, a snapshot was saved, or widget updates were submitted.

## How
Extend the existing bounded, session-scoped diagnostic journal and authenticated ingestion API. Use closed widget operations and categories, a shared request_id as the flow ID, optional work_id, trigger, provider, snapshot_age_millis and snapshot_changed technical metadata. Snapshot age is bounded to nonnegative signed 64-bit milliseconds; overflow saturates at Long.MAX_VALUE so clock-skew evidence is retained. Never record health values, food content, credentials, URLs or arbitrary exception text. Keep existing HTTP clients and events compatible. Record browser launch and return with one flow ID and propagate it through one-off work, fetch outcome, snapshot persistence and per-provider update submission. Give each periodic execution its own flow ID. Persist the existing session generation atomically with login/logout so browser and WorkManager flow guards survive process recreation; legacy sessions start at generation zero. Record deferral, running-worker cancellation and scheduling failure honestly. When a later enqueue requests replacement of pending work, record replacement_requested on the earlier flow without claiming observed cancellation. Write UI-originated telemetry off the main thread in a bounded ordered queue; successful Glance update submission is not launcher rendering proof. Telemetry must not break refresh or change scheduling/retry behavior. Display the technical stages in Diagnostics and upload the final stages after completion, retaining failed uploads locally. No refresh behavior fix, main merge, production deployment or Google Play upload is included.

Validation scope: run the focused diagnostic ingestion, existing Android summary-contract and authentication E2E suites. The web UI and food processing do not change, so the full unrelated web suite is not a release gate for this change. Pending-work replacement records an intention; only running-worker cancellation is observed directly. Real Android browser/WorkManager lifecycle and launcher rendering still require device acceptance.

## Validation Commands
- `make test-backend`
- `make lint`
- `/data/android-build.py HealthVault-worktrees/widget-refresh-telemetry/android "testDebugUnitTest lintDebug assembleDebug"`
- `make test-e2e BASE_URL=http://192.168.1.54:8892 E2E_ARGS="tests/widget-diagnostics.spec.ts tests/summary-today.spec.ts tests/auth.spec.ts --retries=0"`

### Task 1: Extend diagnostic contract
- [x] Add bounded optional technical metadata and closed widget stage validation, preserving old HTTP events.
- [x] Test ingestion, receipt/replay identity, rejection of invalid metadata and account isolation.
- [x] Mark completed

### Task 2: Instrument Android flow
- [x] Trace browser launch/return, enqueue requests, actual worker start/fetch/result/snapshot and both provider submissions with correlation.
- [x] Preserve cancellation, session-generation guards and current scheduling behavior; make telemetry best effort.
- [x] Display stages and correlation in Diagnostics with localized labels; upload terminal stages.
- [x] Add behavior tests of the diagnostic seams, persistence/serialization and failure handling.
- [x] Mark completed

### Task 3: Validate and review
- [x] Run backend and Android tests, static checks and build.
- [x] Complete native and best-effort Claude peer Review Gate.
- [x] Validate ingestion and existing E2E against WIP; identify the missing real-device browser-return proof explicitly.
- [x] Publish a draft PR with the exact validation scope and remaining device acceptance.
- [x] Mark completed

## Validation evidence

Code commit `37aa20e40d31b4eb44fcf8cc266a1afde0c8fc63` passed the backend suite, `make lint`, and shared Android `testDebugUnitTest lintDebug assembleDebug` (106 tests, no failures). The local Makefile skips Android without an SDK; the shared SDK builder supplies the actual Android test/lint/build evidence. Two process-reconstruction regression tests failed before the durable-generation fix and passed afterwards.

Native correctness, standards and spec review completed without remaining findings. Claude's system peer review completed; confirmed issues were fixed, shared Bundle keys replaced duplicated literals, and a boundary test verifies the agreed nonnegative int64 snapshot age contract. Physical browser/launcher acceptance remains unverified.

The WIP diagnostics, existing summary-contract and authentication E2E suites passed all 10 tests with retries disabled on the code commit above. The first attempt preceded actual server replacement and lacked the matching browser; installing Chromium and waiting for the new runtime resolved those prerequisites. Synthetic event ingestion, acknowledgement, persistence, replay and invalid-metadata rejection were verified. WIP was then restored to its original `main` branch; stack status and rejection of the feature-only API stage signature confirm runtime restoration.

Draft PR: https://github.com/ya-breeze/HealthVault/pull/115. No merge, production deployment or Google Play upload occurred. Subsequent commits contain only this validation evidence and task completion ticks.
