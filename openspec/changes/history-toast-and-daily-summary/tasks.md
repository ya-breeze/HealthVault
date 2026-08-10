## 1. Backend: macro fields on meal summary

- [ ] 1.1 Add `ProteinGrams`, `CarbsGrams`, `FatGrams` (`float64`, `json:"protein_grams"`/`"carbs_grams"`/`"fat_grams"`) to the `MealSummary` struct in `backend/pkg/server/food_meal_detail.go`.
- [ ] 1.2 Populate the three new fields from `m.ProteinGrams`, `m.CarbsGrams`, `m.FatGrams` in `ListMeals`'s summary-building loop.
- [ ] 1.3 Update/add a Go test asserting `GET /api/food/meals` response JSON includes `protein_grams`, `carbs_grams`, `fat_grams` with the expected values for a confirmed meal.
- [ ] 1.4 Run `make lint` / `go vet` (per project convention) and the backend test suite for the `server` package; fix any failures.

## 2. Frontend: toast notification system

- [ ] 2.1 Create the toast context/provider (e.g. `frontend/components/Toast.tsx` or `frontend/lib/toast.tsx`): `ToastProvider` holding a list of `{ id, message, variant }`, a `useToast()` hook exposing `showToast(message, variant)`, auto-dismiss after ~3s per toast via timer, and manual dismiss.
- [ ] 2.2 Render the toast stack as a fixed-position element (e.g. bottom-center) that stacks multiple simultaneous toasts, styled consistently with the app's existing Tailwind light/dark conventions.
- [ ] 2.3 Mount `ToastProvider` in `frontend/app/layout.tsx` wrapping `children`.

## 3. Frontend: wire toasts into the food review mutation path

- [ ] 3.1 Extend `applyMealUpdate` in `frontend/app/food/review/ReviewClient.tsx` to accept an optional `label?: string` second parameter; on successful resolution call `showToast(label ?? 'Saved', 'success')`, on rejection call `showToast('Update failed', 'error')` (existing inline error handling is unchanged — this is additive).
- [ ] 3.2 Pass labels at each existing `applyMealUpdate` call site: retry ("Analysis retried" or similar), clarify ("Clarification submitted"), confirm ("Meal confirmed").
- [ ] 3.3 Update `MealMetaEditor`'s `onUpdated` usage/prop chain so its call passes a label ("Meal updated").
- [ ] 3.4 Update `MealItemRow`'s `onUpdated` calls to pass distinct labels for weight edit, rebind ("Item updated"), and delete ("Item removed") — see the four `onUpdated(...)` call sites at `frontend/components/food/MealItemRow.tsx:65,84,106,119,127` (weight commit, refetch-after-bind-race, rebind, manual macro edit, delete).
- [ ] 3.5 Update `AddItemForm`'s `onAdded` call to pass a label ("Item added").
- [ ] 3.6 Update `ReanalyzeControl`'s `onReanalyzed` call to pass a label ("Reanalysis complete").
- [ ] 3.7 Manually exercise the review page (add item, edit weight, delete item, edit meal name/date, reanalyze, confirm) against the WIP stack and confirm a toast appears for each, with existing inline errors still intact on induced failures.

## 4. Frontend: day-grouped history with per-day totals

- [ ] 4.1 Add `protein_grams`, `carbs_grams`, `fat_grams` (`number`) to the `MealSummary` interface in `frontend/lib/api.ts`.
- [ ] 4.2 In `frontend/app/food/history/page.tsx`, add a pure grouping function that takes the flat `meals` array (already sorted `logged_at DESC`) and returns an ordered list of `{ dateKey, meals[] }` buckets keyed by local calendar date, plus a per-bucket total (`{ calories, protein, carbs, fat }`) summed over that bucket's `status === 'confirmed'` meals only (zero if none).
- [ ] 4.3 Update the render to iterate day buckets: a day header (e.g. formatted local date) with the day's total line, followed by that day's meal rows in the existing per-row format/styling.
- [ ] 4.4 Verify "load older" (which appends to the flat `meals` state) re-runs the grouping function over the full accumulated list, so newly loaded meals merge into existing day buckets or create new ones, and totals stay correct — no special-casing needed if grouping is recomputed from the full `meals` array on every render.
- [ ] 4.5 Confirm a day with zero confirmed meals still renders a zero total line rather than omitting it, per spec.

## 5. Tests

- [ ] 5.1 Extend `e2e/tests/food.spec.ts`'s `Meal history` describe block: a test asserting meals from two different days render under two separate day headers with correct per-day totals (mocked meal list, deterministic).
- [ ] 5.2 Add/extend a test asserting a day containing only a non-confirmed meal shows a zero total.
- [ ] 5.3 Add/extend a test covering "Load older" merging into an existing day section and updating its total (extends the existing `"Load older" fetches and appends a real second page` test or adds a sibling).
- [ ] 5.4 Add an e2e assertion that a toast appears after at least one representative mutation (e.g. adding an item) on the review page — extends `e2e/tests/food.spec.ts`'s existing add/edit/delete coverage rather than adding a new top-level describe block.
- [ ] 5.5 Run the full `e2e/tests/food.spec.ts` suite against the deployed `healthvault-wip` stack (per project E2E convention) and fix any failures.

## 6. Wrap-up

- [ ] 6.1 Run backend and frontend static checks (`make lint`, `tsc --noEmit`, or project equivalents) and fix any issues.
- [ ] 6.2 Update the OpenSpec change status / confirm `openspec validate --strict` passes for the full repo (not just this change) once specs are folded in at archive time.
