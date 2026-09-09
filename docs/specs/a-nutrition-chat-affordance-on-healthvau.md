# Preserve nutrition refresh engagement metric invariants
Idea: ya-breeze/idea-forge#385

## Why

The nutrition-advice engagement slice already committed on HealthVault PR #65 measures qualified views, refresh requests, and successful refreshes without adding the proposed chat. A remaining failure-path bug can make those aggregates internally misleading: `foodHandlers.PostFoodAdvice` in `backend/pkg/server/food_advice.go` logs and ignores a failed `refresh_request` write, then independently records `refresh_success` after advice generation and persistence. A transient failure limited to the first write can therefore produce a success without the corresponding request.

The existing `TestFoodAdvice_RefreshEngagementOutcomesAndTelemetryIsolation` coverage does not expose that case because its trigger rejects every engagement insert. There is also no regression covering failure to persist `database.FoodAdvice` between the request and success measurements. These gaps need closing before PR #65 ships, alongside the visibly inconsistent indentation in the engagement additions to `frontend/lib/api.ts` and `e2e/tests/logging-gap.spec.ts`.

## How

Preserve the existing aggregate model, authenticated endpoint, client visibility measurement, documentation, and browser regressions. In `foodHandlers.PostFoodAdvice`, retain whether the current refresh invocation successfully recorded `refresh_request`, and attempt `refresh_success` only when that write succeeded and the generated `FoodAdvice` was subsequently persisted. A telemetry failure must remain best-effort: refreshed advice is still generated, cached, and returned. This deliberately undercounts both events when request telemetry is unavailable instead of recording an impossible success-only outcome. Do not hold a database transaction open across the vision call.

Use selective SQLite failure triggers in the existing server tests to distinguish request-telemetry failure from `FoodAdvice` persistence failure. Add focused package-internal coverage for the low-level `recordFoodAdviceEngagement` branches and preserve the existing external handler coverage. Normalize only the malformed indentation introduced by this engagement slice, following the surrounding TypeScript file style without changing browser behavior or test timing.

No schema, route, or request/response changes are required. The nutrition chat tracked separately by idea #386 remains excluded, as do production cutover and inspection of `dogfood` or `prod` aggregates. Those deployment and evidence-gathering actions require owner-controlled environments and remain owner work after this corrective change lands.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e`

## Ground rules
This spec is implemented by an automated pass running unattended. **There is no approval step and nothing is waiting for one** — do not look for a tick, a marker, or a sign-off anywhere, and do not wait for one.

Tick the boxes in this file as the work is completed; they are the record of progress, and the pipeline reads them to decide whether the change is finished.

Out of scope, deliberately: do NOT mark the pull request ready for review and do NOT call a forge merge API. Implementation marks the pull request ready only after the task list is complete. Afterward Completion may ask the Store to perform Automatic Merge only when the planner and final implementation agent authorized the exact result. Leave the pull request in a state worth reading.

### Task 1: Pair refresh success with the current request measurement
- [x] Update `foodHandlers.PostFoodAdvice` in `backend/pkg/server/food_advice.go` to remember whether `recordFoodAdviceEngagement` successfully persisted `refresh_request` for the current refresh invocation
- [x] Record `refresh_success` only when that request write succeeded and the refreshed `database.FoodAdvice` row was persisted; retain the existing ordering of request measurement before `vision.Client.Advise` and success measurement after the cache write
- [x] Keep telemetry best-effort so a failed request measurement is logged but does not change advice generation, cache persistence, or the successful response
- [x] Extend `TestFoodAdvice_RefreshEngagementOutcomesAndTelemetryIsolation` in `backend/pkg/server/food_advice_test.go` with a selective request-event failure that would allow a success insert, proving the advice is returned and cached while no success-only aggregate is created
- [x] Mark completed

### Task 2: Cover failure to persist refreshed advice
- [x] Add a fresh-cache refresh case in `backend/pkg/server/food_advice_test.go` whose SQLite trigger rejects insertion into `food_advices` after the model returns valid lines
- [x] Assert the handler returns HTTP 200 with `{available:false, reason:"unavailable"}`, persists no `FoodAdvice`, and retains exactly one refresh request with zero refresh successes and no success timestamps
- [x] Keep the existing model-failure, cache-hit, successful-refresh, and all-telemetry-failure assertions intact so the new regression isolates the cache-write boundary
- [x] Mark completed

### Task 3: Complete low-level engagement regression coverage
- [x] Add `backend/pkg/server/food_advice_engagement_internal_test.go` using `package server` to exercise the unexported `recordFoodAdviceEngagement` helper without widening the production API
- [x] Cover `qualified_view`, `refresh_request`, and `refresh_success` using fixed timestamps, invoking success only after a request; verify the selected count and first/last bounds update while unrelated event fields remain unchanged
- [x] Cover earlier, later, and already-bracketed timestamps, including UTC normalization, so each event family preserves minimum first and maximum last semantics
- [x] Prove an unknown event returns an error without creating or changing an aggregate
- [x] Extend `backend/pkg/server/food_advice_engagement_test.go` to assert rejected authentication, origin, and malformed-input requests leave engagement aggregates untouched, and that an engagement insert failure returns HTTP 500 without a partial row
- [x] Mark completed

### Task 4: Normalize the engagement slice formatting
- [ ] Align `logged_day` with the other available-response fields in `NutritionAdviceResponse` in `frontend/lib/api.ts`
- [ ] Re-indent the nutrition-advice fixtures, engagement route handlers, payload assertions, and visibility/deduplication tests added in `e2e/tests/logging-gap.spec.ts` to match the surrounding two-space nesting and multiline callback style
- [ ] Make no behavioral, timing, fixture-value, or assertion changes while performing the formatting cleanup
- [ ] Mark completed
