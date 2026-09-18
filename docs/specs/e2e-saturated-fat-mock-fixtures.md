# Fix stale E2E mock fixtures missing saturated_fat_grams

## Why

`MealItemRow.tsx:235` reads `item.saturated_fat_grams.toFixed(1)`, unguarded — the same pattern
already used for `sodium_grams`/`dietary_fiber_grams` two lines above it. That's correct in
production: `FoodItem.SaturatedFatGrams` is `NOT NULL DEFAULT 0` on the backend (PR #80, merged
today), so the real API never omits it. But several E2E spec files build mock `FoodMeal`/
`FoodItem` JSON objects by hand for Playwright route interception, and those mocks predate the
saturated-fat field. With `item.saturated_fat_grams` `undefined`, `.toFixed(1)` throws, the
component (and everything nested under it) fails to render, and every assertion after that point
fails with unrelated-looking errors like `getByText('вареники')` timing out — the page never
painted at all. Confirmed live: 26 of `food.spec.ts`'s tests failed this way against `hcw-wip`
before this fix, all clustered in the `test.describe` blocks that use `mockFoodMeal()`.

PR #80 validated only `e2e/tests/logging-gap.spec.ts` before merging — that file already had
`saturated_fat_grams` added to its own mocks, so this gap in every other spec file went
undetected. Root-caused while validating an unrelated change (`ya-breeze/idea-forge#640`) in a
different worktree; fixed here as its own small, separate change.

## How

Test-fixture fix only — no production code change. `MealItemRow.tsx` is correct as written; the
bug is stale test data, not a missing null-guard (a guard would paper over data that should never
be absent from a real response, and would mask a genuine future contract break instead of failing
loudly the way it just did).

- `e2e/tests/mobile-tap-targets.spec.ts` and `e2e/tests/food.spec.ts` each declare their own
  identical `mockFoodMeal()` helper (not shared) — add `saturated_fat_grams` to both the
  meal-level and item-level object literals in each.
- `e2e/tests/food.spec.ts`: beyond the helper, two tests assert the exact JSON body of a manual-
  edit PATCH request via `.toEqual({...})` (`ManualItemEditor` always submits all 8 macro fields,
  not just the ones touched). Both mocked items inherit `saturated_fat_grams: 1` from the fixed
  helper and never touch it, so both expected-body objects get `saturated_fat_grams: 1` added.
  A third meal-level mock (`reanalyzed`, the "successful response updates the displayed meal"
  test) explicitly zeroes every other macro to simulate a fresh reanalysis result — added
  `saturated_fat_grams: 0` there too for consistency, though no assertion in that test reads it.
- `e2e/tests/mobile-nav.spec.ts`: one hand-built item (`mockPendingReviewMeal`, used by the nav-bar
  tests) renders through the real review page and needed the field added to both its item and meal
  objects. The other two `dietary_fiber_grams` mocks in this file use `items: []` — nothing for
  `MealItemRow` to render, so they were already safe and are untouched.
- `e2e/tests/data-types.spec.ts` has two unrelated `dietary_fiber_grams` mocks: one is a
  `food_meal` record consumed only by the generic `/data/[type]` table, whose cell formatter
  (`DataTypeClient.tsx`, `Number(v ?? 0)`) already coerces a missing value to 0 rather than
  crashing — confirmed safe, left alone. The other is the wearable-ingestion `nutrition` type
  (`models.go`'s unrelated `Nutrition` struct), which PR #80's own spec explicitly scoped
  saturated fat out of (`NUTRITION_MACROS` drives only that chart, never `food_meal`) — correctly
  untouched.

## Validation Commands

- `make lint`
- `make test`
- `BASE_URL=http://192.168.1.54:8892 make test-e2e E2E_ARGS="tests/food.spec.ts tests/mobile-tap-targets.spec.ts tests/mobile-nav.spec.ts tests/data-types.spec.ts --retries=0"`

### Task 1: Fix stale mock fixtures

- [x] Add `saturated_fat_grams` to `mobile-tap-targets.spec.ts`'s `mockFoodMeal()` helper
- [x] Add `saturated_fat_grams` to `food.spec.ts`'s `mockFoodMeal()` helper, both PATCH-body
      `.toEqual` assertions, and the reanalyzed-meal zero-fixture
- [x] Add `saturated_fat_grams` to `mobile-nav.spec.ts`'s `mockPendingReviewMeal` item and meal
- [x] Confirm `data-types.spec.ts`'s two `dietary_fiber_grams` mocks need no change (generic
      table formatter is null-safe; the wearable `nutrition` type is out of scope by design)
- [x] Re-run the four affected spec files against `hcw-wip` and confirm zero failures
- [x] Mark completed
