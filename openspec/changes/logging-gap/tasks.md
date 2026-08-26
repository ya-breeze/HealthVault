## 1. Backend: Daily Totals endpoint

- [x] 1.1 Add `GET /api/food/daily-totals` handler (new file alongside
      `backend/pkg/server/food_completeness.go`, e.g. `food_daily_totals.go`): auth check (401),
      parse `from`/`to` (400 on missing/malformed), clamp `to` to yesterday in the caller's zone
      *first* (reuse `callerTimezoneAndThreshold`'s timezone half, or extract a shared
      `callerTimezone` helper), then validate `from > to` and the >92-day span against the
      (possibly clamped) `to` — same order as `GetCompleteness`
- [x] 1.2 Add a function computing, for a user and an inclusive Logged-Day date range, one
      `{date, calories}` entry per day — querying `confirmed`-status `FoodMeal` rows for the range,
      grouping by Logged Day via the existing timezone helper, summing `calories` only (no
      protein/carbs/fat — no consumer of this change reads them); a day with no confirmed rows gets
      a zero entry
- [x] 1.3 Register the route in `backend/pkg/server/server.go` (`GET /food/daily-totals`)
- [x] 1.4 Tests: happy path across several days including a zero-meal day, unconfirmed/failed/
      processing meals excluded from sums, `to` clamping when `to` names today or a future date,
      `from` equal to today with `to` also today (400, post-clamp inversion caught), the >92-day
      cap, 401 with no auth, confirms no `?user=` override is honored

## 2. Frontend: API client

- [x] 2.1 Add a `DailyTotal` interface (`date`, `calories`) and `api.getFoodDailyTotals(from, to)`
      (`GET /food/daily-totals`) to `frontend/lib/api.ts`, documented the same way as
      `getCompleteness`

## 3. Frontend: Logging Gap computation library

- [ ] 3.1 Add `frontend/lib/loggingGap.ts`. Add `rejectOutliers(records: {day: number, value:
      number}[]): { kept: {day: number, value: number}[], rejected: {day: number, value: number}[],
      bootstrapSiblingAmbiguous: boolean }` — chronological walk, rejects a record whose implied
      rate from the last **kept** record exceeds 2.0 kg/day per design.md decision 2; a rejected
      record does not become the new reference point; a same-day candidate (`day[i] ===
      lastKept.day`) is never rate-checked, is always provisionally kept, and — same as a rejected
      record — never becomes the new `lastKept` reference (only a record that has actually passed a
      rate check, or the bootstrap below, can); before evaluating the third and later records,
      validate the initial `lastKept` candidate by rate-checking the first two records that fall on
      different calendar days against each other (skipping same-day pairs the same way the main walk
      does), dropping the earlier one and repeating against the next different-day record until two
      agree or one remains; operates on raw, un-bucketed input only — no day-bucketing stage runs
      inside this function; the function itself has no notion of "visible window" vs. lead-in (it
      isn't passed that boundary), so it returns every rejected record with its `day`, unfiltered —
      callers that need a count scoped to the visible 28-day window (3.7's rejection cap, 3.9's
      outlier note) filter `rejected` by `day` against the window boundary they already have rather
      than asking this function to do it. `bootstrapSiblingAmbiguous` is set to `true` iff the
      initial-anchor bootstrap step above rejects a candidate that has one or more same-day-exempted
      siblings (design.md decision 2's same-day-sibling suppression rule) — this is a single boolean
      determined once during the bootstrap step over the whole (lead-in-extended) input the function
      was called with, not scoped to or filtered by any later caller's visible-window boundary, and
      not derived from `kept`/`rejected` day overlap by the caller (see 3.7: overlap between the two
      arrays can also arise from an unrelated main-walk case with no same-day-sibling relationship,
      so it is not a valid proxy for this condition)
- [ ] 3.2 Unit tests for 3.1: a single implausible mid-series reading is excluded and does not
      affect evaluation of the following reading; a gradual multi-day change (≤2 kg/day rate) is
      never rejected; an empty/single-record input passes through unchanged; two consecutive
      rejections in a row (both compared against the same last-kept point); a bad *leading* record
      (e.g. 75 kg before a true ~91 kg series) is rejected during initial-anchor validation rather
      than poisoning `lastKept` for subsequent good readings; two same-day records are never rejected
      against each other and both pass through; a same-day pair appearing as one of the first two
      records is skipped (neither rejected nor treated as agreeing) and initial-anchor validation
      continues to the next different-day record; a same-day, provisionally kept record never becomes
      `lastKept` — a later different-day record is rate-checked against the last record that actually
      passed a rate check, not against the same-day record; a symmetric two-cluster leading sequence
      (e.g. 91, 75, 75.2, 91.2) anchors on the first agreeing pair found (75, 75.2) and rejects both
      91 readings, and the returned `rejected` array reflects this; a leading same-day pair whose
      first member is later rejected as the bootstrap candidate (e.g. 75.0 and 75.2 both on day 1,
      then 91.0 on day 2, then 91.2 on day 3) rejects only 75.0 — 75.2 stays same-day exempted and is
      never rate-checked, rejected, or promoted to candidate, since the bootstrap candidate is 75.0
      throughout the day-1 skip and 91.0 is rate-checked against 75.0, not 75.2; a longer alternating sequence
      where rejections within the visible window exceed 3 is exercised in 3.8, not here, since the
      cap itself is applied by `checkHardFloor` (3.7); `bootstrapSiblingAmbiguous` is `false` for all
      of the above fixtures except the leading-same-day-pair-with-a-later-bootstrap-rejection one
      (75.0/75.2 day 1, 91.0 day 2, 91.2 day 3), where it is `true` (75.0 is the bootstrap-rejected
      candidate and 75.2 is its same-day-exempted sibling); a fixture with two records on the same
      day that are *not* related to the bootstrap (e.g. `lastKept` established at 91.0 on day 1, then
      day 2 carries 94.0 — rejected on its own rate check against day 1's `lastKept` — and 91.2 —
      independently kept on its own rate check against the same `lastKept`) has both a `kept` and a
      `rejected` record sharing day 2's calendar day, yet `bootstrapSiblingAmbiguous` is `false`,
      proving that a shared day between the two output arrays is not by itself evidence of this
      condition
- [ ] 3.3 Add `slopeStandardError(points: {x: number, y: number}[], slope: number, intercept:
      number): number | null` — standard OLS slope SE from the regression's own residuals; returns
      `null` when fewer than 3 distinct points are given (design.md decision 3's `n < 3` case)
- [ ] 3.4 Unit tests for 3.3: a known small dataset with a hand-computed expected SE; exactly 2
      points returns `null`; exactly 3 points returns a defined (non-null) value; residuals of zero
      (perfectly linear input) do not throw or divide by zero unexpectedly for `n >= 3`
- [ ] 3.5 Add a pure `resolveLoggingGapWindow(now, timezone)` (or equivalent) function deriving the
      28-day window (ending yesterday in the caller's Logged Day) and the wider lead-in-extended
      range to fetch, so this boundary arithmetic is unit-testable on its own rather than living only
      inside the card component (5.1)
- [ ] 3.6 Unit tests for 3.5: window ends at yesterday regardless of time-of-day; a weigh-in landing
      exactly on the window's first or last calendar day is included; the lead-in range is wider than
      the visible 28-day window
- [ ] 3.7 Add two pure functions to `loggingGap.ts`, split so the hard floor (design.md decision 5)
      can be evaluated without ever constructing a regression — matching spec.md's "Before computing
      anything else ... SHALL NOT compute a slope" ordering for the hard-floor conditions, and
      avoiding calling `emaSeries`/`linearRegression` on too few points in the first place:
      - `checkHardFloor(kept, rejected, bootstrapSiblingAmbiguous, windowStartDayOffset,
        perDayWindowData, mostRecentKeptDayOffset, windowLastDayOffset): boolean` — takes 3.1's
        `kept` and `rejected` arrays (filtered by the caller to the visible window using
        `windowStartDayOffset`, the same way 3.1's own doc-comment describes: the `kept` count is
        **not** interchangeable with the EMA series' length, since two same-day-exempted survivors
        bucket into a single EMA day and would otherwise undercount), 3.1's
        `bootstrapSiblingAmbiguous` flag passed through **unfiltered** (it is not scoped to
        `windowStartDayOffset` at all — see 3.1: the bootstrap step it reflects runs on the
        chronologically first records of the whole lead-in-extended range, so the pair it concerns
        is typically in the lead-in, not the visible window, and filtering it the way `kept`/
        `rejected` are filtered for the rejection cap would silently discard it), the per-day
        `{state, calories}` window data, and the day offsets needed for the freshness gate — no EMA
        series, regression, or SE. Returns `true` (hard floor fires) when: `n < 2` raw weigh-ins
        survive within the window (counted from `kept`), more than 3 raw weigh-ins rejected within
        the window (counted from `rejected`, decision 2's rejection cap), `bootstrapSiblingAmbiguous`
        is `true` (design.md decision 2's same-day-sibling suppression rule — checked directly from
        the flag, never inferred from whether some calendar day appears in both `kept` and
        `rejected`, since an ordinary main-walk day can produce that overlap with no same-day-sibling
        relationship at all — see 3.2's counter-fixture), `n < 3` valid (complete/confirmed_complete)
        days, or the most recent surviving weigh-in is more than 7 days before the window's last day
      - `computeLoggingGap(regression: {slope: number, intercept: number}, se: number | null,
        nutritionTargetCalories: number, perDayWindowData): {kind: 'gap', value: number, interval:
        number} | {kind: 'not_enough_data'}` — called only once `checkHardFloor` has already returned
        `false` (so a regression exists); computes Implied Intake, Mean Logged Intake, the Logging Gap
        value, and the interval, treating `trendErrorKcal` as unbounded when `se` is `null` (3.3's
        `n < 3` case), returning `{kind: 'gap', ...}` when `abs(value) > interval` or
        `{kind: 'not_enough_data'}` when `abs(value) <= interval` (statistical silence — same tag as
        the hard floor, both paths produce it per the spec's single surface state)
      Callers (5.1) call `checkHardFloor` first and only run `emaSeries`/`linearRegression`/3.3's
      `slopeStandardError` and call `computeLoggingGap` when it returns `false` — never the reverse
      order, so a regression is never attempted on the too-few-points cases the hard floor exists to
      catch
- [ ] 3.8 Unit tests for 3.7's `checkHardFloor`, called with `kept`/`rejected`/`bootstrapSiblingAmbiguous`/
      per-day window data — no regression or SE constructed anywhere in these cases, proving the gate
      genuinely doesn't need one: fewer than 2 weigh-ins → `true`; exactly 2 raw survivors that share a
      calendar day (one EMA day, two `kept` records) does not trip the `n < 2` floor → `false`,
      proving the gate counts `kept` records rather than an EMA series' day count; fewer than 3 valid
      (complete/confirmed_complete) days (0, 1, and 2 — the boundary) → `true`; exactly 3 valid days
      → `false`; the most recent surviving weigh-in exactly 7 days before the window's last day →
      `false`, and exactly 8 days before it → `true` via the freshness gate alone; exactly 3 rejected
      weigh-ins within the window → `false`, and exactly 4 → `true` via the rejection cap alone (a
      two-cluster fixture whose surviving cluster alone would otherwise pass every other gate and
      compute a narrow `gap`), proving the cap — not the interval or the freshness gate — is what
      catches a persistent wrong cluster (design.md decision 2's residual limitation); the 3.2
      same-day-sibling-of-a-bootstrap-rejection fixture (75.0/75.2 on day 1, 91.0 on day 2, 91.2 on
      day 3), passed with `bootstrapSiblingAmbiguous: true` from 3.1's own output for that fixture →
      `true`, proving the same-day-sibling suppression rule fires even though `kept` has 3 records
      and `rejected` has only 1 (neither the `n < 2` nor the rejection-cap condition would catch it
      alone); that same fixture passed with the rejected candidate's day and its sibling's day both
      *outside* the caller's `windowStartDayOffset`-filtered `kept`/`rejected` (i.e. both in the
      lead-in) but `bootstrapSiblingAmbiguous` still `true` → `true`, proving the condition is not
      lost when the window filtering that the rejection cap relies on would otherwise hide it; the
      3.2 unrelated-same-day-overlap counter-fixture (day-1 `lastKept` at 91.0, day 2 with 94.0
      rejected and 91.2 independently kept, `bootstrapSiblingAmbiguous: false`) → `false` even though
      day 2 appears in both `kept` and `rejected`, proving the gate does not treat array overlap
      itself as the trigger
- [ ] 3.8b Unit tests for 3.7's `computeLoggingGap`, called only with a regression/SE already
      constructed (as callers would after `checkHardFloor` returns `false`): a gap whose interval
      covers zero → `not_enough_data`; a gap exactly equal to its interval → `not_enough_data` (`<=`
      comparison, per spec — zero sits on the range's own boundary); a gap clearly outside its
      interval → `gap` with the expected value/interval (mirror the spec's `sqrt(275^2 + 195^2)`
      example); a negative `value` (Mean Logged Intake exceeds Implied Intake) outside its interval
      → `gap` with a negative value, exercised end-to-end through the card's direction-aware
      rendering (task 5.2); days with `unconfirmed`/`incomplete` state are excluded from the Mean
      Logged Intake average, not counted as zero; `se: null` (3.3's `n < 3` case) → `not_enough_data`
      regardless of `value`, since `trendErrorKcal` is treated as unbounded; a dataset whose weigh-ins
      stop several days before the window's last day (fewer/less-spread trend points, per design.md
      decision 1) tends to produce a wider interval than an otherwise-identical dataset with a
      weigh-in on the window's last day, but this is a tendency, not the safety mechanism — the case
      that actually catches a stale-but-narrow-SE series is 3.8's freshness-gate test, which fires
      before `computeLoggingGap` is ever called
- [ ] 3.9 Add an `excludedOutlierCount` (or boolean) output alongside 3.7's result (whichever of
      `checkHardFloor`/`computeLoggingGap` the caller last ran), counting only exclusions within the
      28-day window itself (not the lead-in extension used to converge the EMA — the same count
      `checkHardFloor` uses for the rejection cap), so the card can render the outlier note (spec's
      "Outlier note" scenarios) regardless of which of the four content states is showing

## 4. Frontend: dashboard card registry generalization

- [ ] 4.1 In `frontend/lib/vitals.ts`: introduce a `CardId = DataType | 'logging_gap'` type; widen
      `PRIMARY_METRICS`'s entry type and `DashboardCardPref.type` to `CardId`; add a
      `{ type: 'logging_gap' }` entry to `PRIMARY_METRICS` in the desired default position
- [ ] 4.2 Update `reconcileMetricOrder`'s `known` set construction to include `'logging_gap'`
      (already derived from `PRIMARY_METRICS`, so this should require no logic change — verify with
      a test) so a saved order containing it round-trips correctly
- [ ] 4.3 In `frontend/app/page.tsx`: ensure `SECONDARY_TYPES` (`DATA_TYPES.filter(...)`) is
      unaffected by the new `'logging_gap'` member (it isn't a `DataType`, so the existing filter
      logic should already exclude it correctly — verify with a test rather than assuming) and that
      the presence-based hide/show logic (`hasPresence`, zero-presence exclusion from Edit mode) is
      never evaluated for the `'logging_gap'` entry — it is always eligible to render in both the
      read-only grid and Edit mode, gated only by its own `hidden` preference
- [ ] 4.4 Unit tests for 4.1-4.3: a saved order including `logging_gap` reorders correctly; a saved
      order predating this change (missing `logging_gap` entirely) appends it visible at the
      default position, same as any other newly-added metric per "Saved order tolerates the default
      metric set changing"; hiding `logging_gap` persists and is respected on next load; a presence
      response that omits (or explicitly returns false for) `logging_gap`-shaped data has no effect
      on whether the card renders

## 5. Frontend: Logging Gap Card

- [ ] 5.1 Add a `LoggingGapCard` component: on mount (and whenever the dashboard's data range
      changes), use task 3.5's `resolveLoggingGapWindow` to get the window and lead-in range, fetch
      the `weight` series over that range (`api.data('weight', ...)`, reusing the existing lead-in
      pattern from the weight Trend Projection code in
      `frontend/app/data/[type]/DataTypeClient.tsx`), `api.getNutritionTarget()`,
      `api.getCompleteness(from, to)`, and `api.getFoodDailyTotals(from, to)`; once all four settle,
      run 3.1's `rejectOutliers`, then 3.7's `checkHardFloor` — only when it returns `false` does the
      component go on to run `emaSeries`/`linearRegression` (from `frontend/lib/dataTypeMeta.ts` per
      design.md decision 4) and 3.3's `slopeStandardError`, then 3.7's `computeLoggingGap` with that
      regression and SE (see 3.7's note on why this order matters: it keeps a regression from ever
      being constructed on the too-few-points inputs the hard floor exists to catch); track a loading
      flag that's true until all four requests settle, and catch non-422 failures from any of the four
      (network error, 5xx), surfacing them as a distinct `{kind: 'retrieval_error'}` result — kept separate
      from `{kind: 'not_enough_data'}` per design.md decision 6's revised reasoning (this card's four
      requests are its own, not `app/page.tsx`'s shared `dataMap`, so a failure here does not
      correlate with any other card's state)
- [ ] 5.2 Render the card's states per the spec: a loading state while 5.1's requests are in flight;
      then exactly one of the four content states — a kcal/day range (never a bare point estimate)
      for `{kind: 'gap', ...}`, direction-aware copy for a negative `value` (logged intake exceeding
      implied intake, rendered with the absolute range, never a negative-to-negative range); "not
      enough data yet" for `{kind: 'not_enough_data'}`; "temporarily unavailable" for
      `{kind: 'retrieval_error'}` from 5.1; and catch `NutritionTargetUnmetError` from
      `getNutritionTarget()` to render the distinct "complete your profile/goal weight" state
      (design.md decision 6), linking to the relevant profile/goal-weight settings location
- [ ] 5.3 Render the outlier-excluded note (task 3.7's output) alongside whichever of the four
      states is showing, and the two static caveats — photo-estimated intake bias, and the
      activity multiplier's unquantified error — per spec's "Logging Gap Card content and
      placement" requirement
- [ ] 5.4 Wire the card into `app/page.tsx`'s dashboard render alongside the vitals grid, respecting
      the task-4 registry's order/visibility for the `'logging_gap'` entry

## 6. i18n

- [ ] 6.1 Add English strings for the card's title, the four content states (including
      "temporarily unavailable"), the outlier note, and the two caveats (photo estimation, activity
      multiplier) to `frontend/lib/i18n/en.ts`
- [ ] 6.2 Add the corresponding Russian strings to `frontend/lib/i18n/ru.ts`

## 7. Docs

- [ ] 7.1 `CONTEXT.md`: add glossary entries for **Implied Intake**, **Logging Gap**, and **Trend
      Weight** (Dashboard/Nutrition-targets section), and update the existing **Food Card** entry's
      example if needed to reference the Logging Gap Card as a concrete instance
- [ ] 7.2 New `docs/adr/ADR-008-<slug>.md` (`Status: Proposed`) recording: reporting a Logging Gap
      instead of a TDEE (and why — no third expenditure source exists in the data, per idea #10's
      grilling); silence-over-a-clamp as the safety mechanism; the rate-based 2 kg/day outlier rule
      and its rejection cap (design.md decision 2's residual limitation and bound); the quadrature
      interval formula and its two fixed/derived error terms; the card's distinct
      "temporarily unavailable" state for non-422 fetch failures (design.md decision 6); the card
      registry's Food Card generalization
- [ ] 7.3 `todo.md`: record idea #10 as shipped by this change instead of adaptive TDEE — note the
      framing change explicitly (not a TDEE, doesn't touch ADR-006) so a future reader doesn't
      re-open the original "replaces the activity multiplier" framing as if it were still live

## 8. E2E

- [ ] 8.1 Extend or add an `e2e/tests/` spec: seed weight + confirmed meals producing a clearly
      out-of-interval Logging Gap, load the dashboard, assert the card shows a range (not a bare
      number) and both caveats (photo estimation, activity multiplier)
- [ ] 8.2 E2E coverage for the "not enough data yet" state with a freshly seeded user (no weight,
      no meals)
- [ ] 8.3 E2E coverage for the "complete your profile/goal weight" state with a user who has weight
      and food history but no goal weight set
- [ ] 8.4 E2E coverage for hiding/reordering the Logging Gap Card via Edit mode and confirming the
      change persists across a reload, alongside existing Vital Cards
- [ ] 8.5 E2E or component-level coverage for the "temporarily unavailable" state: simulate a non-422
      failure (e.g. a 500) from one of the card's four requests while the others succeed, and assert
      the card shows "temporarily unavailable" rather than "not enough data yet"
- [ ] 8.6 Run the full suite against `hcw-wip`

## 9. Verification

- [ ] 9.1 `make lint`
- [ ] 9.2 `make test`
- [ ] 9.3 `npx tsc --noEmit` in `frontend/`
- [ ] 9.4 `openspec validate logging-gap --strict`
