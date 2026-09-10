# Measure nutrition-advice engagement before adding chat
Idea: ya-breeze/idea-forge#385

## Why

HealthVault PR #64 has merged and supplies the cached, refreshable nutrition advice rendered beneath the dashboard’s Healthiness Label. Before adding a larger chat affordance, the product needs evidence that people actually encounter and refresh those advice lines. [HealthVault PR #65](https://github.com/ya-breeze/HealthVault/pull/65) therefore adds the engagement-measurement slice: privacy-minimized qualified-view and refresh aggregates, their API contract, and client-side visibility measurement. The implementation and tests at `240c34d` are the baseline and must be preserved rather than rewritten.

The current specification describes only the later corrective work and incorrectly says that the schema, route, and advice response do not change. That would leave the squash-merged implementation without an accurate design record. The complete specification must cover the full diff from merged-PR-#64 commit `73b7ca4` through the final PR #65 head, including the corrective invariant that a `refresh_success` can never be recorded unless the same refresh invocation successfully recorded `refresh_request` and persisted its replacement `database.FoodAdvice`.

This correction is needed before PR #65 ships. The WIP E2E run at `240c34d` passed every nutrition-engagement scenario; its only unchanged authentication failure passed a separate rerun without retries. The final documentation-only and comment cleanup still requires lint, tests, and the Codex Review Gate on the new exact head.

## How

Persist one `FoodAdviceEngagement` aggregate per user and Logged Day using the existing `models.TenantModel` convention. Store only counts and nullable first/last timestamps for qualified views, refresh requests, and refresh successes. Do not store advice text, health measurements, browser or session identifiers, user-agent data, IP addresses, or future chat content. Atomic upserts must preserve increments under concurrency, normalize event times to UTC, and maintain minimum first and maximum last timestamps independently for each event family.

Expose `logged_day` alongside `generated_at` in successful cached and newly generated `POST /api/food/advice` responses so the client can identify the rendered revision without receiving database IDs. Register authenticated `POST /api/food/advice/engagement` for self-only qualified-view submissions, enforce the repository’s same-origin rule, reject malformed payloads, and verify that the exact advice revision belongs to the caller. Refresh events remain authoritative inside `foodHandlers.PostFoodAdvice`: record a request after validation and before the vision call, then record success only after advice persistence and only if that request write succeeded. Telemetry remains best-effort, accepting undercounting when a request write fails rather than creating an impossible success-only aggregate; do not hold a database transaction open across the vision call.

In `LoggingGapCard`, treat a qualified view as a conservative visibility proxy, not proof that the advice was read. Require the complete advice element to remain in the viewport while the document is visible for two continuous seconds. Deduplicate the same Logged Day and generation timestamp within one tab through guarded `sessionStorage`; storage or engagement-request failures may permit overcounting but must never change advice rendering or refresh behavior. Clean up timers, visibility listeners, and observers when the revision changes or the component unmounts, including protection from already-queued observer callbacks.

Document the signal and privacy boundary in `CONTEXT.md` and retain the evidence gate in `todo.md`. The ephemeral nutrition chat remains separately tracked by [Idea #386](https://github.com/ya-breeze/idea-forge/issues/386) and is deliberately excluded from PR #65. Production cutover and inspection of `dogfood` or `prod` aggregates remain owner work because they require owner-controlled environments and data unavailable to the unattended implementation phase. After deployment, the owner must observe qualified views and refresh requests over a representative period before deciding whether chat has earned implementation.

Preserve the implementation already present at `240c34d`. Besides replacing this incomplete specification, revert only the unrelated wording change in the existing comment at `backend/pkg/database/models_food_test.go:87`. Keep every task unchecked until the complete `73b7ca4..HEAD` diff has been audited against this combined specification; only after that audit, final validation, and the Review Gate may implementation mark the tasks complete.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e`

### Task 1: Persist privacy-minimized engagement aggregates
- [ ] Add `database.FoodAdviceEngagement` in `backend/pkg/database/models_food.go` with `models.TenantModel`, caller ownership, a Logged Day, and a unique index over `user_id` and `logged_day`
- [ ] Store counts and nullable first/last timestamps for qualified views, refresh requests, and refresh successes without adding advice, health, browser, session, network, or chat content
- [ ] Register `FoodAdviceEngagement` in the `database.Open` `AutoMigrate` list in `backend/pkg/database/db.go`
- [ ] Cover migration, user/day uniqueness, isolation between users and days, and the aggregate’s privacy boundary in `backend/pkg/database/models_food_test.go`
- [ ] Mark completed

### Task 2: Add the authenticated qualified-view endpoint
- [ ] Add `backend/pkg/server/food_advice_engagement.go` with `foodHandlers.RecordFoodAdviceEngagement`, accepting only `qualified_view`, `logged_day`, and the advice revision’s `generated_at`
- [ ] Enforce `isSameOriginRequest`, require `ClaimsFromCtx`, reject unknown fields or trailing JSON, validate the canonical Logged Day and nonzero timestamp, and prevent clients from reporting refresh events or naming another user
- [ ] Verify the exact `database.FoodAdvice` revision belongs to the caller before recording engagement, returning the established unauthorized, forbidden, invalid-input, not-found, and unavailable responses
- [ ] Register `POST /api/food/advice/engagement` beside the protected food routes in `backend/pkg/server/server.go`
- [ ] Mark completed

### Task 3: Aggregate event counts and time bounds atomically
- [ ] Implement the unexported `recordFoodAdviceEngagement` helper using a conflict update over `user_id` and `logged_day` so concurrent calls cannot lose increments
- [ ] Update only the selected event family, normalize event time to UTC, preserve the earliest first timestamp and latest last timestamp, and reject unknown events without mutation
- [ ] Keep qualified views client-reportable while reserving `refresh_request` and `refresh_success` for the server refresh path
- [ ] Return storage failures to the handler without creating a partial aggregate or widening the production API
- [ ] Mark completed

### Task 4: Identify advice revisions and instrument refresh outcomes
- [ ] Add `LoggedDay` to `foodAdviceResponse` in `backend/pkg/server/food_advice.go` and populate it for both newly generated and cached available responses while retaining the stable generation timestamp
- [ ] Extend `NutritionAdviceResponse` in `frontend/lib/api.ts` with the matching `logged_day` field for the available response branch
- [ ] In `foodHandlers.PostFoodAdvice`, record `refresh_request` after request validation and before `vision.Client.Advise`, remembering whether that write succeeded for the current invocation
- [ ] Record `refresh_success` only after the replacement `database.FoodAdvice` is persisted and only when the corresponding request measurement succeeded
- [ ] Log and isolate telemetry failures so advice generation, cache persistence, cache hits, and successful delivery retain their existing behavior without a transaction spanning the model call
- [ ] Mark completed

### Task 5: Measure conservative client-side visibility
- [ ] Add `FoodAdviceEngagementRequest` and `api.recordFoodAdviceEngagement` to `frontend/lib/api.ts` using the existing authenticated JSON and empty-response helpers
- [ ] Carry `loggedDay` and `generatedAt` with `DisplayedAdvice` in `frontend/components/LoggingGapCard.tsx` for both initial and refreshed revisions
- [ ] Observe the rendered advice element at a full-intersection threshold and start a two-second timer only while it is wholly visible and `document.visibilityState` is `visible`, resetting the timer whenever either condition stops holding
- [ ] Deduplicate by Logged Day and generation timestamp within the current tab using guarded `sessionStorage`, while keeping storage and engagement-delivery failures invisible to the user
- [ ] Clean up timers, listeners, and observers on revision changes and unmount, and ignore callbacks queued for a disposed revision without changing label, advice, refresh, or precedence rendering
- [ ] Mark completed

### Task 6: Cover endpoint and aggregation behavior
- [ ] Cover authentication, same-origin enforcement, malformed and unknown input, client-supplied refresh events, absent or stale revisions, and cross-user revisions in `backend/pkg/server/food_advice_engagement_test.go`, asserting rejected requests leave aggregates untouched
- [ ] Cover successful qualified-view recording, concurrent aggregation, caller/day isolation, first/last expansion, and already-bracketed timestamps
- [ ] Prove an engagement insert failure returns HTTP 500 without a partial row
- [ ] Add `backend/pkg/server/food_advice_engagement_internal_test.go` under `package server` to exercise qualified-view, refresh-request, and refresh-success counts and bounds, UTC normalization, unrelated-field isolation, and unknown-event non-mutation
- [ ] Mark completed

### Task 7: Regress refresh failure boundaries
- [ ] Extend `TestFoodAdvice_RefreshEngagementOutcomesAndTelemetryIsolation` in `backend/pkg/server/food_advice_test.go` to retain coverage for successful refresh, model failure, cache hit, and best-effort telemetry failure
- [ ] Use a selective SQLite trigger that rejects only `refresh_request` insertion, proving refreshed advice is still returned and cached while no success-only aggregate is created
- [ ] Add a fresh-cache case whose SQLite trigger rejects insertion into `food_advices` after valid model output
- [ ] Assert advice-persistence failure returns HTTP 200 with `{available:false, reason:"unavailable"}`, persists no `FoodAdvice`, and retains one refresh request with zero successes and no success timestamps
- [ ] Verify generated and cached successful responses both carry the server-resolved Logged Day and stable generation timestamp
- [ ] Mark completed

### Task 8: Cover browser measurement and preserve formatting
- [ ] Extend `e2e/tests/logging-gap.spec.ts` advice fixtures and assertions with `logged_day` and `generated_at`, retaining the existing nutrition-card behavior coverage
- [ ] Prove advice hidden by precedence or outside the viewport is not counted, interrupted viewport or document visibility restarts the two-second window, and a continuous qualified interval emits the expected payload once
- [ ] Prove rerender and reload in the same tab do not duplicate a revision, a refreshed revision can earn its own view, endpoint failure leaves advice and refresh usable, and queued callbacks cannot report a superseded revision
- [ ] Keep nutrition-engagement fixtures, route handlers, payload assertions, callbacks, and `NutritionAdviceResponse` fields aligned with surrounding TypeScript formatting without changing timing, values, or behavior
- [ ] Mark completed

### Task 9: Restore the complete design record and verify the exact result
- [ ] Keep `docs/specs/a-nutrition-chat-affordance-on-healthvau.md` sourced to Idea 385 and covering the complete engagement schema, endpoint, advice response, client measurement, corrective refresh invariant, and explicit exclusions for HealthVault PR #65 and Idea #386
- [ ] Update `CONTEXT.md` to define Qualified Advice View as a visibility proxy and document the aggregate-only privacy boundary; update `todo.md` to retain the owner-observed evidence gate before ephemeral chat work begins
- [ ] Revert the unrelated comment rewording at `backend/pkg/database/models_food_test.go:87` to the `73b7ca4` wording without changing the test or other pre-existing code
- [ ] Audit the complete `git diff 73b7ca4..HEAD` against every task before marking any checkbox, preserving the correct implementation at `240c34d` except for the requested specification and comment cleanup
- [ ] Run every command in `## Validation Commands` on the final exact head and run the Codex Review Gate after all changes, resolving any in-scope findings before handoff
- [ ] Mark completed
