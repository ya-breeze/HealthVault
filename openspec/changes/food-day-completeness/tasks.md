## 1. Backend: settings keys + local-day helpers

- [x] 1.1 Document `timezone` and `usual_meals_per_day` as recognized (but unvalidated-at-write)
      keys on the `UserSettings` JSON blob, alongside the existing `dashboard_order`/
      `display_language` comment in `backend/pkg/database/models.go` (or wherever the Go-side
      settings-reading helpers already live)
- [x] 1.2 Add a `resolveTimezone(settingsJSON string) *time.Location` helper: parses `timezone`,
      falls back to `time.UTC` on missing/empty/`time.LoadLocation` error — never returns an error
      itself
- [x] 1.3 Add a `resolveUsualMealsPerDay(settingsJSON string) int` helper: parses
      `usual_meals_per_day`, falls back to `3` on missing/non-positive/non-integer
- [x] 1.4 Add a `localDate(t time.Time, loc *time.Location) string` helper returning `YYYY-MM-DD`
- [x] 1.5 Unit tests for 1.2-1.4: missing key, empty string, invalid zone name, valid zone name
      (e.g. `America/Los_Angeles` shifting a UTC timestamp across a day boundary), missing/zero/
      negative/non-integer `usual_meals_per_day`
- [x] 1.6 In the `PUT /api/users/me/settings` handler, compare the incoming `timezone` value
      against the previously stored one; if it differs, delete all of that user's
      `FoodDayCompletion` rows in the same write, using a hard (`Unscoped()`) delete — same
      requirement and same reason as the retract-confirmation delete in task 5.2 (see design.md §4
      "Storage": `FoodDayCompletion` embeds `TenantModel`'s `DeletedAt`, and the
      `(UserID, LocalDate)` unique index has no `deleted_at` clause, so a plain `Delete()` here
      would soft-delete every row and permanently block re-confirming any of those dates — the
      `CustomFood` trap (`food_custom.go`, `DeleteCustomFood`) repeated at a second call site).
      Wrap the settings row write and this cascade delete in a single `.Transaction(...)` call so
      a mid-write failure cannot leave a confirmation tied to a `timezone` the settings row no
      longer has.
- [x] 1.7 Tests for 1.6: writing an unchanged `timezone` leaves existing confirmations intact;
      writing a different `timezone` deletes all of the caller's confirmations and none of another
      user's; writing settings with no `timezone` key present (unchanged) leaves confirmations
      intact; confirming a date, changing `timezone`, then confirming that same date string again
      succeeds (200/201, not a unique-constraint violation) — proves the cleanup used `Unscoped()`
      and didn't leave a soft-deleted row occupying the `(user, date)` slot

## 2. Backend: Eating Occasion collapsing

- [x] 2.1 Add a pure function `collapseOccasions(loggedAt []time.Time) int` (or returning the
      grouped slices, whichever the completeness computation in task 3 needs): sort ascending,
      new group when the gap to the previous timestamp exceeds 10 minutes
- [x] 2.2 Unit tests: empty input (0 occasions), single timestamp (1), two timestamps 3 minutes
      apart (1), two timestamps exactly 10 minutes apart (1 — inclusive boundary per design.md),
      two timestamps 10 minutes and 1 second apart (2), the doc's own three-row trap case
      (09:1x/14:39/14:42 → 2), unsorted input (function must sort internally)

## 3. Backend: `FoodDayCompletion` storage + day-completeness computation

- [x] 3.1 Add `FoodDayCompletion` struct to `backend/pkg/database/models_food.go` per design.md
      (`UserID`, `LocalDate` unique per user, `ConfirmedAt`)
- [x] 3.2 Add `&FoodDayCompletion{}` to the `AutoMigrate(...)` list in `backend/pkg/database/db.go`
- [x] 3.3 Add a `computeDayState(occasionCount int, threshold int, confirmed bool) string` pure
      function returning one of `complete`/`confirmed_complete`/`unconfirmed`/`incomplete` per the
      table in design.md §3
- [x] 3.4 Add a function computing, for a user and an inclusive Logged-Day date range (already
      clamped to exclude today), one `{date, occasion_count, state}` entry per day — querying
      `FoodMeal` rows for the range, grouping by Logged Day via the task-1 timezone helper,
      collapsing occasions via task 2, and checking `FoodDayCompletion` for a confirmation per day.
      If a day has 0 occasions but a confirmation row still exists for it (e.g. every meal on that
      day was since deleted, or moved off that date via a `logged_at` edit), hard-delete
      (`Unscoped()`) that stale row as part of this computation, same reasoning as the retract
      delete in task 5.2, so a later unrelated meal on that date doesn't silently inherit the old
      confirmation
- [x] 3.5 Unit tests: a day with 0 meals (incomplete), a day at/above threshold (complete,
      regardless of any stray confirmation row for it), a day below threshold with no confirmation
      (unconfirmed), a day below threshold with a confirmation (confirmed_complete), a range
      spanning a threshold change mid-way (confirms task 1.3/3.3 recompute per call, not cached), a
      day with 0 occasions that still has a stale confirmation row (state computes as `incomplete`
      and the stale row is deleted as a side effect)

## 4. Backend: completeness range-query endpoint

- [x] 4.1 Add `GET /api/food/completeness` handler (new file alongside
      `backend/pkg/server/food_meal_detail.go`, e.g. `food_completeness.go`): auth check (401),
      parse `from`/`to` (400 on missing/malformed), clamp `to` to yesterday in the caller's zone
      *first*, then validate `from > to` and the >92-day span against the (possibly clamped) `to`
      (400 on either) — this order matters: a `from` naming today or a future date must fail the
      `from > to` check post-clamp, not resolve to an inverted/empty range — call task 3.4's range
      function, return the JSON array
- [x] 4.2 Register the route in `backend/pkg/server/server.go` (`GET /food/completeness`)
- [x] 4.3 Tests: full happy path against seeded meals across several days including a zero-meal
      day, `to` clamping when `to` names today or a future date, `from` equal to today with `to`
      also today (400, post-clamp inversion caught), each other 400 case, 401 with no auth,
      confirms no `?user=` override is honored (family member's data never leaks in)

## 5. Backend: day-confirmation endpoints

- [x] 5.1 Add `POST /api/food/completeness/{date}/confirm` handler: auth check (401), parse/
      validate `{date}` (400 on malformed / today-or-future), compute current state, 400 if
      `incomplete` or `complete`, upsert-idempotent `FoodDayCompletion` row (200 if already
      `confirmed_complete`, 201 with the new row otherwise)
- [x] 5.2 Add `DELETE /api/food/completeness/{date}/confirm` handler: auth check (401), parse/
      validate `{date}` (400 on malformed / today-or-future), `Unscoped().Delete()` any existing
      row for `(user, date)` — a plain `Delete()` soft-deletes and would permanently block
      re-confirming that date via the unique index, mirroring `DeleteCustomFood`'s existing
      `Unscoped()` usage — always 204
- [x] 5.3 Register both routes in `backend/pkg/server/server.go`
- [x] 5.4 Tests: confirm an eligible unconfirmed day (201), re-confirm an already-confirmed day
      (200, no duplicate row), confirm a zero-occasion day (400), confirm an already-complete day
      (400), confirm/delete against today or a future date (400 both), delete an existing
      confirmation (204, state reverts correctly), delete a non-existent confirmation (204, no
      error), 401 on both endpoints with no auth, confirms no `?user=` override is honored,
      **confirm → retract → confirm the same date again succeeds** (guards against the soft-delete/
      unique-index trap in 5.2)

## 6. Frontend: settings keys + API client

- [x] 6.1 Add `timezone?: string` and `usual_meals_per_day?: number` to the `UserSettings`
      interface in `frontend/lib/api.ts`, documented the same way as the existing keys
- [x] 6.2 Add `api.getCompleteness(from, to)` (`GET /food/completeness`) and
      `api.confirmDay(date)` / `api.unconfirmDay(date)` (`POST`/`DELETE
      /food/completeness/{date}/confirm`) to `frontend/lib/api.ts`

## 7. Frontend: timezone-aware day grouping on the history page

- [x] 7.1 Fetch the caller's settings on `frontend/app/food/history/page.tsx` load (or reuse
      whatever settings fetch already exists on this page) and resolve `timezone` (default
      `'UTC'`)
- [x] 7.2 Extract the day-grouping key computation into a small pure function in `frontend/lib/`
      (e.g. `loggedDayKey(d: Date, tz: string): string`), replacing the inline
      `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}` key (`page.tsx:44`) with
      `Intl.DateTimeFormat('en-CA', { timeZone: tz }).format(d)`, keeping the rest of the
      grouping/merge logic (including the "load older" merge behavior) unchanged
- [x] 7.3 Confirm day-section headers render the same `YYYY-MM-DD` grouping key consistently with
      what the backend's `GET /api/food/completeness` will return for the same day, so task 8's
      merge-by-date is a straightforward lookup
- [x] 7.4 Unit tests for 7.2's `loggedDayKey` (mirroring `dataTypeMeta.test.ts`'s pattern): default
      `UTC`, a zone that shifts the day (design.md's `America/Los_Angeles` /
      `2026-08-21T02:00:00Z` example), and a missing/invalid timezone falling back to `UTC`

## 8. Frontend: completeness badges + confirm/retract controls

- [x] 8.1 Fetch `GET /api/food/completeness` for the range covering the currently-loaded day
      sections (excluding the caller's current Logged Day) whenever the loaded range changes
      (initial load and each "load older"). Since "load older" has no depth limit, the loaded
      range can exceed the endpoint's 92-day cap — split it into consecutive ≤92-day windows,
      fetch each, and merge the results by date, rather than one call for the whole span
- [x] 8.2 Render, per day section: nothing extra for `complete`; a "Complete" badge plus an
      "unconfirm" control for `confirmed_complete`; a "Mark day complete" button for `unconfirmed`;
      nothing for the current Logged Day's own section
- [x] 8.3 Wire the confirm button to `api.confirmDay`, the unconfirm control to `api.unconfirmDay`,
      updating that day's local state from the response (or a refetch) on success, and surfacing a
      toast on failure without changing the displayed state
- [x] 8.4 Confirm a day with 0 meals never renders a day section at all (it's already excluded
      from the meal list itself, so this should require no new code) — add this as an assertion
      in the E2E history-page spec added under task 12, not a separate test file

## 9. Frontend: timezone / usual-meals-per-day settings panel

- [x] 9.1 Add a collapsed-by-default settings panel at the top of `/food/history` with a
      `timezone` `<select>` (options from `Intl.supportedValuesOf('timeZone')` when available,
      prefilled from the browser's own zone via
      `Intl.DateTimeFormat().resolvedOptions().timeZone` but not auto-saved) and a
      `usual_meals_per_day` number input (min 1, default 3)
- [x] 9.2 Save both fields through the existing `api.updateSettings` read-modify-write helper, not
      a raw `putSettings` call, so neither field can clobber `dashboard_order`/`display_language`
- [x] 9.3 On successful save, refetch the page's grouping (task 7) and completeness data (task 8)
      so the new setting takes effect without a full page reload

Test coverage for 9.1-9.3 is the E2E settings-panel scenario in task 12.4, which exercises 9.2's
read-modify-write correctness directly — the regression it guards against (clobbering
`dashboard_order`/`display_language`) is exactly what 9.2 was written to prevent.

## 10. i18n

- [x] 10.1 Add English strings for the completeness badges/controls and the settings panel's
      labels to `frontend/lib/i18n/en.ts`
- [x] 10.2 Add the corresponding Russian strings to `frontend/lib/i18n/ru.ts`

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
      confirm the control reverts (also assert a zero-meal day never renders a section, per 8.4)
- [ ] 12.2 E2E coverage for a day meeting the threshold showing no control at all
- [ ] 12.3 E2E coverage for the timezone-aware day grouping: set a non-UTC `timezone` via the
      settings panel (task 9) for a user with meals that straddle the UTC/zone day boundary, and
      assert the meals now group under the shifted day headers rather than UTC ones
- [ ] 12.4 E2E coverage for the settings panel (task 9) end to end: open it, change `timezone` and
      `usual_meals_per_day`, save, and confirm both (a) `dashboard_order`/`display_language` are
      unchanged afterward and (b) day grouping/completeness badges update without a page reload
- [ ] 12.5 Run the full suite against `hcw-wip`

## 13. Verification

- [ ] 13.1 `make lint`
- [ ] 13.2 `make test`
- [ ] 13.3 `npx tsc --noEmit` in `frontend/`
- [ ] 13.4 `openspec validate food-day-completeness --strict`
