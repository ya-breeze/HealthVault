## Context

`photo-food-nutrition-logging` (archived) shipped photo upload, vision recognition, clarification, item resolution, and confirm. Once confirmed, a meal's items are permanently locked: `PatchMealItem` (`backend/pkg/server/food_item.go`) returns 409 for a `confirmed` meal, and `ReviewClient.tsx` renders `MealItemRow` with `readOnly={meal.status === 'confirmed'}`. The meal's macro aggregate (`FoodMeal.Aggregate`) is computed exactly once, inside `ConfirmMeal`.

There is also no way to browse past meals: `GET /api/food/meals/{id}` (owner-scoped) exists but nothing lists meal IDs for a user. The only entry points are the direct link from the upload flow and a hand-typed URL. The generic `GET /api/data/food_meal` table (family-visible, no items/photo) shows rows with no links out.

`RetryMeal` (`backend/pkg/server/food_retry.go`) already re-runs vision analysis on a stored photo and fully replaces items via `persistAnalysis`, but only for `failed` or stale-`processing` meals, and with no way to give the model new information.

## Goals / Non-Goals

**Goals:**
- Let the owner find any of their past meals (any status) and open it.
- Let the owner edit, add, and delete items on a `confirmed` meal, and correct its `name`/`logged_at`, with the meal's stored totals always reflecting its current items.
- Let the owner supply a free-text hint and have vision re-analyze the stored photo, from `failed`, `pending_review`, or `confirmed`.

**Non-Goals:**
- Paginated/cursor-based meal history — a capped recent-N list is enough for now.
- Preserving or merging manual edits across a reanalysis — full item replacement, as `Retry` already does.
- Giving `Retry` itself a hint parameter, or changing its existing eligibility/behavior.
- Family visibility for the new meal list endpoint — it stays owner-scoped like `GetMeal`, unlike the generic `/api/data/food_meal`.

## Decisions

**Owner-scoped list endpoint, not the generic `/api/data` mechanism.** `GET /api/data/food_meal` is family-visible and exposes only denylist-filtered raw columns; `GetMeal` (item detail) is deliberately owner-only. Building the history list on top of the generic endpoint would let a user see a row in the list they then get 404 opening — reusing it would require either making detail family-visible (a bigger, unrelated change) or accepting broken links in the list. A small dedicated `GET /api/food/meals` handler, scoped to `claims.UserID` exactly like `GetMeal`, keeps every listed row openable and needs no change to the generic data-API's family-visibility model.

**Aggregate recompute is synchronous and per-write, not a background job.** There is no job infrastructure in this backend (see project conventions) and none is needed here: each item add/edit/delete handler, after persisting the item change, checks `meal.Status == confirmed` and if so calls `FoodMeal.Aggregate(items)` again and updates the meal's stored macro columns in the same request. This mirrors `ConfirmMeal`'s existing update pattern and keeps the invariant "a confirmed meal's stored totals equal the sum over its current items" true after every write, with no eventual-consistency window.

**`Reanalyze` is a new endpoint, not a `Retry` extension.** `Retry`'s contract (eligibility: `failed` or stale-`processing`; no new information, same prompt) is used elsewhere and has its own test suite. Adding an optional hint and broadening eligibility to `pending_review`/`confirmed` would change what "retry" means for every existing caller conceptually, even if backward compatible. A separate `POST /api/food/meals/{id}/reanalyze` with a **required** hint keeps `Retry`'s meaning ("recover from a failure, unchanged") and `Reanalyze`'s meaning ("I have new information, redo it") each single-purpose. Both funnel into the same underlying `analyzeMeal`/`processRecognition`/`persistAnalysis` pipeline, parameterized by an optional hint string.

**Reanalyzing a confirmed meal discards it back to `pending_review`/`pending_clarification`, full item replacement.** `persistAnalysis` already deletes and replaces all items for every analysis run (`Retry` included). Reusing it for `Reanalyze` gets this for free and keeps one item-replacement code path instead of two. The alternative (try to reconcile new vision output against manually-edited items) has no clear merge rule — e.g. a manually added item has no vision-model counterpart to reconcile against — and was explicitly ruled out as out of scope.

**Vision hint threads through `Recognize`, not a separate prompt-building path.** `vision.Client.Recognize(ctx, image, mimeType string)` gains a fourth parameter, `hint string` (empty for the normal upload/retry path). `OpenAIClient.Recognize` appends it to the user-turn content when non-empty; `Fake` and `Unconfigured` accept and ignore it. This keeps one prompt-construction path instead of forking the system prompt or adding a hint-specific method, and mirrors how `Clarify` already extends the conversation textually without a second prompt template.

**New item endpoints reuse the existing PATCH request shape and precedence.** `POST /api/food/meals/{id}/items` (create) accepts the same `patchItemRequest` fields (`manual`/`fdc_id`/`custom_food_id`/`weight_grams`/`name` plus macro fields) as `PATCH .../items/{item_id}`, with the same manual-vs-reference precedence, so `ItemResolver`'s existing bind/manual flows in the frontend can be reused for "add" with only a different endpoint and method.

## Risks / Trade-offs

- **[Risk]** A confirmed meal being knocked back to `pending_review` by `Reanalyze` is a real state change the owner might not expect ("I thought I was just adding a hint"). → **Mitigation**: the frontend UI for triggering reanalysis explicitly warns that current items will be replaced and the meal will need re-confirming before it's shown to the user; this was confirmed as the desired behavior.
- **[Risk]** Recomputing the aggregate on every item write to a confirmed meal adds a second DB write per edit (item save + meal update). → **Mitigation**: both are small, single-row updates on SQLite in the same request; no batching needed at this scale (personal food log, not high-frequency writes).
- **[Trade-off]** The meal history list has no pagination; a user with a very large meal history gets only the most recent N. → Acceptable for now; revisit if it becomes a real limitation (Non-Goal above).

## Migration Plan

No data migration — reuses existing tables and columns. Deploy is a normal backend + frontend release; no rollback concerns beyond reverting the branch.

## Open Questions

None outstanding — design decisions above were confirmed with the user during brainstorming.
