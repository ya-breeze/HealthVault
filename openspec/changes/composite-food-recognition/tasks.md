## 1. Recognize: composite-dish naming and macro estimate

- [ ] 1.1 Update the Recognize system prompt in `backend/pkg/vision/openai.go` to default to whole-dish naming, decomposing into multiple items only when visually separate components are observed.
- [ ] 1.2 Extend `vision.Item` (`backend/pkg/vision/vision.go`) with an optional per-item estimated nutrient profile (calories, protein, carbs, fat, sugar, sodium, dietary fiber) and update the Recognize JSON schema/response parsing to populate it unconditionally.
- [ ] 1.3 Add unit tests covering: a homogeneous composite dish photo yields one item; a plate with visually separate components still yields multiple items; the estimated profile is present on parsed Recognize output.

## 2. Custom-food candidate ranking

- [ ] 2.1 Implement a frequency/recency ranking query for a user's `CustomFood` rows, joining `FoodItem`/`FoodMeal` by `user_id` and `custom_food_id`, weighting usage frequency higher than recency.
- [ ] 2.2 Extend `retrieveCandidates` in `backend/pkg/server/food_upload.go`: keep the existing exact-name short-circuit; when it misses, add the top-N frequency/recency-ranked custom foods to the candidate shortlist alongside the existing Open Food Facts/USDA candidates for that item.
- [ ] 2.3 Adjust the Select prompt/instructions so the model is told to prefer a user's own custom-food candidate over a generic reference candidate when both are reasonable matches.
- [ ] 2.4 Add unit/integration tests: a differently-worded recognized item still matches a previously-used custom food via the ranked shortlist; a custom food used only monthly still surfaces; a custom food is preferred over a plausible USDA/OFF candidate when both are offered.

## 3. Macro-source resolution and estimate fallback

- [ ] 3.1 Add `estimated` to the `MacroSource` values in `backend/pkg/database/models_food.go` and everywhere `MacroSource` is validated or switched on.
- [ ] 3.2 In item resolution (`food_upload.go`), when Select returns no match for an item, scale that item's macros from its Recognize-provided estimated profile and set `macro_source = estimated`, instead of `macro_source = none`.
- [ ] 3.3 Update meal-aggregate confirmation logic so `estimated` items contribute to the 7-macro totals, alongside `reference` and `manual` (only `none` stays excluded).
- [ ] 3.4 Add tests: an unmatched item with an estimated profile is stored as `estimated` with scaled macros; a meal confirm aggregates `estimated` items; a matched item discards its (now-unused) estimate.

## 4. Frontend

- [ ] 4.1 Add a distinct visual treatment for `macro_source = estimated` items on the confirm-review screen (e.g. `frontend/components/food/...`), separate from the existing `reference`/`manual`/`none` treatments, indicating it is an unverified AI estimate.
- [ ] 4.2 Verify the existing item-edit/override flow lets a user correct an `estimated` item's macros and optionally save it as a `CustomFood`.

## 5. Validation

- [ ] 5.1 Run backend unit tests (`make test` or equivalent) and confirm all pass.
- [ ] 5.2 Deploy to the WIP/dogfood stack per the standard workflow and manually verify: a composite dish photo produces one item; a repeated (differently-worded) dish reuses a saved custom food's macros; an unmatched dish gets an `estimated` macro fallback shown distinctly in review.
- [ ] 5.3 Run the existing Playwright E2E suite against the deployed stack and confirm no regressions; add/extend a spec if an existing gap is found for this behavior.
