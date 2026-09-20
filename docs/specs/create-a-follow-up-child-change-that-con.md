# Consume the dashboard read model in Dashboard
Idea: ya-breeze/idea-forge#667

## Why

The prerequisite contract from `docs/specs/after-this-presence-profiling-optimizati.md` is now present on this branch: `backend/pkg/server/dashboard.go` serves authenticated `GET /api/dashboard`, and `frontend/lib/api.ts` exposes its independently discriminated settings, Presence, eight aggregate, and needs-attention sections through `api.getDashboardReadModel()`. `Dashboard` in `frontend/app/page.tsx` still ignores that contract and coordinates the legacy browser wave itself through `api.getSettings()`, `api.dataTypesPresence()`, eight `api.data(..., 'day')` calls in `fetchPrimaryVitals`, and `api.needsAttentionCount()`.

That leaves every fresh dashboard navigation paying eleven core HTTP requests despite the server already being able to assemble the same information with one settings read and isolated source failures. The completed profile in `docs/investigations/idea-478-dashboard-read-path.md` found no material database latency that warrants caching; the remaining benefit is consolidating browser orchestration while preserving the current UI’s deliberately different degradation policies.

This cutover is timely because the backend and typed frontend contract are already tested, but the dashboard’s saved layout, pre-render Presence gate, retry lifecycle, and partial-result behavior remain tied to the legacy routes. The Playwright suite also encodes those routes directly, so the consumer and its route fixtures must move together before the new endpoint provides its intended request-budget improvement.

## How

Replace the settings, Presence, primary-aggregate, and needs-attention effects in `frontend/app/page.tsx` with one effect that calls `api.getDashboardReadModel()`. Keep the independent `api.me()` effect and login redirect. Start the read-model request without serializing it behind that identity check, while retaining the effective authentication gate before the dashboard grid renders.

The combined effect must preserve the existing `settingsAttempt` retry and cancellation ownership. Set the settings state to loading for each attempt, use one cleanup-owned `cancelled` flag, and check it before any response or error path mutates React state. An overall request failure and a `settings.status === 'error'` response both produce the existing retryable settings error, toast, disabled Customize control, and blocked grid. A successful settings section continues through `reconcileMetricOrder`, strict `more_data_hidden === true` normalization, and the saved timezone passed to both Food Cards.

Map `presence.status === 'ok'` to its complete value and an error branch to `null`, then mark Presence resolved in either case so `hasPresence` and `hasCardPresence` retain their fail-open behavior without flashing an unfiltered grid before the response settles. For each DataType-backed entry in `PRIMARY_METRICS`, pass a successful server-provided `rows` array through the existing `extractVital` transformation; map an aggregate error to `null` for that card only. Continue rendering from the saved `order`, not aggregate object iteration, so card ordering and visibility do not change. Only after that mapping exists, remove `fetchPrimaryVitals` and its now-unused date/type imports. Map a successful `needs_attention` section to its count and an error to zero.

Rewrite `e2e/tests/dashboard.spec.ts` fixtures around complete `/api/dashboard` envelopes, with focused overrides for settings, Presence, individual aggregate, and needs-attention states. Preserve coverage for pending and failed settings, in-page retry, Presence fail-open behavior, per-card no-result degradation, saved order and visibility, and the needs-attention fallback. Update the fresh-navigation budget to require exactly one `/api/dashboard` request and zero legacy core-wave Presence, bucketed-primary, or needs-attention requests. Continue counting `/api/users/me` and LanguageProvider’s `/api/users/me/settings` read independently, and track the raw weight, Today Summary, Day Completeness, and Daily Totals requests still owned by the Food Cards rather than misclassifying them as legacy dashboard traffic.

Audit dashboard navigations and route interception in `e2e/tests/mobile-nav.spec.ts`, `e2e/tests/logging-gap.spec.ts`, and `e2e/tests/settings.spec.ts`. Replace mocks that were intended to control dashboard state with equivalent read-model envelopes, while retaining direct settings GET/PUT helpers used to seed or persist account settings and retaining `LoggingGapCard`’s four independent request fixtures.

Do not extend the read model for `LoggingGapCard` in this split. The current `DashboardReadModel` has no discriminated sections for Today Summary, raw weight history, Day Completeness, or Daily Totals, while `LoggingGapCard.tsx` deliberately uses `Promise.allSettled` to make Summary failure a whole-card error and failures of the other three sources degrade the Logging Gap and Healthiness rows together without removing Today. Moving those requests requires a separate backend-contract review and is deferred below. Do not add caching, precomputation, schema changes, or invalidation work; the existing profile does not justify them. No public hostname, Cloudflare Access policy, credential, `dogfood` or `prod` stack change, or deployment is part of this implementation, and no owner-only action is required.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e`

### Task 1: Consume the read-model envelope in Dashboard
- [ ] In `frontend/app/page.tsx`, replace the settings, Presence, eight daily-aggregate, and needs-attention request paths with one `api.getDashboardReadModel()` effect keyed by `settingsAttempt`, while leaving the independent `api.me()` redirect intact.
- [ ] Preserve the existing retryable settings failure UI and toast for both an overall request rejection and `settings.status === 'error'`; successful settings must still drive `reconcileMetricOrder`, strict More Data visibility normalization, and the timezone props for `LoggingGapCard` and `FoodLogHistoryCard`.
- [ ] Preserve the pre-render Presence gate, map a Presence error to fail-open `null`, and ensure the grid cannot render before both identity readiness and the read-model’s settings and Presence sections have settled successfully enough for their existing policies.
- [ ] Map every successful aggregate section through `extractVital` using the DataType-backed entries in `PRIMARY_METRICS`, map only the failed metric to `null`, and keep rendering order and hidden state driven exclusively by the reconciled saved `order`.
- [ ] Map `needs_attention.status === 'ok'` to its count and the error branch to zero without changing the indicator’s rendering or links.
- [ ] Remove `fetchPrimaryVitals` and its unused `loggedDayKey`, `DataType`, or related imports only after the server rows use the existing transformation path.
- [ ] Mark completed

### Task 2: Preserve retry and cancellation ownership
- [ ] Keep a cleanup-owned cancellation guard around the complete dashboard load and check it before every success or failure state transition, so an unmounted component or superseded `settingsAttempt` cannot apply stale settings, Presence, vital, count, readiness, or error state.
- [ ] Reset only the loading/readiness state needed to prevent stale content from satisfying the grid gate during a retry, without discarding persisted order or visibility defaults into a renderable state.
- [ ] Keep `settingsAttempt` as the retry trigger used by `vitals-grid-retry`; one click must issue one new read-model request and must not restore any legacy request wave.
- [ ] Leave both Food Card components’ own lifecycles and requests unchanged.
- [ ] Mark completed

### Task 3: Pin the frontend client boundary
- [ ] Update `frontend/lib/api.test.ts` so the dashboard client test continues to require exactly `GET /api/dashboard` and covers the success and error discriminants consumed by the page, including opaque settings, Presence failure, one failed aggregate beside successful siblings, and needs-attention failure.
- [ ] Assert that `api.getDashboardReadModel()` returns the envelope without initiating legacy settings, Presence, aggregate, or needs-attention calls.
- [ ] Keep the standalone legacy API methods because other screens and components still use them.
- [ ] Mark completed

### Task 4: Rewrite dashboard fixtures and degradation coverage
- [ ] In `e2e/tests/dashboard.spec.ts`, add a complete dashboard-envelope fixture with successful default settings, full Presence, all eight aggregate entries, and needs-attention count, plus narrowly scoped overrides for each section.
- [ ] Replace dashboard-purpose `/api/users/me/settings`, `/api/data-types/presence`, bucketed `/api/data/{type}`, and `/api/food/meals/needs-attention-count` interceptors with `/api/dashboard` envelopes while retaining settings routes used for LanguageProvider or real settings writes.
- [ ] Preserve the tests for legacy saved-order decoding, hidden-card and More Data persistence, pending and failed settings gates, in-page retry, Presence filtering and fail-open behavior, and absence of pre-resolution grid rendering.
- [ ] Drive an individual aggregate error or successful empty rows through the envelope and assert only that Vital Card shows its existing no-result state while sibling cards remain usable.
- [ ] Drive needs-attention success and error branches through the envelope and retain the indicator’s pluralization, link, and zero-fallback assertions.
- [ ] Mark completed

### Task 5: Enforce the new request budget and audit adjacent suites
- [ ] Rewrite the fresh-navigation request-budget test in `e2e/tests/dashboard.spec.ts` to require exactly one `/api/dashboard` request, exactly one `/api/users/me` request, and exactly one `/api/users/me/settings` read attributable to LanguageProvider.
- [ ] Assert that a fresh navigation issues no `/api/data-types/presence`, no `/api/food/meals/needs-attention-count`, and no `bucket=day` request for any of the eight primary metrics.
- [ ] Count the independently retained Food Card requests separately, including unbucketed weight history, `/api/summary/today`, and the distinct Logging Gap and Food Log History completeness and daily-total windows, so their traffic cannot hide a legacy-core regression.
- [ ] Replace the Presence-only dashboard interceptor in `e2e/tests/mobile-nav.spec.ts` with a complete `/api/dashboard` envelope that preserves the layout test’s all-types-present intent.
- [ ] Audit `e2e/tests/logging-gap.spec.ts` and `e2e/tests/settings.spec.ts` for dashboard navigations and stale core-wave assumptions; retain their direct settings seeding/writes and Logging Gap source mocks, and add a dashboard envelope only where a test intentionally controls dashboard state.
- [ ] Update comments in all four suites so they describe the read-model request, LanguageProvider settings read, and independent Food Card requests accurately.
- [ ] Mark completed
