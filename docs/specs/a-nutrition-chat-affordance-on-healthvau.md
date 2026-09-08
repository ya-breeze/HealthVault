# Measure nutrition-advice engagement before adding chat
Idea: ya-breeze/idea-forge#331

## Why

The chat is intentionally gated on evidence that the nutrition advice is used, not merely available. That evidence does not exist in the current checkout. `vision.Client` in `backend/pkg/vision/vision.go` has no `Advise` method, `backend/pkg/server/server.go` registers no `/api/food/advice` route, `frontend/lib/api.ts` has no advice client, and `LoggingGapCard` renders only the sustainability warning or deterministic Healthiness Label. The repository therefore cannot show that advice lines have been read or refreshed; it shows that the advice change from HealthVault PR #64 has not landed here yet.

The owner has resolved the chat’s persistence question in favour of an ephemeral conversation, but that does not remove the usage gate. The first shippable part of this idea is consequently narrow engagement measurement for the advice introduced by PR #64. Landing it immediately after the advice change avoids an unmeasured period and gives the owner durable evidence on which to decide whether the card has earned a larger chat affordance.

## How

This change is sequenced after HealthVault PR #64. Its implementation branch must start from the merged advice implementation rather than recreating or duplicating that pull request. It adds a per-user, per-Logged-Day aggregate recording qualified advice views, refresh requests and successful refreshes. The aggregate stores counts and first/last timestamps only: no advice text, health measurements, browser identifiers, session identifiers or future chat content.

A response from `POST /api/food/advice` is not treated as evidence that somebody read it. `LoggingGapCard` records a qualified view only after the rendered advice remains fully visible while the document is active for two continuous seconds. This is still a visibility proxy rather than proof of attention, and documentation must call it that. A `sessionStorage` marker deduplicates the same advice revision within one browser-tab session; storage failure merely permits another count and must never break the card. Refresh interest is measured authoritatively in the existing server refresh path: increment the request count after validation and before the model call, then increment the success count only after replacement advice has been persisted.

The engagement write is authenticated, self-only and same-origin, following `ClaimsFromCtx` and `isSameOriginRequest`; clients cannot name another user. It validates that the referenced Logged Day and advice revision belong to the caller before atomically updating the aggregate. Measurement failures are best-effort from the dashboard and do not hide advice or surface a user-facing error. There is no analytics SDK, third-party disclosure or general event framework in this slice.

The chat itself is deliberately excluded. After this measurement change and PR #64 are deployed, the owner must inspect production aggregates over a representative period and record whether qualified views and refresh requests demonstrate actual use. That observation touches a `dogfood` or `prod` stack and production data unavailable to the unattended implementation phase, so it is owner work described here rather than an untickable task. If advice is not viewed and refreshed, the next change should improve or remove the advice affordance instead of adding chat.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e`

### Task 1: Persist privacy-minimized advice engagement aggregates
- [ ] Add `FoodAdviceEngagement` to `backend/pkg/database/models_food.go`, using the repository’s `models.TenantModel` convention and a unique index over `user_id` and the advice Logged Day
- [ ] Store qualified-view, refresh-request and refresh-success counts plus nullable first/last timestamps; do not store advice text, health data, user-agent data, session identifiers or IP addresses
- [ ] Register the model in `database.Open`’s `AutoMigrate` list in `backend/pkg/database/db.go`
- [ ] Add database coverage proving migration, uniqueness and isolation between users and Logged Days
- [ ] Mark completed

### Task 2: Record qualified views and refresh outcomes server-side
- [ ] Add `backend/pkg/server/food_advice_engagement.go` with a `foodHandlers.RecordFoodAdviceEngagement` handler accepting only a qualified-view event tied to the advice response’s Logged Day and stable generation timestamp
- [ ] Register `POST /api/food/advice/engagement` beside the other protected food routes in `backend/pkg/server/server.go`; require authenticated same-origin requests and reject malformed dates, unknown revisions and advice belonging to another user
- [ ] Atomically upsert the caller’s daily aggregate so concurrent dashboard tabs cannot lose increments or move first/last timestamps backwards
- [ ] Extend the refresh branch behind `POST /api/food/advice` in `backend/pkg/server/food_advice.go` to record a validated refresh request before generation and a success only after refreshed advice is persisted; telemetry-write failures must be logged without changing the advice response
- [ ] Ensure the advice response exposes its Logged Day and stable generation timestamp so the client can identify the exact rendered revision without receiving database IDs
- [ ] Mark completed

### Task 3: Emit a conservative client-side visibility signal
- [ ] Add the engagement request type and `api.recordFoodAdviceEngagement` method to `frontend/lib/api.ts`, matching the authenticated JSON conventions already used by the `api` object
- [ ] In `frontend/components/LoggingGapCard.tsx`, observe the advice element introduced by PR #64 and start a two-second timer only while the whole element intersects the viewport and `document.visibilityState` is `visible`; reset the timer whenever either condition stops holding
- [ ] Send one qualified-view event per advice Logged Day and generation timestamp in the current tab session, using guarded `sessionStorage` access consistent with `frontend/lib/session.ts`
- [ ] Keep measurement best-effort: swallow request/storage failures, clean up observers and timers on state changes or unmount, and never change advice rendering, refresh behaviour or the warning-label-advice precedence
- [ ] Mark completed

### Task 4: Cover privacy, counting and browser behaviour
- [ ] Add `backend/pkg/server/food_advice_engagement_test.go` covering authentication, same-origin enforcement, invalid input, absent or stale advice revisions, cross-user isolation, atomic aggregation and first/last timestamp semantics
- [ ] Extend the advice handler tests introduced by PR #64 to prove successful refreshes increment both refresh counters, failed model calls increment requests only, cached non-refresh requests increment neither, and telemetry failures do not fail advice delivery
- [ ] Extend `e2e/tests/logging-gap.spec.ts` to prove advice hidden by precedence or outside the viewport is not counted, two continuous visible seconds produces one event, interrupted visibility resets the timer, and rerender/reload within the same tab session does not duplicate the same revision
- [ ] Assert that a newly refreshed revision can produce its own qualified view and that engagement endpoint failures leave the advice and refresh control usable
- [ ] Mark completed

### Task 5: Record the evidence gate
- [ ] Update `CONTEXT.md` to define a qualified advice view as a visibility proxy, not proof that the text was read, and document the aggregate-only privacy boundary
- [ ] Update `todo.md`’s Phase 4 section to record that chat remains unbuilt until the owner observes real qualified views and refresh requests from the deployed advice lines
- [ ] Document that production inspection and the resulting go/no-go decision belong to the owner because unattended implementation cannot access the `dogfood` or `prod` stack or its data
- [ ] Mark completed
