## 1. Backend — status filter on GET /api/food/meals

- [x] 1.1 Add `status` query parsing to `ListMeals` (`backend/pkg/server/food_meal_detail.go`): accept zero or more repeated `status` values via `r.URL.Query()["status"]`, validate each against the five `database.MealStatus*` constants, return HTTP 400 on any unrecognized value.
- [x] 1.2 When one or more valid `status` values are supplied, add a `status IN (?)` clause to the existing GORM query, combined with the existing `user_id`, ordering, and keyset-cursor `WHERE` clauses (`before`/`before_id` still apply unchanged).
- [x] 1.3 Update `ListMeals`'s doc comment to describe the new `status` parameter.

## 2. Backend — needs-attention count endpoint

- [x] 2.1 Add a `needsAttentionCountResponse{ Count int }` type and a `NeedsAttentionCount` handler in `backend/pkg/server/food_meal_detail.go`: authenticate via `ClaimsFromCtx`, `SELECT COUNT(*)` on `FoodMeal` scoped to `user_id` and `status IN (processing, pending_clarification, pending_review, failed)`, write `{"count": <n>}` via `writeJSON`.
- [x] 2.2 Register the route in `backend/pkg/server/server.go`: `GET /food/meals/needs-attention-count` — register it before the `/food/meals/{id}` routes so gorilla/mux doesn't try to match `needs-attention-count` as a UUID path segment.

## 3. Backend tests

- [x] 3.1 Unit/integration tests for `status` filtering: single status, multiple statuses, invalid status (400), status filter combined with keyset paging, status omitted behaves as before.
- [x] 3.2 Unit/integration tests for the count endpoint: mixed statuses gives the right count, zero when none/all-confirmed, scoped to caller only (not other family members), 401 when unauthenticated.

## 4. Frontend — API client

- [x] 4.1 Extend `listMeals` in `frontend/lib/api.ts` to accept an optional `status?: string[]` option, serialized as repeated `status=` query params.
- [x] 4.2 Add a `needsAttentionCount(): Promise<{ count: number }>` call to `frontend/lib/api.ts` hitting `GET /food/meals/needs-attention-count`.

## 5. Frontend — dashboard indicator

- [x] 5.1 In `frontend/app/page.tsx`, fetch the needs-attention count alongside the existing vitals fetch (on mount, once `ready`).
- [x] 5.2 Render a compact indicator (count + short label, e.g. "3 meals need attention") above the "Log food" section when count > 0; render nothing when count is 0 or not yet loaded.
- [x] 5.3 Make the indicator a link to `/food/history/`, styled consistently with the existing dashboard sections (`bg-bg-elevated`/`border-border`/`hover:border-accent` pattern used elsewhere on this page).

## 6. Verification

- [x] 6.1 `cd backend && go build ./... && go vet ./...` and run the new/updated tests.
- [x] 6.2 `cd frontend && npm run lint` and `npx tsc --noEmit`. (No `lint` script in this project's `package.json` — `npx tsc --noEmit` and `next build` both pass clean.)
- [ ] 6.3 Deploy to `hcw-wip`, run the Playwright suite (`dashboard.spec.ts`, `food.spec.ts`) against it, and add a new e2e case covering the indicator appearing/disappearing based on meal status.
- [ ] 6.4 Manually verify at a 375px viewport: indicator renders correctly, doesn't crowd the vitals grid or "Log food" row, link navigates to history.
