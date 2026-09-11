# Remove the nutrition advice refresh control

Idea: https://ideaforge.ikoro.in/idea/504

## Why

The nutrition card carried a "Get new advice" link under the advice lines. It asked the model to
write the same advice again from the same inputs. The owner called it silly on 2026-09-11 and
asked for it to go.

He is right about what it offered. The advice is a deterministic judgment turned into prose: the
label, the reason codes and the seven-day means are all fixed before the model is called, so a
refresh could only reword the same finding. A user who pressed it because the advice looked wrong
got a differently-worded version of the same wrong-looking advice. The surface that actually
answers "why does it say that" shipped the same day, as the advice basis and the chat beside it.

Removing the link removes the whole refresh path, not only the control. The `refresh` flag on the
advice request exists to serve it, and the `refresh_request` and `refresh_success` engagement
counters exist to measure it. Counters that can no longer fire are worse than absent ones: a zero
in the aggregates would later read as "nobody refreshes" when the truth is that nobody can. The
owner chose the full removal for that reason.

Qualified-view measurement stays. It measures whether the advice is seen at all, which does not
depend on the refresh control.

## How

**Drop the control and everything that existed only for it.** In `LoggingGapCard.tsx` that is the
refresh `TapTarget`, `refreshAdvice`, the `adviceRefreshError` state and the error line it
rendered, and the `adviceLoading` disabled state. The flex row that separated the two controls
goes with it, because only the discuss control is left. The advice lines, the label, the discuss
control, the qualified-view measurement and the precedence rules are untouched.

**Drop the request flag.** `refresh` leaves `NutritionAdviceRequest` and `foodAdviceRequest`.
`PostFoodAdvice` keeps exactly one path: serve the cached advice for this Logged Day and input
hash, or generate and cache it when there is none. The double-checked cache lookup around the
mutex stays, because two tabs opening at once still race for the first generation.

**Drop the refresh events.** `recordFoodAdviceEngagement` keeps only `qualified_view`, and the
`refresh_request` and `refresh_success` constants, switch branches and telemetry calls go.
`database.FoodAdviceEngagement` loses its six refresh columns.

AutoMigrate adds columns and never drops them, so the six columns stay in any existing
`food_advice_engagements` table with whatever they last held. Nothing reads or writes them, and no
migration is written to remove them: this is a single-user deployment whose aggregate is a handful
of rows, and a hand-written column drop risks the data that matters to save bytes that do not.
The privacy boundary is unaffected, because those columns only ever held counts and timestamps.

**Keep the corrective invariant's coverage where it still applies.** The regression proving a
`refresh_success` could never be recorded without its `refresh_request` goes with the events it
guarded. What remains and must stay covered is the surrounding behaviour it was written around:
advice generation, cache persistence, a cache hit, and a persistence failure returning
`{available:false, reason:"unavailable"}` without writing a row.

**Browser coverage follows the same rule.** The end-to-end tests that pressed the refresh control
go. Two engagement tests used a refresh to produce a second advice revision inside one page life,
which is no longer possible: with the control gone, the card fetches advice once per load, so a
revision changes only between loads.

They are re-seeded differently, because they were proving different things. The one asserting a
new revision earns its own qualified view reloads the page and serves a newer generation
timestamp, which is how a second revision now reaches a reader. The one asserting a queued
observer callback cannot report a superseded revision navigates away instead: with one revision
per page life, the surviving case for that guard is a callback delivered after the card unmounts,
and the `disposed` flag is still the only thing that stops it reporting.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e`

### Task 1: Remove the control from the card
- [x] Delete the refresh `TapTarget`, `refreshAdvice`, `adviceRefreshError` and the advice error
      line from `frontend/components/LoggingGapCard.tsx`
- [x] Leave the discuss control in place without the flex row that only existed to separate two
      controls, and leave the label, advice lines, qualified-view measurement and precedence
      rendering unchanged
- [x] Remove `refresh` from `NutritionAdviceRequest` in `frontend/lib/api.ts`
- [x] Remove the `loggingGap.adviceRefresh`, `adviceRefreshing` and `adviceUnavailable` strings
      from both `frontend/lib/i18n/en.ts` and `ru.ts`
- [x] Mark completed

### Task 2: Remove the refresh path from the advice endpoint
- [x] Remove `Refresh` from `foodAdviceRequest` in `backend/pkg/server/food_advice.go` and reduce
      `PostFoodAdvice` to the cache-or-generate path, keeping the double-checked lookup around the
      mutex
- [x] Remove the `refresh_request` and `refresh_success` telemetry calls and the
      `refreshRequestRecorded` bookkeeping
- [x] Remove the `refreshRequestEvent` and `refreshSuccessEvent` constants and their switch
      branches from `backend/pkg/server/food_advice_engagement.go`
- [x] Remove the six refresh columns from `database.FoodAdviceEngagement` in
      `backend/pkg/database/models_food.go`
- [x] Mark completed

### Task 3: Bring the Go tests with it
- [x] Remove `TestFoodAdvice_RefreshEngagementOutcomesAndTelemetryIsolation` and every other case
      that posts `refresh`, keeping coverage for generation, cache persistence, a cache hit, and an
      advice-persistence failure returning the unavailable response without writing a row
- [x] Remove the refresh event families from
      `backend/pkg/server/food_advice_engagement_internal_test.go`, keeping qualified-view counts,
      bounds, UTC normalization and unknown-event non-mutation
- [x] Update `backend/pkg/server/food_advice_engagement_test.go` and
      `backend/pkg/database/models_food_test.go` so no assertion names a removed column
- [x] Prove the endpoint still rejects an unknown field, so a client posting the old `refresh`
      flag fails loudly rather than being silently ignored
- [x] Mark completed

### Task 4: Bring the browser tests with it
- [x] Remove the end-to-end tests that press the refresh control from
      `e2e/tests/logging-gap.spec.ts`
- [x] Re-seed the new-revision test to reload with a newer generation timestamp, and the
      queued-callback test to navigate away rather than change revision
- [x] Assert the refresh control is absent wherever advice renders
- [x] Mark completed

### Task 5: Record the decision and verify the result
- [x] Update `CONTEXT.md` so Qualified Advice View no longer describes refresh counters
- [x] Update `todo.md` to record that the refresh affordance was removed on 2026-09-11 and why
- [x] Run every command in `## Validation Commands` against the deployed `hcw-wip` stack on the
      final head
- [x] Run the Review Gate and resolve every valid finding before handoff
- [x] Mark completed
