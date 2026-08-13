## 1. Data model

- [x] 1.1 Add `estimated` to the `MacroSource` values in `backend/pkg/database/models_food.go` and everywhere `MacroSource` is validated or switched on.
- [x] 1.2 Add a nullable persisted per-100g estimated-nutrient-profile field to `FoodItem` (calories/protein/carbs/fat/sugar/sodium/dietary fiber) and run the corresponding GORM auto-migration.
- [x] 1.3 Add unit tests for the new field's persistence and default-nil behavior on existing rows.

## 2. Recognize: composite-dish naming and macro estimate

- [x] 2.1 Update the Recognize system prompt in `backend/pkg/vision/openai.go` to default to whole-dish naming, decomposing into multiple items only when visually separate components are observed.
- [x] 2.2 Extend `vision.Item` (`backend/pkg/vision/vision.go`) with an optional per-item estimated nutrient profile and update the Recognize JSON schema/response parsing to populate it unconditionally.
- [x] 2.3 When an item is first created (`resolveItems`/`processRecognition` in `food_upload.go`), persist that item's estimated profile onto its `FoodItem` row (task 1.2) at creation time, regardless of whether a match is later found.
- [x] 2.4 Add unit tests: a homogeneous composite dish photo yields one item; a plate with visually separate components still yields multiple items; the estimated profile is present on parsed Recognize output and persisted on the created `FoodItem`.

## 3. Custom-food candidate ranking

- [x] 3.1 Implement a frequency/recency ranking query: `SELECT custom_food_id, COUNT(*), MAX(food_meals.logged_at) FROM food_items JOIN food_meals ON food_items.meal_id = food_meals.id WHERE food_items.user_id = ? AND food_items.custom_food_id IS NOT NULL AND food_meals.status = 'confirmed' GROUP BY custom_food_id`, weighting usage frequency higher than recency. Note: join on `meal_id`, not `user_id` — `FoodItem.user_id` is a filter, not a join key, and joining two tables on it would fan out/multiply counts.
- [x] 3.2 Extend `retrieveCandidates` in `backend/pkg/server/food_upload.go`: keep the existing exact-name lookup and its single-candidate-shortlist behavior unchanged; when it misses, add the top-N frequency/recency-ranked custom foods to the candidate shortlist alongside whatever Open Food Facts/USDA candidates the existing brand-based routing already produces (additive, not exclusive with either branch).
- [x] 3.3 Adjust the Select prompt/instructions so the model is told to prefer a user's own custom-food candidate over a generic reference candidate when both are reasonable matches.
- [x] 3.4 Add unit/integration tests: a differently-worded recognized item still matches a previously-used custom food via the ranked shortlist; a custom food used only monthly still surfaces; a custom food is preferred over a plausible USDA/OFF candidate when both are offered; a match on a still-`pending_review` meal is excluded from the ranking until the meal is confirmed; a branded item with Open Food Facts candidates still also gets ranked custom-food candidates alongside them.

## 4. Macro-source resolution and estimate fallback

- [x] 4.1 In item resolution (`food_upload.go`), apply the persisted estimated profile as fallback (`macro_source = estimated`, scaled by weight) at every current path that results in `macro_source = none`: an empty candidate shortlist (today, `resolveItems` returns before calling `Select` at all when every item's shortlist is empty), an explicit `Select` index of `-1`, a `Select` response error in non-strict mode, an out-of-range selected index, and a selected candidate whose profile lookup fails. Only fall through to `none` when no usable persisted estimate exists either.
- [x] 4.2 Update meal-aggregate confirmation logic so `estimated` items contribute to the 7-macro totals, alongside `reference` and `manual` (only `none` stays excluded).
- [x] 4.3 Add tests covering each of the paths in 4.1: an item with an empty shortlist and a usable estimate becomes `estimated`; the same with no usable estimate stays `none`; a matched item discards its (now-unused) persisted estimate but keeps it stored on the row; a meal confirm aggregates `estimated` items.

## 5. Clarification-round interaction

- [x] 5.1 Confirm `ClarifyMeal`'s reconstruction of prior items from persisted `FoodItem` fields does not need to reproduce the estimated profile (since it's already persisted on the row from task 2.3) — verify the item-resolution path after a clarification round reads the same persisted field rather than expecting a fresh one from the text-only clarify call.
- [x] 5.2 Add a test: an item enters `pending_clarification`, the clarification round concludes with still no candidate match, and the item falls back to its originally-persisted estimated profile rather than `none`.

## 6. Item-resolution: weight rescale and save-as-custom-food

- [x] 6.1 Extend the weight-only PATCH rescale path in `backend/pkg/server/food_item.go` to also handle `macro_source = estimated` items, rescaling from the item's persisted per-100g estimated profile (mirrors the existing `reference` rescale branch).
- [x] 6.2 Add an optional "save as custom food" action to `PATCH /api/food/meals/{id}/items/{item_id}` when the caller supplies direct macro values: on request, also create a `CustomFood` owned by the caller from the item's name and supplied per-100g values, in the same transaction.
- [x] 6.3 Add tests: a weight-only edit on an `estimated` item recalculates macros correctly; a correction with the save-as-custom-food flag creates a matching `CustomFood` row; a correction without the flag behaves exactly as today (no `CustomFood` created).

## 7. Frontend

- [x] 7.1 Add a distinct visual treatment for `macro_source = estimated` items on the confirm-review screen (e.g. `frontend/components/food/...`), separate from the existing `reference`/`manual`/`none` treatments, indicating it is an unverified AI estimate.
- [x] 7.2 Add a "save as reusable food" checkbox/action to the item-correction UI, wired to the new PATCH option from task 6.2.

## 8. OpenSpec projected specs

- [x] 8.1 After specs stabilize, run `make projected-specs` and commit the result so the PR's drift-check passes and reviewers can see the old→new spec diff.

## 9. Validation

- [x] 9.1 Run backend unit tests (`make test` or equivalent) and confirm all pass.
- [x] 9.2 Deploy to the WIP/dogfood stack per the standard workflow and manually verify: a composite dish photo produces one item; a repeated (differently-worded) dish reuses a saved custom food's macros; an unmatched dish gets an `estimated` macro fallback shown distinctly in review; a weight edit on an estimated item rescales correctly; a correction can be saved as a reusable custom food and is picked up on a later photo.
- [x] 9.3 Run the existing Playwright E2E suite against the deployed stack and confirm no regressions; add/extend a spec if an existing gap is found for this behavior.
