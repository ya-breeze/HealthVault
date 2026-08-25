## 1. Prompt wording

- [ ] 1.1 Reword the composite-dish-naming paragraph of `recognizeSystemPrompt` in
  `backend/pkg/vision/openai.go` to key the split/merge decision on whether each visible
  component was ever served as its own separate portion, not on whether it plays a different
  role from its neighbor, not on whether components are spatially separated on the plate, and
  not on whether an individual piece can be pointed to and named (an ingredient chunk inside a
  combined preparation almost always can — that alone must not trigger a split) — a
  protein-and-side pairing is the common example, but two foods that were each served
  separately and share a role (e.g. two different cooked vegetable sides plated touching) split
  the same way. State explicitly that "served as its own separate portion" is judged from what
  the photo shows, not from unobservable prep history: a piece that visibly keeps its own
  separately-servable, portion-scale form (an intact fillet, a whole cutlet, a distinct pile)
  counts as served separately; a piece broken down, mixed, or tossed into one preparation does
  not. A sauce, glaze, or juices from a neighboring food covering a piece does not by itself turn
  it into a combined preparation — a fillet coated in sauce still counts as its own
  separately-servable piece as long as it keeps its own portion-scale shape.
- [ ] 1.2 Add a worked example pair to the prompt text: a protein served touching or on top of a
  vegetable/starch side (e.g. "fish served on/next to stewed cabbage") should split into two
  items — including when the protein was baked or braised directly in contact with the side
  (e.g. a fish fillet baked on top of stewed cabbage), so long as it keeps its own portion-scale
  shape and could be lifted off and served on its own; sharing a pan, pot, or oven dish is not by
  itself a merge signal — a single preparation whose ingredients were mixed, chopped, or
  cooked/sauced together into one served dish (e.g. a stir-fry, curry, stew, or mixed salad)
  should remain one item even though individual ingredient pieces (a lettuce leaf, a carrot
  chunk) are still visually distinguishable within it. Also state that a minor garnish or
  condiment (a lemon wedge, a sprig of herbs, a spoonful of sauce) that isn't itself a
  portion-sized food stays folded into its main item rather than becoming its own item.
- [ ] 1.3 Re-read the full prompt after editing to confirm the new wording does not contradict or
  weaken the still-desired merge behavior for genuinely homogeneous dishes (the original
  over-decomposition problem this requirement was added to fix), and does not cause garnishes or
  condiments to split out as their own items.

## 2. Spec update

- [ ] 2.1 Confirm the "Composite Dish Naming" requirement delta under
  `openspec/changes/split-distinct-plated-foods/specs/food-photo-recognition/spec.md` (already
  drafted in this change) matches the final prompt wording from task 1; adjust either if they
  drift during implementation.

## 3. Clarify interaction

- [ ] 3.1 Verify `Clarify` (`backend/pkg/vision/openai.go`), which shares
  `recognizeSystemPrompt`, does not have any separate item-count guidance elsewhere (e.g. in how
  its request is constructed or in `food_upload.go`'s handling of the clarify response) that
  would re-merge items an initial `Recognize` pass already split. If none is found, no code
  change is needed here beyond the shared prompt from task 1 — record that finding in the PR
  description.

## 4. Tests

- [ ] 4.1 Add a unit test fixture to `backend/pkg/server/food_upload_test.go` (or the vision
  package, whichever already covers prompt-adjacent behavior) constructing a multi-item
  `Recognize` response and asserting the persistence path creates one `FoodItem` per item in the
  response — the multi-item path is structurally supported today but has no existing test
  (confirmed in this change's research).
- [ ] 4.2 Add or extend a prompt-level test (if the project has one, e.g. a snapshot/golden test
  of `recognizeSystemPrompt`, or otherwise skip this) confirming the reworded prompt text is
  present as expected; otherwise rely on 4.1 plus manual verification against real photos.
- [ ] 4.3 Add a unit test fixture exercising the `Clarify` path (using `vision.Fake`'s
  `ClarifyResult`/`ClarifyResults`, mirroring the existing
  `TestCreateMeal_ClarificationQuestionsSetPendingClarification` test in
  `backend/pkg/server/food_upload_test.go`) constructing a multi-item `Clarify` response and
  asserting persistence creates one `FoodItem` per item — covering the risk task 3.1 identifies
  but does not itself add a repeatable check for.

## 5. Validation

- [ ] 5.1 Run backend unit tests (`make test` or equivalent) and confirm all pass.
- [ ] 5.2 Run `make lint` and fix anything it reports.
- [ ] 5.3 Deploy to the WIP/dogfood stack per the standard workflow and manually verify with a
  real photo similar to the "Тушёная капуста с нежирной белой рыбой" case: the plate is now
  split into two items (a vegetable/side item and a fish/protein item), and a control photo of a
  genuinely homogeneous dish (e.g. a stir-fry or stew) still returns as one item.
- [ ] 5.4 Run the existing Playwright E2E suite against the deployed stack and confirm no
  regressions.
