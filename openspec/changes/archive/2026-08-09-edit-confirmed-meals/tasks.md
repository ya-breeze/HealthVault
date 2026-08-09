## 1. Backend: meal history list

- [x] 1.1 Define a `MealSummary` DTO (`id`, `name`, `logged_at`, `status`, `calories`) in `backend/pkg/server/food_meal_detail.go` — no `photo_path`, `raw_response`, `clarify_log`, or tenant metadata
- [x] 1.2 Add `GET /api/food/meals` handler: owner-scoped (`claims.UserID`), ordered `logged_at DESC, id DESC` (shipped as a two-field keyset — `id` alone is a complete tie-break, no `created_at` needed); parse `limit` (absent → 50; positive integer ≤ 200 → that value; anything else → 400), and `before`+`before_id` together as an exact `(logged_at, id)` keyset cursor (`before` alone → simpler non-cursor timestamp filter; `before_id` without `before` → 400; either malformed → 400)
- [x] 1.3 Register the route in `backend/pkg/server/server.go`
- [x] 1.4 Unit tests: own-meals-only scoping (incl. cross-user isolation), deterministic tie-break ordering for meals sharing `logged_at`, default/max/invalid `limit` (`-1`, `0`, `201`, `abc`), `before` paging and invalid `before`, all statuses included, response contains only the five summary fields, 401 unauthenticated

## 2. Backend: item add/delete + confirmed-meal item editing

- [x] 2.1 In `backend/pkg/server/food_item.go`, change `PatchMealItem`'s status guard from "reject if confirmed" to "reject unless `pending_review` or `confirmed`"
- [x] 2.2 Add `POST /api/food/meals/{id}/items` handler with creation-specific validation: non-blank `name` required, plus exactly one of `manual: true` + macro values, or `fdc_id`/`custom_food_id` + `weight_grams > 0` — 400 otherwise. Remember `TenantModel` has no `BeforeCreate` — set `ID`/`FamilyID`/`UserID`/`MealID` explicitly. Same status guard as 2.1
- [x] 2.3 Add `DELETE /api/food/meals/{id}/items/{item_id}` handler, same status guard, 404 for missing/unowned item
- [x] 2.4 Register both new routes in `backend/pkg/server/server.go`
- [x] 2.5 Add a shared helper, called from patch/create/delete, that wraps the item mutation in one `db.Transaction`: apply the mutation, and if the meal's status is `confirmed`, reload its current items *inside that transaction*, recompute via `FoodMeal.Aggregate`, and persist before commit — rolling back the item mutation too if the aggregate write fails. Leave `pending_review` behavior untouched (no recompute)
- [x] 2.6 Change all three handlers' response bodies from the bare `FoodItem` to the full updated `FoodMeal` (items + current aggregate), matching `GetMeal`/`ConfirmMeal`'s existing response shape
- [x] 2.7 Unit tests: patch/create/delete on a confirmed meal succeed, update the meal's stored aggregate, and return the full meal in the response; a simulated failure in the aggregate half of the transaction leaves neither the item change nor the aggregate committed; patch/create/delete on `processing`/`pending_clarification`/`failed` return 409; create/delete/patch on `pending_review` behave as before (no aggregate recompute); delete of nonexistent/unowned item returns 404; create rejects missing name, missing source, and non-positive reference weight with 400; cross-user patch/create/delete all return 404 and make no change

## 3. Backend: meal name/logged_at correction

- [x] 3.1 Add `PATCH /api/food/meals/{id}` handler in `backend/pkg/server/food_meal_detail.go` accepting optional `name`/`logged_at`, requiring at least one; reject a zero-value `logged_at` with 400; same `pending_review`/`confirmed` status guard, 409 otherwise
- [x] 3.2 Register the route in `backend/pkg/server/server.go`
- [x] 3.3 Unit tests: rename confirmed meal, correct logged_at, empty body → 400, zero-value logged_at → 400, wrong status → 409, cross-user → 404

## 4. Backend: vision hint plumbing

- [x] 4.1 Add a `hint string` parameter to `vision.Client.Recognize` in `backend/pkg/vision/vision.go`
- [x] 4.2 Update `OpenAIClient.Recognize` (`backend/pkg/vision/openai.go`) to append the hint to the prompt when non-empty, image still sent
- [x] 4.3 Update `Fake.Recognize` and `Unconfigured.Recognize` to accept (and, for `Fake`, optionally assert on) the new parameter
- [x] 4.4 Update all existing callers of `Recognize` (`analyzeMeal` in `backend/pkg/server/food_upload.go`, and anywhere else it's called, e.g. calibration) to pass an empty hint, preserving current behavior
- [x] 4.5 In `persistAnalysis` (`backend/pkg/server/food_upload.go`), add unconditional zeroing of the meal's seven aggregate columns to its existing update — a no-op for the three current callers (upload/clarify/retry, whose aggregate is already zero at that point), and required for `Reanalyze` (task 5) so a meal leaving `confirmed` never carries forward stale totals

## 5. Backend: reanalyze endpoint

- [x] 5.1 Add `backend/pkg/server/food_reanalyze.go` with `POST /api/food/meals/{id}/reanalyze`: read the body through a 4 KiB-limited reader (413 if exceeded), parse `{"hint": string}`, 400 if blank or over 500 characters
- [x] 5.2 Capture the meal's current `status`, `clarify_round`, `clarify_log` before claiming it
- [x] 5.3 Atomically claim the meal on the *exact* `(status, updated_at)` observed, writing a fresh `updated_at` as this attempt's lease token: `UPDATE ... SET status='processing', clarify_round=0, clarify_log='', updated_at=<lease> WHERE id=? AND status=<observed> AND updated_at=<observed>`; 0 rows affected → 409, no vision call. 409 also if no stored photo. (Shipped as an exact match, not `status IN (...)` — see task 11 for why)
- [x] 5.4 On success: run the same recognize → `processRecognition` → `persistAnalysis` pipeline `analyzeMeal` uses (which now also zeroes the aggregate per 4.5), passing the hint through
- [x] 5.5 On vision error/timeout: restore the captured `status`/`clarify_round`/`clarify_log` (not `failMeal`), and respond HTTP 502 with an error body — do not touch items or aggregate
- [x] 5.6 Register the route in `backend/pkg/server/server.go`
- [x] 5.7 Unit tests: reanalyze from `failed`/`pending_review`/`confirmed` succeeds and replaces items; confirmed meal's status becomes `pending_review`/`pending_clarification` with aggregate zeroed, not `confirmed`; blank hint → 400; hint over 500 chars → 400; body over 4 KiB → 413; no photo → 409; `processing`/`pending_clarification` → 409; two concurrent reanalyze calls on the same meal — only one proceeds, the other gets 409; a meal that already went through clarification reanalyzes with `clarify_round` reset to 0 and no leftover log; a simulated vision failure on a `confirmed` meal leaves status/items/aggregate exactly as they were and returns 502; same for a `pending_review` meal; cross-user reanalyze → 404, no vision call

## 6. Frontend: API client

- [x] 6.1 Add `api.listMeals({limit?, before?, beforeId?})`, `api.patchMeal(id, {name?, logged_at?})`, `api.createMealItem(id, body)`, `api.deleteMealItem(id, itemId)`, `api.reanalyzeMeal(id, hint)` to `frontend/lib/api.ts`, matching existing method conventions. `patchMealItem`, `createMealItem`, and the delete call all now resolve to a `FoodMeal`, not a `FoodItem` — update their return types accordingly. Handle `reanalyzeMeal`'s HTTP 502 as a distinct "reanalysis failed" outcome, not a generic error. `beforeId` must be sent alongside `before` for the lossless keyset cursor — a `before`-only call falls back to a lossy timestamp filter (see 1.2)

## 7. Frontend: meal history page

- [x] 7.1 Add `frontend/app/food/history/page.tsx` (+ client component if needed, following the `output: 'export'` static-page pattern already used by `frontend/app/food/review/`) listing meals from `api.listMeals()`: name, date, status badge, calories (blank unless confirmed), linking to `/food/review/?meal=<id>`
- [x] 7.2 Add a "load older" action that re-calls `api.listMeals({before: <oldest shown logged_at>, beforeId: <oldest shown id>})` and appends results — both fields are required together for the lossless cursor. `hasMore` is set from `rows.length === PAGE_SIZE`: the keyset cursor never defers or splits rows, so a short page reliably means nothing more remains
- [x] 7.3 Add a "History" link from the dashboard (`frontend/app/page.tsx`), alongside the existing Photo/Manual food-logging links

## 8. Frontend: unlock confirmed-meal editing

- [x] 8.1 In `frontend/app/food/review/ReviewClient.tsx`, drop the `readOnly={meal.status === 'confirmed'}` prop passed to `MealItemRow` (item editing now permitted for `confirmed`)
- [x] 8.2 Update `MealItemRow`'s `onUpdated` (and the review page's handling of it) to replace the whole `meal` object from the response, not splice a single item into `meal.items` — since patch/create/delete now return the full meal, this is what keeps `MacroSummary`'s displayed total in sync immediately after an edit
- [x] 8.3 Add a delete control to `frontend/components/food/MealItemRow.tsx`, calling `api.deleteMealItem`, updating parent state (full meal) on success
- [x] 8.4 Add an "add item" control to the review page (reusing `ItemResolver`'s bind/manual flows) calling `api.createMealItem`, updating parent state (full meal) on success
- [x] 8.5 Add inline editing for the meal's `name` and `logged_at` in `ReviewClient.tsx`, calling `api.patchMeal`
- [x] 8.6 Update `MacroSummary.tsx`'s "totals will be calculated when you confirm" copy to account for confirmed-meal totals now being live/editable, not just a one-time confirm result

## 9. Frontend: reanalyze with hint

- [x] 9.1 Add a "Reanalyze with a hint" control to `ReviewClient.tsx` (available whenever the backend would accept it: `failed`, `pending_review`, `confirmed`), prompting for the hint text (enforce the 500-char limit client-side too) and warning that current items will be replaced and the meal may need re-confirming
- [x] 9.2 Wire it to `api.reanalyzeMeal`; on success, refresh the displayed meal from the response; on the 502 "reanalysis failed" outcome, show an error and leave the currently displayed meal exactly as it was (no refetch needed — the backend guarantees nothing changed)

## 10. Validation

- [x] 10.1 `make lint` / `go vet` (backend), `tsc --noEmit` (frontend)
- [x] 10.2 Backend unit tests: `make test` or equivalent, all green
- [x] 10.3 Deploy to `hcw-wip`, exercise: edit an item on a confirmed meal and see totals update live; add and delete an item on a confirmed meal; rename/re-time a confirmed meal; browse meal history, page to older meals, and open a meal from it; reanalyze a confirmed meal with a hint and confirm it reverts to pending_review with new items and a zeroed total; simulate a reanalyze failure (e.g. bad hint length) and confirm the meal is untouched
- [x] 10.4 Add/extend Playwright E2E coverage in `e2e/tests/food.spec.ts` for: history list navigation and "load older" (seeds 51 real meals to prove a genuine second page is fetched and appended, not just that an empty page hides the button), editing/adding/deleting an item on a confirmed meal with the visible total updating immediately (via a real macro edit, not a weight-only one that leaves calories untouched), and reanalyze-with-hint. Reanalyze's deterministic UI coverage (both success and 502-failure) uses `page.route()` network mocking rather than depending on live vision output; a separate live-provider test remains as supplementary smoke coverage, since it can't guarantee which outcome a synthetic test image produces. Every test that creates data wraps its assertions in `try`/`finally` so a mid-test failure can't leak meals into the shared, reused WIP account

## 11. Backend: lease token for concurrent analysis attempts

Found in review: `Reanalyze`'s exact-status claim (task 5.3) doesn't account for `RetryMeal`'s own stale-processing eligibility overlapping the same `processing` state — see design.md "A lease token... guards every analysis attempt against being superseded".

- [x] 11.1 Thread a `lease time.Time` parameter through `analyzeMeal`/`runAnalysis`/`processRecognition`/`persistAnalysis`/`failMeal` (`backend/pkg/server/food_upload.go`); `persistAnalysis`'s and `failMeal`'s final writes are conditioned on `updated_at = lease`, returning/no-oping (`errLeaseLost`) rather than erroring loudly when the lease no longer matches
- [x] 11.2 `CreateMeal` captures its lease from `meal.UpdatedAt` right after `Create` (GORM sets it); `RetryMeal`'s claim becomes an optimistic-concurrency check (`WHERE status = ? AND updated_at = ?`, matching what was just read) that also writes a fresh lease; `ClarifyMeal`'s existing claim gains the same `updated_at` condition and fresh-lease write
- [x] 11.3 `Reanalyze`'s revert (`revertReanalyze`) is conditioned on its own lease; if the lease is gone (a newer attempt claimed the meal), the revert no-ops and the response reports failure via a *distinct* status (HTTP 412, not the 502 used when the revert actually applies), since the "unchanged" guarantee no longer holds once another attempt owns the row (see 11.6)
- [x] 11.4 Unit tests: `RetryMeal` claim rejected when a concurrent claim lands first (0 vision calls); `Reanalyze`'s revert does not stomp a newer concurrent claim's status when its own attempt fails — both via a deterministic GORM `Before(update)` hook simulating the concurrent write, no goroutines needed
- [x] 11.5 Sync `openspec/specs/food-photo-recognition`'s delta: rewrite the Reanalyze "Atomic claim" requirement to describe the exact-status + lease design (not the original `status IN (...)` text), and add a `MODIFIED Requirements` delta for the archived "Analysis Retry Without Re-Upload" requirement describing `Retry`'s own strengthened claim
- [x] 11.6 Distinguish lease-loss from a genuine unchanged failure at the HTTP level: `Reanalyze` responds HTTP 412 (Precondition Failed) when its revert doesn't apply, vs the existing HTTP 502 when it does. Frontend: `api.reanalyzeMeal` maps 412 to a new `ReanalyzeSupersededError` (distinct from `ReanalyzeFailedError`'s 502); `ReanalyzeControl` refetches the meal via `api.getMeal` and updates the display on that error, instead of applying the "unchanged" message/no-refetch behavior meant for the 502 case
- [x] 11.7 `analyzeMeal` and `failMeal` (`backend/pkg/server/food_upload.go`) now return whether their write actually applied; add a `reloadIfSuperseded` helper that returns the given meal unchanged if applied, or a fresh `loadOwnedMeal` reload if not. `CreateMeal`, `RetryMeal`, and `ClarifyMeal` all respond with `reloadIfSuperseded(meal, applied)` instead of their raw local struct, so a superseded attempt's response reflects the meal's real current state rather than its own stale claim
- [x] 11.8 Unit tests: `RetryMeal`'s response reflects the actual superseding state (not its own stale "processing" view) when its own write is preempted by a newer attempt — via the gated-vision-client + goroutine technique (a GORM hook would deadlock here, since the injected write would need to land *inside* `persistAnalysis`'s own active transaction)
- [x] 11.9 E2E: add a mocked 412 test alongside the existing mocked 502 test, asserting the UI refetches and shows the superseded state; fix the 51-meal "Load older" test to not assert global account exhaustion (remove the fragile final assertion, keep the real-page-2-append proof) and add a separate, fully isolated mocked test for the short-page-hides-the-button behavior; make `deleteMeal` verify a successful response (2xx or 404) and add `deleteMeals` (`Promise.allSettled`) so cleanup attempts every ID even if one fails, instead of silently trusting an unchecked response or abandoning a sequential loop after one throw
