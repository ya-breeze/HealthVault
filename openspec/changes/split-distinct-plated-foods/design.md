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
- Fix under-splitting of independently identifiable foods that are plated touching or stacked,
  not just plated apart.
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

- **Identity separation replaces spatial separation as the split/merge test.** Two or more foods
  split when each remains independently namable and visually distinguishable, regardless of
  whether they're plated apart or touching. This is deliberately **role-agnostic**: a
  protein-and-side pairing is the common case, but two foods that share a role (e.g. two
  different cooked vegetable sides plated touching) split the same way, since the test is
  identity, not role difference. An earlier draft of this proposal keyed the rule on differing
  roles ("a protein vs. a vegetable/starch side"); that framing was narrower than the
  requirement's own general sentence and only covered the most common case, so the requirement
  text, its scenarios, and the task list were reconciled to state the general identity test
  first and use the protein/side pairing only as the worked example.
- **A minor garnish or condiment stays folded into its main item.** A lemon wedge, herb sprig, or
  spoonful of sauce is technically independently namable under the identity test, but it isn't a
  portion-sized food a user would log on its own — the requirement carves this out explicitly so
  the identity test doesn't over-trigger on trivial accompaniments the same way the original
  spatial test over-triggered on raw ingredients.
- **Scenario titles for the two pre-existing scenarios are kept verbatim** ("A single composite
  dish is recognized as one item", "A plate with visually separate components is still
  decomposed"), only their body text changes. OpenSpec's archive step matches `MODIFIED`
  scenarios against the canonical spec by exact title, not content — renaming a scenario title
  makes the archive tool treat the original as dropped rather than modified and aborts the
  merge. This was caught by running `scripts/generate-projected-specs.sh main` against an
  earlier draft that had renamed both titles; keeping the two original titles and only adding
  new scenarios for genuinely new behavior avoids it.

## Risks / Trade-offs

- [Risk] The identity-vs-homogeneous line is still a model judgment, not a mechanical rule —
  some real photos (e.g. a stir-fry with a few large, distinguishable protein chunks) will sit
  near the boundary either way. → Mitigated by the worked examples in the reworded prompt (task
  1.2) and the control-photo manual check in task 5.3, not eliminated; this is inherent to the
  problem (see the original 2026-08-13 change, which had the same trade-off in the other
  direction).
- [Risk] `Clarify` shares the prompt but runs as a text-only follow-up call; nothing currently
  tests that a multi-item `Clarify` response persists all items rather than silently collapsing
  them. → Task 4.3 adds that coverage using `vision.Fake`'s `ClarifyResult`/`ClarifyResults`.
- [Risk] Without a repeatable check, a later unrelated prompt edit could quietly reintroduce
  over-decomposition of homogeneous dishes (the original failure mode). → Task 5.3's control
  photo of a homogeneous dish exists precisely to catch this at implementation time; there's no
  CI-level guard beyond that, which is a known gap in this change's test plan rather than
  something silently missed.

## Open Questions

- Whether `Clarify`'s request construction or `food_upload.go`'s handling of the clarify
  response has any separate item-count guidance that could re-merge items a first `Recognize`
  pass already split — task 3.1 is the implementer's check for this; expected to find nothing,
  but not yet confirmed since no code has been written for this change.
