## Why

The backend already lets an owner permanently delete a meal (`DELETE /api/data/food_meal/{id}`, cascading to items and the stored photo — see `record-deletion` and `food-nutrition-logging`), but the only UI that can reach it is the generic `/data/food_meal` raw-data table, which is a debug-style view of every registered type rather than a food-specific screen a normal user would find. Someone who logs a meal by mistake, duplicates one, or wants to remove a bad photo entry has no way to delete it from the food-tracking UI itself (meal history or the meal review page).

## What Changes

- Add a "Delete meal" control to the meal review page (`/food/review/?meal={id}`), using the same trash-icon → Confirm/Cancel inline pattern already used for per-item deletion and the generic data-table row delete.
- On confirm, call the existing `DELETE /api/data/food_meal/{id}` endpoint (via the existing `api.deleteRecord` helper — no new API surface), show a success toast, and navigate back to `/food/history`.
- No backend change: the endpoint, its ownership check, and its item/photo cleanup are already implemented and specified.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `record-deletion`: adds a second UI entry point for the existing delete-record capability — a delete control on the meal review page, alongside the existing generic data-table row delete. Same endpoint, same confirm-then-delete interaction shape, different screen.

## Impact

- Frontend only: `frontend/app/food/review/ReviewClient.tsx` (new delete control + navigation on success), reusing `api.deleteRecord` from `frontend/lib/api.ts` (unchanged).
- No backend code changes.
- E2E: new Playwright case in `e2e/tests/food.spec.ts` covering create → open review → delete → verify removal from history.
