## Context

Recognize's system prompt (`recognizeSystemPrompt` in `backend/pkg/vision/openai.go`, shared
by both `Recognize` and text-only `Clarify` follow-ups) is the single place that decides how
many `Item`s a photo produces. `openspec/changes/archive/2026-08-13-composite-food-recognition/`
introduced the current "Composite Dish Naming" requirement to stop a genuinely homogeneous dish
(e.g. a mixed vegetable side) from being decomposed into its raw ingredients ("beans 30g, corn
30g, tomato 20g"), which defeated custom-food reuse. It did this by keying the split/merge
decision on *spatial* separation: only split when the photo shows "distinct piles on a plate, or
separate foods placed next to each other."

That wording now also suppresses splits a user does want: two visually and texturally distinct
foods served touching or resting on each other (a piece of white fish on a bed of stewed
cabbage) read as one scene rather than two placed-apart piles, so they merge into a single named
item even though a user logging the meal thinks of them as two foods with separate macros.

## Goals / Non-Goals

**Goals:**
- Fix under-splitting of separately-served foods that are plated touching or stacked, not just
  plated apart.
- Preserve the original fix: a genuinely homogeneous preparation (stew, curry, stir-fry, sauced
  or mixed dish) still returns as one item, not one item per ingredient.
- Keep the change to prompt/spec wording only — no schema, database, or frontend change, since
  `RecognizeResult.Items` is already an array and every downstream consumer (`resolveItems`,
  `persistAnalysis`, `ReviewClient.tsx`) already handles N items per meal.

**Non-Goals:**
- Not reintroducing raw-ingredient-level decomposition of a single cooked dish.
- Not adding a user-facing setting or per-upload toggle for split granularity — this stays a
  model judgment made per photo, as it is today.
- Not touching `Clarify`'s request construction or response handling — it shares the prompt, so
  it inherits the new boundary automatically; task 3 verifies there's no separate item-count
  guidance elsewhere that would undo that.

## Decisions

- **Separate-serving replaces spatial separation as the split/merge test.** Two or more foods
  split when each was served as its own separate portion, regardless of whether they're plated
  apart or touching. This is deliberately **role-agnostic**: a protein-and-side pairing is the
  common case, but two foods that share a role (e.g. two different cooked vegetable sides
  plated touching) split the same way, since the test is separate-serving, not role difference.
  An earlier draft of this proposal keyed the rule on differing roles ("a protein vs. a
  vegetable/starch side"); that framing was narrower than the requirement's own general sentence
  and only covered the most common case, so the requirement text, its scenarios, and the task
  list were reconciled to state the general test first and use the protein/side pairing only as
  the worked example.
- **The test is "was it served separately", not "can a piece be named".** An earlier draft
  phrased the merge/split boundary as "each remains independently namable and visually
  distinguishable" vs. "no individual component can be pointed to and named on its own." That
  wording contradicted its own examples: a chopped mixed salad's lettuce, tomato, and cucumber
  pieces are almost always individually namable and visually distinguishable, yet the
  requirement lists "a mixed salad" as a merge case. The requirement, its scenarios, and the
  task list were reworded so the test is whether a component was ever served as its own separate
  portion (split) versus mixed/chopped/cooked into one combined preparation whose ingredient
  pieces remain merely visually distinguishable, not separately served (merge) — this preserves
  the mixed-salad and stir-fry merge examples instead of contradicting them. Since Recognize only
  ever sees the photo, "was it served separately" is not literally about unobservable prep
  history — it is operationalized as visible form: a piece that still keeps its own
  separately-servable, portion-scale shape (an intact fillet, a distinct pile) counts as served
  separately, while a piece broken down, mixed, or tossed into one preparation does not. A sauce
  or another food's juices touching or coating a piece does not by itself change that — only
  losing the piece's own separately-servable shape does, the same grounding the "Sharing a
  cooking vessel does not by itself trigger a merge" decision below applies to the
  cooking-contact case specifically; it is stated here as the general rule.
- **Sharing a cooking vessel does not by itself trigger a merge.** An earlier draft left the
  "cooked ... together" merge trigger and the "served as its own portion" split trigger able to
  both fire on the same case: a whole fish fillet baked directly on top of stewed cabbage is
  arguably both "cooked ... together" (merge) and "each independently served as their own
  portion-scale food" (split) — which is the reported bug case, not an edge case, so leaving it
  genuinely ambiguous would have missed the fix's own target. The requirement now states the
  tie-breaker explicitly: physical cooking contact (same pan, pot, or oven dish) is not what
  defines a combined preparation; only the cooking process blending the pieces so that none
  remains in separately-servable, portion-scale form does. A whole fillet that keeps its shape
  and could be lifted off and plated on its own stays split even if it was cooked touching
  another food.
- **A minor garnish or condiment stays folded into its main item.** A lemon wedge, herb sprig, or
  spoonful of sauce is technically independently namable, but it isn't a portion-sized food
  served as its own portion — a user would not log it on its own — so the requirement carves
  this out explicitly so the separate-serving test doesn't over-trigger on trivial accompaniments
  the same way the original spatial test over-triggered on raw ingredients.
- **Scenario titles for the two pre-existing scenarios are kept verbatim** ("A single composite
  dish is recognized as one item", "A plate with visually separate components is still
  decomposed"), only their body text changes. OpenSpec's archive step matches `MODIFIED`
  scenarios against the canonical spec by exact title, not content — renaming a scenario title
  makes the archive tool treat the original as dropped rather than modified and aborts the
  merge. This was caught by running `scripts/generate-projected-specs.sh main` against an
  earlier draft that had renamed both titles; keeping the two original titles and only adding
  new scenarios for genuinely new behavior avoids it.

## Risks / Trade-offs

- [Risk] The separate-serving-vs-homogeneous line is still a model judgment, not a mechanical
  rule — some real photos (e.g. a stir-fry with a few large, distinguishable protein chunks) will
  sit near the boundary either way. → Mitigated by the worked examples in the reworded prompt (task
  1.2) and the control-photo manual check in task 5.3, not eliminated; this is inherent to the
  problem (see the original 2026-08-13 change, which had the same trade-off in the other
  direction).
- [Risk] `Clarify` shares the prompt but runs as a text-only follow-up call; nothing currently
  tests that a multi-item `Clarify` response persists all items rather than silently collapsing
  them. → Task 4.3 adds that coverage using `vision.Fake`'s `ClarifyResult`/`ClarifyResults`,
  which exercises the persistence path but not the real model's prompt-following behavior. To
  reduce the model-level risk (the shared prompt asks the model to judge split/merge from photo
  evidence it doesn't have on this call), the "since you only see the photo" phrasing was
  softened to not assert photo presence, and `Clarify`'s user message now explicitly instructs
  the model to keep `previously_recognized_items`' split as-is unless a clarification answer
  says otherwise — this is a prompt-level mitigation, not a hard guarantee, since the model could
  still deviate.
- [Risk] Without a repeatable check, a later unrelated prompt edit could quietly reintroduce
  over-decomposition of homogeneous dishes (the original failure mode). → Task 5.3's control
  photo of a homogeneous dish exists precisely to catch this at implementation time; there's no
  CI-level guard beyond that, which is a known gap in this change's test plan rather than
  something silently missed.

## Open Questions

- Resolved by task 3.1: no separate item-count guidance exists outside the shared prompt.
  `Clarify` (`backend/pkg/vision/openai.go`) sends `recognizeSystemPrompt` verbatim — the same
  string `Recognize` uses — with no additional system content constraining item count.
  `ClarifyMeal` (`backend/pkg/server/food_clarify.go`) builds `priorItems` 1:1 from `meal.Items`
  and passes `history` unchanged; nothing there caps or collapses the item count either.
  `carryForwardPriorFields` (same file) only copies `EstimatedProfile`/`CanonicalName` across
  when `len(priorItems) == len(recognizedItems)` and the corresponding names look like the same
  item — it never merges or drops items, and does nothing at all when the model's returned count
  differs from the prior round's. `processRecognition` and `unresolvedItemsFrom`
  (`backend/pkg/server/food_upload.go`) map `recognized.Items` to `database.FoodItem` rows 1:1
  with no count-collapsing logic, for both the `Recognize` and `Clarify` paths. No code change
  was needed beyond the shared prompt from task 1.
