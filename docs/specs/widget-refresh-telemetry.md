# Trace Android widget refresh after food entry

## Why
The owner adds food from a widget, closes the browser, and intermittently sees old widget data. HTTP diagnostics exist, but do not show whether the browser return was received, refresh work started, a snapshot was saved, or widget updates were submitted.

## How
Extend the existing bounded, session-scoped diagnostic journal and authenticated ingestion API. Use closed widget operations and categories, a shared request_id as the flow ID, optional work_id, trigger, provider, snapshot_age_millis and snapshot_changed technical metadata. Never record health values, food content, credentials, URLs or arbitrary exception text. Keep existing HTTP clients and events compatible. Record browser launch and return with one flow ID and propagate it through one-off work, fetch outcome, snapshot persistence and per-provider update submission. Give each periodic execution its own flow ID. Record deferral, cancellation and failure honestly; successful Glance update submission is not launcher rendering proof. Telemetry must not break refresh or change scheduling/retry behavior. Display the technical stages in Diagnostics and upload the final stages after completion, retaining failed uploads locally. No refresh behavior fix, main merge, production deployment or Google Play upload is included.

## Validation Commands
- `make test-backend`
- `make lint`
- `/data/android-build.py HealthVault-worktrees/widget-refresh-telemetry/android "testDebugUnitTest lintDebug assembleDebug"`
- `make test-e2e BASE_URL=http://192.168.1.54:8892 E2E_ARGS=--retries=0`

### Task 1: Extend diagnostic contract
- [ ] Add bounded optional technical metadata and closed widget stage validation, preserving old HTTP events.
- [ ] Test ingestion, receipt/replay identity, rejection of invalid metadata and account isolation.
- [ ] Mark completed

### Task 2: Instrument Android flow
- [ ] Trace browser launch/return, enqueue requests, actual worker start/fetch/result/snapshot and both provider submissions with correlation.
- [ ] Preserve cancellation, session-generation guards and current scheduling behavior; make telemetry best effort.
- [ ] Display stages and correlation in Diagnostics with localized labels; upload terminal stages.
- [ ] Add behavior tests of the diagnostic seams, persistence/serialization and failure handling.
- [ ] Mark completed

### Task 3: Validate and review
- [ ] Run backend and Android tests, static checks and build.
- [ ] Complete native and best-effort Claude peer Review Gate.
- [ ] Validate ingestion and existing E2E against WIP; identify the missing real-device browser-return proof explicitly.
- [ ] Publish a draft PR with the exact validation scope and remaining device acceptance.
- [ ] Mark completed
