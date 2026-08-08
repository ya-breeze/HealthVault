## 1. Backend: meal history list

- [ ] 1.1 Add `GET /api/food/meals` handler (owner-scoped by `claims.UserID`, ordered by `logged_at` desc, `?limit=` default 50 / max 200) in `backend/pkg/server/food_meal_detail.go`
- [ ] 1.2 Register the route in `backend/pkg/server/server.go`
- [ ] 1.3 Unit tests: own-meals-only scoping, ordering, default/max limit, all statuses included, 401 unauthenticated

## 2. Backend: item add/delete + confirmed-meal item editing

- [ ] 2.1 In `backend/pkg/server/food_item.go`, change `PatchMealItem`'s status guard from "reject if confirmed" to "reject unless `pending_review` or `confirmed`"
- [ ] 2.2 Add `POST /api/food/meals/{id}/items` handler reusing `patchItemRequest`'s fields/precedence to create a new `FoodItem` (remember `TenantModel` has no `BeforeCreate` — set `ID`/`FamilyID`/`UserID`/`MealID` explicitly), same status guard as 2.1
- [ ] 2.3 Add `DELETE /api/food/meals/{id}/items/{item_id}` handler, same status guard, 404 for missing/unowned item
- [ ] 2.4 Register both new routes in `backend/pkg/server/server.go`
- [ ] 2.5 Factor a shared helper that, after any item write, recomputes and persists the meal's aggregate via `FoodMeal.Aggregate` **only when the meal's current status is `confirmed`** (leave `pending_review` behavior untouched); call it from patch, create, and delete handlers
- [ ] 2.6 Unit tests: patch/create/delete on a confirmed meal succeed and update the meal's stored aggregate; patch/create/delete on `processing`/`pending_clarification`/`failed` return 409; create/delete/patch on `pending_review` behave as before (no aggregate recompute); delete of nonexistent/unowned item returns 404

## 3. Backend: meal name/logged_at correction

- [ ] 3.1 Add `PATCH /api/food/meals/{id}` handler in `backend/pkg/server/food_meal_detail.go` accepting optional `name`/`logged_at`, requiring at least one, same `pending_review`/`confirmed` status guard, 409 otherwise
- [ ] 3.2 Register the route in `backend/pkg/server/server.go`
- [ ] 3.3 Unit tests: rename confirmed meal, correct logged_at, empty body → 400, wrong status → 409

## 4. Backend: vision hint plumbing

- [ ] 4.1 Add a `hint string` parameter to `vision.Client.Recognize` in `backend/pkg/vision/vision.go`
- [ ] 4.2 Update `OpenAIClient.Recognize` (`backend/pkg/vision/openai.go`) to append the hint to the prompt when non-empty, image still sent
- [ ] 4.3 Update `Fake.Recognize` and `Unconfigured.Recognize` to accept (and, for `Fake`, optionally assert on) the new parameter
- [ ] 4.4 Update all existing callers of `Recognize` (`analyzeMeal` in `backend/pkg/server/food_upload.go`, and anywhere else it's called, e.g. calibration) to pass an empty hint, preserving current behavior

## 5. Backend: reanalyze endpoint

- [ ] 5.1 Add `backend/pkg/server/food_reanalyze.go` with `POST /api/food/meals/{id}/reanalyze`: parse `{"hint": string}`, 400 if blank, 409 if no photo or status is `processing`/`pending_clarification`, otherwise call the same recognize → `processRecognition` → `persistAnalysis` pipeline `analyzeMeal` uses, passing the hint through
- [ ] 5.2 Register the route in `backend/pkg/server/server.go`
- [ ] 5.3 Unit tests: reanalyze from `failed`/`pending_review`/`confirmed` succeeds and replaces items; confirmed meal's status becomes `pending_review`/`pending_clarification`, not `confirmed`; blank hint → 400; no photo → 409; `processing`/`pending_clarification` → 409

## 6. Frontend: API client

- [ ] 6.1 Add `api.listMeals(limit?)`, `api.patchMeal(id, {name?, logged_at?})`, `api.createMealItem(id, body)`, `api.deleteMealItem(id, itemId)`, `api.reanalyzeMeal(id, hint)` to `frontend/lib/api.ts`, matching existing method conventions

## 7. Frontend: meal history page

- [ ] 7.1 Add `frontend/app/food/history/page.tsx` (+ client component if needed, following the `output: 'export'` static-page pattern already used by `frontend/app/food/review/`) listing meals from `api.listMeals()`: name, date, status badge, calories (blank unless confirmed), linking to `/food/review/?meal=<id>`
- [ ] 7.2 Add a "History" link from the dashboard (`frontend/app/page.tsx`), alongside the existing Photo/Manual food-logging links

## 8. Frontend: unlock confirmed-meal editing

- [ ] 8.1 In `frontend/app/food/review/ReviewClient.tsx`, drop the `readOnly={meal.status === 'confirmed'}` prop passed to `MealItemRow` (item editing now permitted for `confirmed`)
- [ ] 8.2 Add a delete control to `frontend/components/food/MealItemRow.tsx`, calling `api.deleteMealItem`, updating parent state on success
- [ ] 8.3 Add an "add item" control to the review page (reusing `ItemResolver`'s bind/manual flows) calling `api.createMealItem`
- [ ] 8.4 Add inline editing for the meal's `name` and `logged_at` in `ReviewClient.tsx`, calling `api.patchMeal`
- [ ] 8.5 Update `MacroSummary.tsx`'s "totals will be calculated when you confirm" copy to account for confirmed-meal totals now being live/editable, not just a one-time confirm result

## 9. Frontend: reanalyze with hint

- [ ] 9.1 Add a "Reanalyze with a hint" control to `ReviewClient.tsx` (available whenever the backend would accept it: `failed`, `pending_review`, `confirmed`), prompting for the hint text and warning that current items will be replaced and the meal may need re-confirming
- [ ] 9.2 Wire it to `api.reanalyzeMeal`, refreshing the displayed meal from the response

## 10. Validation

- [ ] 10.1 `make lint` / `go vet` (backend), `tsc --noEmit` (frontend)
- [ ] 10.2 Backend unit tests: `make test` or equivalent, all green
- [ ] 10.3 Deploy to `hcw-wip`, exercise: edit an item on a confirmed meal and see totals update; add and delete an item on a confirmed meal; rename/re-time a confirmed meal; browse meal history and open a meal from it; reanalyze a confirmed meal with a hint and confirm it reverts to pending_review with new items
- [ ] 10.4 Add/extend Playwright E2E coverage in `e2e/tests/food.spec.ts` for: history list navigation, editing/adding/deleting an item on a confirmed meal, reanalyze-with-hint flow
