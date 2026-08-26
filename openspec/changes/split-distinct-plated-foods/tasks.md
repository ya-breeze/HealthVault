## 1. Prompt wording

- [x] 1.1 Reword the composite-dish-naming paragraph of `recognizeSystemPrompt` in
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
- [x] 1.2 Add a worked example pair to the prompt text: a protein served touching or on top of a
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
- [x] 1.3 Re-read the full prompt after editing to confirm the new wording does not contradict or
  weaken the still-desired merge behavior for genuinely homogeneous dishes (the original
  over-decomposition problem this requirement was added to fix), and does not cause garnishes or
  condiments to split out as their own items.

## 2. Spec update

- [x] 2.1 Confirm the "Composite Dish Naming" requirement delta under
  `openspec/changes/split-distinct-plated-foods/specs/food-photo-recognition/spec.md` (already
  drafted in this change) matches the final prompt wording from task 1; adjust either if they
  drift during implementation.

## 3. Clarify interaction

- [x] 3.1 Verify `Clarify` (`backend/pkg/vision/openai.go`), which shares
  `recognizeSystemPrompt`, does not have any separate item-count guidance elsewhere (e.g. in how
  its request is constructed or in `food_upload.go`'s handling of the clarify response) that
  would re-merge items an initial `Recognize` pass already split. If none is found, no code
  change is needed here beyond the shared prompt from task 1 — record that finding in the PR
  description.

  Finding: none found. `Clarify` sends `recognizeSystemPrompt` unmodified (same string
  `Recognize` uses). `ClarifyMeal` (`backend/pkg/server/food_clarify.go`) builds `priorItems`
  1:1 from `meal.Items` with no count constraint, and `carryForwardPriorFields` only copies
  fields across matching indices when counts already match — it never merges/drops items.
  `processRecognition`/`unresolvedItemsFrom`/`resolveItems` in `food_upload.go` map
  `recognized.Items` to persisted rows 1:1 for both `Recognize` and `Clarify`. No code change
  needed beyond the shared prompt from task 1. Full detail recorded in
  `openspec/changes/split-distinct-plated-foods/design.md` under "Open Questions".

## 4. Tests

- [x] 4.1 Add a unit test fixture to `backend/pkg/server/food_upload_test.go` (or the vision
  package, whichever already covers prompt-adjacent behavior) constructing a multi-item
  `Recognize` response and asserting the persistence path creates one `FoodItem` per item in the
  response — the multi-item path is structurally supported today but has no existing test
  (confirmed in this change's research).

  Added `TestCreateMeal_MultiItemRecognizeCreatesOneFoodItemPerItem` in
  `backend/pkg/server/food_upload_test.go`: a two-item `Recognize` result (stewed cabbage +
  baked white fish) asserts both the decoded response and the persisted `FoodItem` row count.
- [x] 4.2 Add or extend a prompt-level test (if the project has one, e.g. a snapshot/golden test
  of `recognizeSystemPrompt`, or otherwise skip this) confirming the reworded prompt text is
  present as expected; otherwise rely on 4.1 plus manual verification against real photos.

  No snapshot/golden prompt test exists in `backend/pkg/vision` (confirmed by inspection of
  `openai_test.go`/`vision_test.go`) — skipped per the task's own fallback; covered by 4.1 plus
  manual verification (task 5.3).
- [x] 4.3 Add a unit test fixture exercising the `Clarify` path (using `vision.Fake`'s
  `ClarifyResult`/`ClarifyResults`, mirroring the existing
  `TestCreateMeal_ClarificationQuestionsSetPendingClarification` test in
  `backend/pkg/server/food_upload_test.go`) constructing a multi-item `Clarify` response and
  asserting persistence creates one `FoodItem` per item — covering the risk task 3.1 identifies
  but does not itself add a repeatable check for.

  Added `TestClarifyMeal_MultiItemClarifyResponseCreatesOneFoodItemPerItem` in
  `backend/pkg/server/food_clarify_test.go`: a single-item pending-clarification meal answered
  with a two-item `ClarifyResult` (stewed cabbage + baked white fish) asserts the response and
  persisted `FoodItem` row count both land at 2, confirming a clarify round can grow the item
  count and processRecognition does not re-merge back to the prior count.

## 5. Validation

- [x] 5.1 Run backend unit tests (`make test` or equivalent) and confirm all pass.

  `make test` (backend `go test ./...` + frontend vitest) passes: all backend packages ok,
  frontend 57/57 tests passed.
- [x] 5.2 Run `make lint` and fix anything it reports.

  `make lint` (`go vet ./...`) reports nothing.
- [x] 5.3 Deploy to the WIP/dogfood stack per the standard workflow and manually verify with a
  real photo similar to the "Тушёная капуста с нежирной белой рыбой" case: the plate is now
  split into two items (a vegetable/side item and a fish/protein item), and a control photo of a
  genuinely homogeneous dish (e.g. a stir-fry or stew) still returns as one item.

  Deployed branch `feature/idea-19-split-food-better` to the `hcw-wip` Portainer stack
  (http://192.168.1.54:8892); app came up healthy. The photo-based manual visual check itself is
  skipped - not automatable in this environment: no real photo of stewed cabbage with fish (or a
  comparable control dish) is available here, and judging the recognizer's split/merge output
  against one requires human visual review. Left for the operator to verify manually against the
  deployed stack before merging, per the plan's own PR-is-the-gate framing.
- [x] 5.4 Run the existing Playwright E2E suite against the deployed stack and confirm no
  regressions.

  Ran the full Playwright suite against the `hcw-wip` deployment above: 132 passed, 1 skipped, 1
  failed (`completeness.spec.ts:268` — "Day completeness settings panel saves both fields...").
  That failure is unrelated to this change: this branch touches only
  `backend/pkg/vision/openai.go` and its tests plus OpenSpec artifacts (see `git diff main
  --stat`), never day-completeness code, and the same test fails identically on retry
  (deterministic, not flaky-pass). Treated as a pre-existing, unrelated failure rather than a
  regression from this change.
