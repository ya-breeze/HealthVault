## 1. Backend: Daily Totals endpoint

- [ ] 1.1 Add `GET /api/food/daily-totals` handler (new file alongside
      `backend/pkg/server/food_completeness.go`, e.g. `food_daily_totals.go`): auth check (401),
      parse `from`/`to` (400 on missing/malformed), clamp `to` to yesterday in the caller's zone
      *first* (reuse `callerTimezoneAndThreshold`'s timezone half, or extract a shared
      `callerTimezone` helper), then validate `from > to` and the >92-day span against the
      (possibly clamped) `to` — same order as `GetCompleteness`
- [ ] 1.2 Add a function computing, for a user and an inclusive Logged-Day date range, one
      `{date, calories, protein_grams, carbs_grams, fat_grams}` entry per day — querying
      `confirmed`-status `FoodMeal` rows for the range, grouping by Logged Day via the existing
      timezone helper, summing each field; a day with no confirmed rows gets an all-zero entry
- [ ] 1.3 Register the route in `backend/pkg/server/server.go` (`GET /food/daily-totals`)
- [ ] 1.4 Tests: happy path across several days including a zero-meal day, unconfirmed/failed/
      processing meals excluded from sums, `to` clamping when `to` names today or a future date,
      `from` equal to today with `to` also today (400, post-clamp inversion caught), the >92-day
      cap, 401 with no auth, confirms no `?user=` override is honored

## 2. Frontend: API client

- [ ] 2.1 Add a `DailyTotal` interface (`date`, `calories`, `protein_grams`, `carbs_grams`,
      `fat_grams`) and `api.getFoodDailyTotals(from, to)` (`GET /food/daily-totals`) to
      `frontend/lib/api.ts`, documented the same way as `getCompleteness`

## 3. Frontend: Logging Gap computation library

- [ ] 3.1 Add `frontend/lib/loggingGap.ts`. Add `rejectOutliers(records: {day: number, value:
      number}[]): {day: number, value: number}[]` — chronological walk, rejects a record whose
      implied rate from the last **kept** record exceeds 2.0 kg/day per design.md decision 2; a
      rejected record does not become the new reference point
- [ ] 3.2 Unit tests for 3.1: a single implausible mid-series reading is excluded and does not
      affect evaluation of the following reading; a gradual multi-day change (≤2 kg/day rate) is
      never rejected; an empty/single-record input passes through unchanged; two consecutive
      rejections in a row (both compared against the same last-kept point)
- [ ] 3.3 Add `slopeStandardError(points: {x: number, y: number}[], slope: number, intercept:
      number): number | null` — standard OLS slope SE from the regression's own residuals; returns
      `null` when fewer than 3 distinct points are given (design.md decision 3's `n < 3` case)
- [ ] 3.4 Unit tests for 3.3: a known small dataset with a hand-computed expected SE; exactly 2
      points returns `null`; exactly 3 points returns a defined (non-null) value; residuals of zero
      (perfectly linear input) do not throw or divide by zero unexpectedly for `n >= 3`
- [ ] 3.5 Add `computeLoggingGap(...)` taking: the outlier-filtered/EMA'd trend series with its
      28-day regression `{slope, intercept}` and SE, the Nutrition Target `calories`, and the
      per-day `{state, calories}` window data (from `food-day-completeness` + Daily Totals) —
      returns a discriminated union: `{kind: 'not_enough_data'}` (hard floor per design.md decision
      5, or `n < 3` from 3.3), `{kind: 'gap', value: number, interval: number}` when
      `abs(value) >= interval`, or `{kind: 'not_enough_data'}` again when `abs(value) < interval`
      (statistical silence — same tag, both paths produce it per the spec's single surface state)
- [ ] 3.6 Unit tests for 3.5: fewer than 2 weigh-ins → `not_enough_data`; zero valid
      (complete/confirmed_complete) days → `not_enough_data`; a gap whose interval covers zero →
      `not_enough_data`; a gap clearly outside its interval → `gap` with the expected value/interval
      (mirror the spec's `sqrt(275^2 + 195^2)` example); days with `unconfirmed`/`incomplete` state
      are excluded from the Mean Logged Intake average, not counted as zero
- [ ] 3.7 Add an `excludedOutlierCount` (or boolean) output alongside `computeLoggingGap`'s result
      so the card can render the outlier note (spec's "Outlier note" scenarios) regardless of which
      of the three content states is showing

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
      changes), fetch the 28-day-plus-lead-in `weight` series (`api.data('weight', ...)`, reusing
      the existing lead-in pattern from the weight Trend Projection code in
      `frontend/app/data/[type]/DataTypeClient.tsx`), `api.getNutritionTarget()`,
      `api.getCompleteness(from, to)`, and `api.getFoodDailyTotals(from, to)`; run them through
      task 3's `loggingGap.ts` functions (reusing `emaSeries`/`linearRegression` from
      `frontend/lib/dataTypeMeta.ts` per design.md decision 4)
- [ ] 5.2 Render the three content states per the spec: a kcal/day range (never a bare point
      estimate) for `{kind: 'gap', ...}`; "not enough data yet" for `{kind: 'not_enough_data'}`;
      and catch `NutritionTargetUnmetError` from `getNutritionTarget()` to render the distinct
      "complete your profile/goal weight" state (design.md decision 6), linking to the relevant
      profile/goal-weight settings location
- [ ] 5.3 Render the outlier-excluded note (task 3.7's output) alongside whichever of the three
      states is showing, and static caveat copy about photo-estimated intake bias (spec's "Logging
      Gap Card content and placement" requirement)
- [ ] 5.4 Wire the card into `app/page.tsx`'s dashboard render alongside the vitals grid, respecting
      the task-4 registry's order/visibility for the `'logging_gap'` entry

## 6. i18n

- [ ] 6.1 Add English strings for the card's title, the three content states, the outlier note, and
      the photo-estimation caveat to `frontend/lib/i18n/en.ts`
- [ ] 6.2 Add the corresponding Russian strings to `frontend/lib/i18n/ru.ts`

## 7. Docs

- [ ] 7.1 `CONTEXT.md`: add glossary entries for **Implied Intake**, **Logging Gap**, and **Trend
      Weight** (Dashboard/Nutrition-targets section), and update the existing **Food Card** entry's
      example if needed to reference the Logging Gap Card as a concrete instance
- [ ] 7.2 New `docs/adr/ADR-008-<slug>.md` (`Status: Proposed`) recording: reporting a Logging Gap
      instead of a TDEE (and why — no third expenditure source exists in the data, per idea #10's
      grilling); silence-over-a-clamp as the safety mechanism; the rate-based 2 kg/day outlier rule;
      the quadrature interval formula and its two fixed/derived error terms; the card registry's
      Food Card generalization
- [ ] 7.3 `todo.md`: record idea #10 as shipped by this change instead of adaptive TDEE — note the
      framing change explicitly (not a TDEE, doesn't touch ADR-006) so a future reader doesn't
      re-open the original "replaces the activity multiplier" framing as if it were still live

## 8. E2E

- [ ] 8.1 Extend or add an `e2e/tests/` spec: seed weight + confirmed meals producing a clearly
      out-of-interval Logging Gap, load the dashboard, assert the card shows a range (not a bare
      number) and the photo-estimation caveat
- [ ] 8.2 E2E coverage for the "not enough data yet" state with a freshly seeded user (no weight,
      no meals)
- [ ] 8.3 E2E coverage for the "complete your profile/goal weight" state with a user who has weight
      and food history but no goal weight set
- [ ] 8.4 E2E coverage for hiding/reordering the Logging Gap Card via Edit mode and confirming the
      change persists across a reload, alongside existing Vital Cards
- [ ] 8.5 Run the full suite against `hcw-wip`

## 9. Verification

- [ ] 9.1 `make lint`
- [ ] 9.2 `make test`
- [ ] 9.3 `npx tsc --noEmit` in `frontend/`
- [ ] 9.4 `openspec validate logging-gap --strict`
