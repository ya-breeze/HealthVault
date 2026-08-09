## Why

Once a meal is confirmed, its items are locked (`PatchMealItem` returns 409, the review UI renders `readOnly`), and there is no way to add an item that was missed, delete one that was wrong, or fix the meal's name/time. There is also no page to find a past meal at all — the only way into `/food/review/?meal=<uuid>` is a hand-typed URL. A user who logs food regularly needs to be able to go back and correct entries, and needs a way to reach them.

## What Changes

- Add a meal history list (new page + new owner-scoped list endpoint) so a user can browse and open any of their past meals, of any status.
- Unlock item editing (weight, food binding, manual macros, name) for `confirmed` meals, matching what is already possible for `pending_review` meals.
- Add the ability to add a new item to, and delete an existing item from, a meal (previously only editing an existing item was possible).
- Add a way to correct a meal's `name` and `logged_at` directly, independent of confirming.
- Make a confirmed meal's stored macro totals stay correct as its items change after confirmation, instead of only being computed once at confirm time.
- Add a "reanalyze with hint" action: the user supplies free text (e.g. "this is actually chicken and rice, not berries") and the system re-runs vision recognition on the stored photo with that hint, replacing the meal's items — available even for an already-confirmed meal, which reverts it to `pending_review`/`pending_clarification` for re-review.

## Capabilities

### New Capabilities
- `food-meal-history`: owner-scoped list of a user's meals (any status) and the page that browses it, linking into the existing per-meal review page.

### Modified Capabilities
- `food-nutrition-logging`: item resolution (`PATCH .../items/{item_id}`) is no longer rejected once a meal is `confirmed`; add `POST .../items` (create) and `DELETE .../items/{item_id}` (remove); add `PATCH /api/food/meals/{id}` for `name`/`logged_at`; the meal's stored macro aggregate is recomputed and persisted on every item change while the meal is `confirmed`, not only at confirm time.
- `food-photo-recognition`: add `POST /api/food/meals/{id}/reanalyze`, which re-runs vision recognition on the stored photo with a required free-text hint, eligible from `failed`, `pending_review`, or `confirmed`, replacing all items and resetting status — a hint-driven counterpart to the existing no-hint `Retry`. Because `Retry`'s existing eligibility (accepting `processing` once `updated_at` is older than the vision timeout) now overlaps with a state `Reanalyze` can also put a meal in, `Retry`'s own claim is strengthened from a blind write to the same lease-token pattern `Reanalyze` uses, so the two can't clobber each other's in-flight or completed work.

## Impact

- Backend: `backend/pkg/server/food_meal_detail.go` (new list handler, new meal-level PATCH), `backend/pkg/server/food_item.go` (drop confirmed-status 409, add create/delete handlers, aggregate recompute), new `backend/pkg/server/food_reanalyze.go` (new handler), `backend/pkg/vision/vision.go` + `openai.go` + `fake.go` + `unconfigured.go` (`Recognize` gains an optional hint parameter), `backend/pkg/server/server.go` (new routes).
- Frontend: new `frontend/app/food/history/` page, `frontend/app/food/review/ReviewClient.tsx` and `frontend/components/food/MealItemRow.tsx` (unlock editing for confirmed, add delete/add-item controls, meal name/logged_at editing, reanalyze-with-hint UI), `frontend/app/page.tsx` (dashboard link to history), `frontend/lib/api.ts` (new API client methods).
- No database schema changes — reuses existing `FoodMeal`/`FoodItem` tables and `FoodMeal.Aggregate`.
