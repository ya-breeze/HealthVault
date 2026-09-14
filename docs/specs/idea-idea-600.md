# Food logging history card
Idea: ya-breeze/idea-forge#600

## Why

Day Completeness already records whether each completed Logged Day is Complete, Confirmed Complete, Unconfirmed, or Incomplete, but the user can see those states only one day at a time on `frontend/app/food/history/page.tsx`. The dashboard’s Nutrition card consumes the same history indirectly through `isValidDay` in `frontend/lib/loggingGap.ts`; when too few days qualify, it can only say that there is not enough data. It does not show which part of the logging habit caused that result.

The existing APIs already provide the evidence needed for an honest summary. `GET /api/food/completeness` returns one state and Eating Occasion count per closed Logged Day, while `GET /api/food/daily-totals` reports `unconfirmed_meals`, which distinguishes a complete-looking day from one whose nutrition is still unresolved. Adding a dashboard card now makes the shipped completeness model visible without inventing another heuristic or persistence model.

The card should answer the user’s immediate questions over the same rolling seven-day horizon used by nutrition insights: how many days will actually count, how many have no food logged, and how many contain data but still need attention. This also exposes why a downstream insight is unavailable instead of leaving the user to infer it from a generic “not enough data” message.

## How

Add a new independently hideable and reorderable Food Card with registry id `food_log_history`, implemented in `frontend/components/FoodLogHistoryCard.tsx`. It covers the seven completed Logged Days ending yesterday in the stored `timezone`; today is excluded because `database.DayRange` deliberately does not compute Day Completeness for an in-progress day. The parent dashboard already loads the timezone and passes it to `LoggingGapCard`, so it will pass the same value to this card rather than issuing another settings request.

Create a framework-free helper in `frontend/lib/foodLogHistory.ts`. `resolveFoodLogHistoryWindow(now, timezone)` returns the inclusive seven-day `from` and `to` strings using `loggedDayKey` and UTC-safe calendar-date arithmetic. `summarizeFoodLogHistory(completeness, dailyTotals, expectedDates)` joins the two zero-filled API responses by date and produces seven chronological day results plus aggregate counts. It must reuse `isValidDay` from `frontend/lib/loggingGap.ts`, because “will count” means exactly what the existing Nutrition card and Healthiness Label count: Day Completeness is Complete or Confirmed Complete and `unconfirmed_meals === 0`.

The seven days form three exhaustive display outcomes. A `counted` day passes `isValidDay`. A `no_food` day has the `incomplete` completeness state and therefore zero Eating Occasions. Every other day is `needs_attention`: it either needs the user’s day-complete assertion, contains one or more non-confirmed Food Meals, or both. The card shows “days counted: X of 7”, “no food logged: Y”, and “need attention: Z”, plus an oldest-to-newest seven-day strip. Each day marker includes its weekday and a visible status symbol, with an accessible label containing the date, outcome, Eating Occasion count, and unresolved-meal count where applicable; colour alone must not communicate status.

Both API calls start in parallel and the card renders its own loading and retrieval-error states. A successful response must contain exactly one matching row from each endpoint for every requested date; a partial, duplicate, or mismatched response is treated as retrieval failure instead of silently classifying an absent row as a missed day. Outside dashboard edit mode the card links to `/food/history/`, where the existing `DayCompletenessControl` and meal review flows let the user resolve attention states. In edit mode it follows `VitalCard` and `LoggingGapCard` conventions for move and visibility controls and does not navigate.

Extend `CardId` in `frontend/lib/vitals.ts` to include `food_log_history`. Replace the current one-off `logging_gap` checks with a narrow shared predicate for non-`DataType` Food Cards: both Food Card ids are always presence-eligible, neither is sent through `api.data` by `fetchPrimaryVitals`, and `secondaryTypes` continues to operate only on real `DataType` values. Add the new id to `PRIMARY_METRICS`; saved `dashboard_order` values that predate it retain their existing order and receive the new card appended visible, matching `reconcileMetricOrder`’s established migration convention.

This change deliberately does not alter Day Completeness, Eating Occasion collapsing, Usual Meals Per Day, confirmation storage, or either backend endpoint. It does not estimate a number of missed meals: a below-threshold day cannot reveal whether the user forgot food or genuinely ate fewer times, which is why ADR-007 made completeness a heuristic-gated user assertion. It also excludes streaks, configurable periods, direct confirmation inside the card, notifications, and changes to the existing Nutrition card. No deployment, public-hostname, Cloudflare Access, `dogfood`, `prod`, or credential work is part of this change.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e`

### Task 1: Define the seven-day logging-history model
- [ ] Add `frontend/lib/foodLogHistory.ts` with the `resolveFoodLogHistoryWindow` and `summarizeFoodLogHistory` pure functions and typed `counted`, `no_food`, and `needs_attention` day outcomes described in `## How`
- [ ] Reuse `loggedDayKey` for the stored-timezone boundary and `isValidDay` for downstream eligibility instead of duplicating either rule
- [ ] Validate that both endpoint results contain one row for every expected date and reject missing, duplicate, out-of-window, or mismatched rows
- [ ] Add `frontend/lib/foodLogHistory.test.ts` covering UTC and date-boundary timezones, chronological seven-day windows, all four Day Completeness states, unresolved Food Meals, overlapping attention reasons, aggregate counts, and malformed response joins
- [ ] Mark completed

### Task 2: Generalize the dashboard registry for another Food Card
- [ ] Extend `CardId` and `PRIMARY_METRICS` in `frontend/lib/vitals.ts` with `food_log_history`
- [ ] Add a narrow `DataType`-card predicate and use it from `frontend/app/page.tsx#fetchPrimaryVitals` so neither `logging_gap` nor `food_log_history` is requested through `api.data`
- [ ] Update `hasCardPresence` so both non-`DataType` Food Cards are always eligible while real `DataType` cards retain the existing fail-open Presence behavior
- [ ] Extend `frontend/lib/vitals.test.ts` for old saved orders, explicit reorder/hide persistence, unconditional Food Card presence, DataType filtering, and exclusion from `secondaryTypes`
- [ ] Mark completed

### Task 3: Build the Food logging history card
- [ ] Add `frontend/components/FoodLogHistoryCard.tsx` with local loading, ready, and retrieval-error states and parallel calls to `api.getCompleteness` and `api.getFoodDailyTotals`
- [ ] Render the counted, no-food, and needs-attention totals and a chronological seven-day status strip from the pure summary result
- [ ] Give every marker a visible non-colour status cue and an accessible label containing its date, outcome, Eating Occasion count, and applicable unresolved-meal information
- [ ] Link the read-only card to `/food/history/` and implement the same move, show/hide, disabled-control, hidden-card dimming, `data-hidden`, and test-id conventions used by `VitalCard` and `LoggingGapCard` in edit mode
- [ ] Treat either failed request or an invalid joined response as “temporarily unavailable” without presenting missing response rows as logging gaps
- [ ] Mark completed

### Task 4: Wire and localize the card
- [ ] Import and render `FoodLogHistoryCard` from `frontend/app/page.tsx` for the `food_log_history` registry branch, passing the already-loaded timezone and the existing reorder/visibility callbacks
- [ ] Add matching `foodLogHistory.*` keys to `frontend/lib/i18n/en.ts` and `frontend/lib/i18n/ru.ts` for the title, loading/error states, aggregate labels, three day outcomes, and accessible day descriptions
- [ ] Use count-safe Russian copy such as “Засчитано дней: X из 7” rather than introducing unhandled noun inflection
- [ ] Update relevant registry comments in `frontend/lib/vitals.ts`, `frontend/app/page.tsx`, and `CONTEXT.md` so they describe multiple non-`DataType` Food Cards rather than treating `logging_gap` as the only possible one
- [ ] Mark completed

### Task 5: Cover the dashboard behavior end to end
- [ ] Extend `e2e/tests/dashboard.spec.ts` or add a focused E2E spec that supplies deterministic completeness and daily-total responses and asserts the three summary counts and seven chronological markers
- [ ] Assert that a day with Complete or Confirmed Complete state but a nonzero `unconfirmed_meals` value is shown as needing attention and is not included in the counted total
- [ ] Assert that the card’s request window contains exactly the seven closed Logged Days in the stored timezone and excludes today
- [ ] Cover English and Russian copy, navigation to `/food/history/`, retrieval failure, and show/hide/reorder persistence using the dashboard’s existing cleanup conventions for shared settings
- [ ] Mark completed
