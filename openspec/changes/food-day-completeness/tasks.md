## 1. Backend: settings keys + local-day helpers

- [ ] 1.1 Document `timezone` and `usual_meals_per_day` as recognized (but unvalidated-at-write)
      keys on the `UserSettings` JSON blob, alongside the existing `dashboard_order`/
      `display_language` comment in `backend/pkg/database/models.go` (or wherever the Go-side
      settings-reading helpers already live)
- [ ] 1.2 Add a `resolveTimezone(settingsJSON string) *time.Location` helper: parses `timezone`,
      falls back to `time.UTC` on missing/empty/`time.LoadLocation` error — never returns an error
      itself
- [ ] 1.3 Add a `resolveUsualMealsPerDay(settingsJSON string) int` helper: parses
      `usual_meals_per_day`, falls back to `3` on missing/non-positive/non-integer
- [ ] 1.4 Add a `localDate(t time.Time, loc *time.Location) string` helper returning `YYYY-MM-DD`
- [ ] 1.5 Unit tests for 1.2-1.4: missing key, empty string, invalid zone name, valid zone name
      (e.g. `America/Los_Angeles` shifting a UTC timestamp across a day boundary), missing/zero/
      negative/non-integer `usual_meals_per_day`

## 2. Backend: Eating Occasion collapsing

- [ ] 2.1 Add a pure function `collapseOccasions(loggedAt []time.Time) int` (or returning the
      grouped slices, whichever the completeness computation in task 3 needs): sort ascending,
      new group when the gap to the previous timestamp exceeds 10 minutes
- [ ] 2.2 Unit tests: empty input (0 occasions), single timestamp (1), two timestamps 3 minutes
      apart (1), two timestamps exactly 10 minutes apart (1 — inclusive boundary per design.md),
      two timestamps 10 minutes and 1 second apart (2), the doc's own three-row trap case
      (09:1x/14:39/14:42 → 2), unsorted input (function must sort internally)

## 3. Backend: `FoodDayCompletion` storage + day-completeness computation

- [ ] 3.1 Add `FoodDayCompletion` struct to `backend/pkg/database/models_food.go` per design.md
      (`UserID`, `LocalDate` unique per user, `ConfirmedAt`)
- [ ] 3.2 Add `&FoodDayCompletion{}` to the `AutoMigrate(...)` list in `backend/pkg/database/db.go`
- [ ] 3.3 Add a `computeDayState(occasionCount int, threshold int, confirmed bool) string` pure
      function returning one of `complete`/`confirmed_complete`/`unconfirmed`/`incomplete` per the
      table in design.md §3
- [ ] 3.4 Add a function computing, for a user and an inclusive Logged-Day date range (already
      clamped to exclude today), one `{date, occasion_count, state}` entry per day — querying
      `FoodMeal` rows for the range, grouping by Logged Day via the task-1 timezone helper,
      collapsing occasions via task 2, and checking `FoodDayCompletion` for a confirmation per day
- [ ] 3.5 Unit tests: a day with 0 meals (incomplete), a day at/above threshold (complete,
      regardless of any stray confirmation row for it), a day below threshold with no confirmation
      (unconfirmed), a day below threshold with a confirmation (confirmed_complete), a range
      spanning a threshold change mid-way (confirms task 1.3/3.3 recompute per call, not cached)

## 4. Backend: completeness range-query endpoint

- [ ] 4.1 Add `GET /api/food/completeness` handler (new file alongside
      `backend/pkg/server/food_meal_detail.go`, e.g. `food_completeness.go`): auth check (401),
      parse/validate `from`/`to` (400 on missing/malformed/`from > to`/>92-day span), clamp `to`
      to yesterday in the caller's zone, call task 3.4's range function, return the JSON array
- [ ] 4.2 Register the route in `backend/pkg/server/server.go` (`GET /food/completeness`)
- [ ] 4.3 Tests: full happy path against seeded meals across several days including a zero-meal
      day, `to` clamping when `to` names today or a future date, each 400 case, 401 with no auth,
      confirms no `?user=` override is honored (family member's data never leaks in)

## 5. Backend: day-confirmation endpoints

- [ ] 5.1 Add `POST /api/food/completeness/{date}/confirm` handler: auth check (401), parse/
      validate `{date}` (400 on malformed / today-or-future), compute current state, 400 if
      `incomplete` or `complete`, upsert-idempotent `FoodDayCompletion` row (200 if already
      `confirmed_complete`, 201 with the new row otherwise)
- [ ] 5.2 Add `DELETE /api/food/completeness/{date}/confirm` handler: auth check (401), parse/
      validate `{date}` (400 on malformed / today-or-future), delete any existing row for
      `(user, date)`, always 204
- [ ] 5.3 Register both routes in `backend/pkg/server/server.go`
- [ ] 5.4 Tests: confirm an eligible unconfirmed day (201), re-confirm an already-confirmed day
      (200, no duplicate row), confirm a zero-occasion day (400), confirm an already-complete day
      (400), confirm/delete against today or a future date (400 both), delete an existing
      confirmation (204, state reverts correctly), delete a non-existent confirmation (204, no
      error), 401 on both endpoints with no auth, confirms no `?user=` override is honored

## 6. Frontend: settings keys + API client

- [ ] 6.1 Add `timezone?: string` and `usual_meals_per_day?: number` to the `UserSettings`
      interface in `frontend/lib/api.ts`, documented the same way as the existing keys
- [ ] 6.2 Add `api.getCompleteness(from, to)` (`GET /food/completeness`) and
      `api.confirmDay(date)` / `api.unconfirmDay(date)` (`POST`/`DELETE
      /food/completeness/{date}/confirm`) to `frontend/lib/api.ts`

## 7. Frontend: timezone-aware day grouping on the history page

- [ ] 7.1 Fetch the caller's settings on `frontend/app/food/history/page.tsx` load (or reuse
      whatever settings fetch already exists on this page) and resolve `timezone` (default
      `'UTC'`)
- [ ] 7.2 Replace the `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}` grouping key
      (`page.tsx:44`) with `Intl.DateTimeFormat('en-CA', { timeZone: tz }).format(d)`, keeping the
      rest of the grouping/merge logic (including the "load older" merge behavior) unchanged
- [ ] 7.3 Confirm day-section headers render the same `YYYY-MM-DD` grouping key consistently with
      what the backend's `GET /api/food/completeness` will return for the same day, so task 8's
      merge-by-date is a straightforward lookup

## 8. Frontend: completeness badges + confirm/retract controls

- [ ] 8.1 Fetch `GET /api/food/completeness` for the range covering the currently-loaded day
      sections (excluding the caller's current Logged Day) whenever the loaded range changes
      (initial load and each "load older")
- [ ] 8.2 Render, per day section: nothing extra for `complete`; a "Complete" badge plus an
      "unconfirm" control for `confirmed_complete`; a "Mark day complete" button for `unconfirmed`;
      nothing for the current Logged Day's own section
- [ ] 8.3 Wire the confirm button to `api.confirmDay`, the unconfirm control to `api.unconfirmDay`,
      updating that day's local state from the response (or a refetch) on success, and surfacing a
      toast on failure without changing the displayed state
- [ ] 8.4 Confirm a day with 0 meals never renders a day section at all (it's already excluded
      from the meal list itself, so this should require no new code, only a test)

## 9. Frontend: timezone / usual-meals-per-day settings panel

- [ ] 9.1 Add a collapsed-by-default settings panel at the top of `/food/history` with a
      `timezone` `<select>` (options from `Intl.supportedValuesOf('timeZone')` when available,
      prefilled from the browser's own zone via
      `Intl.DateTimeFormat().resolvedOptions().timeZone` but not auto-saved) and a
      `usual_meals_per_day` number input (min 1, default 3)
- [ ] 9.2 Save both fields through the existing `api.updateSettings` read-modify-write helper, not
      a raw `putSettings` call, so neither field can clobber `dashboard_order`/`display_language`
- [ ] 9.3 On successful save, refetch the page's grouping (task 7) and completeness data (task 8)
      so the new setting takes effect without a full page reload

## 10. i18n

- [ ] 10.1 Add English strings for the completeness badges/controls and the settings panel's
      labels to `frontend/lib/i18n/en.ts`
- [ ] 10.2 Add the corresponding Russian strings to `frontend/lib/i18n/ru.ts`

## 11. Docs

- [ ] 11.1 `CONTEXT.md`: add glossary entries for **Eating Occasion**, **Logged Day**, **Usual
      Meals Per Day**, and **Day Completeness** (the four states), per the "Vocabulary introduced"
      section of the idea-forge grilling comment
- [ ] 11.2 New `docs/adr/ADR-006-<slug>.md` (`Status: Proposed`) recording: Day Completeness as a
      user assertion gated by a heuristic that only decides when to ask; Eating Occasion
      collapsing (10-minute window) over raw meal-count; a dedicated table instead of a metric
      type; the settled open questions (no override of an automatic Complete day; 3-of-7 minimum
      downstream coverage; threshold changes recompute past days on read)
- [ ] 11.3 `todo.md`: record this change as claimed/shipped in the dashboard/food-tracking
      initiative section, referencing this change and ADR-006

## 12. E2E

- [ ] 12.1 Extend or add an `e2e/tests/` spec: seed meals producing an `unconfirmed` day, confirm
      it via the history page's control, reload, confirm the badge persists; retract it and
      confirm the control reverts
- [ ] 12.2 E2E coverage for a day meeting the threshold showing no control at all
- [ ] 12.3 Run the full suite against `hcw-wip`

## 13. Verification

- [ ] 13.1 `make lint`
- [ ] 13.2 `make test`
- [ ] 13.3 `npx tsc --noEmit` in `frontend/`
- [ ] 13.4 `openspec validate food-day-completeness --strict`
