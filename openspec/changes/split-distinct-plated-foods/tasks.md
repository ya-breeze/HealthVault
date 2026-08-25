## 1. Prompt wording

- [ ] 1.1 Reword the composite-dish-naming paragraph of `recognizeSystemPrompt` in
  `backend/pkg/vision/openai.go` to key the split/merge decision on whether each visible
  component is an independently identifiable food of a different role (e.g. a protein vs. a
  vegetable/starch side), not on whether they are spatially separated on the plate.
- [ ] 1.2 Add a worked example pair to the prompt text: a protein served touching or on top of a
  vegetable/starch side (e.g. "fish served on/next to stewed cabbage") should split into two
  items; a single homogeneous preparation where ingredients are cooked/sauced together and no
  longer independently identifiable (e.g. a stir-fry, curry, stew, or mixed salad) should remain
  one item.
- [ ] 1.3 Re-read the full prompt after editing to confirm the new wording does not contradict or
  weaken the still-desired merge behavior for genuinely homogeneous dishes (the original
  over-decomposition problem this requirement was added to fix).

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

## 5. Validation

- [ ] 5.1 Run backend unit tests (`make test` or equivalent) and confirm all pass.
- [ ] 5.2 Run `make lint` and fix anything it reports.
- [ ] 5.3 Deploy to the WIP/dogfood stack per the standard workflow and manually verify with a
  real photo similar to the "Тушёная капуста с нежирной белой рыбой" case: the plate is now
  split into two items (a vegetable/side item and a fish/protein item), and a control photo of a
  genuinely homogeneous dish (e.g. a stir-fry or stew) still returns as one item.
- [ ] 5.4 Run the existing Playwright E2E suite against the deployed stack and confirm no
  regressions.
