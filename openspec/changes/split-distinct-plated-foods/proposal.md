## Why

Recognize can currently return "Тушёная капуста с нежирной белой рыбой" (stewed cabbage with
lean white fish) as a single item for a photo that shows two distinct foods — a vegetable side
and a protein — served on the same plate. The "Composite Dish Naming" requirement
(`openspec/specs/food-photo-recognition/spec.md`, added by
`openspec/changes/archive/2026-08-13-composite-food-recognition/`) only triggers a split when
the photo shows "distinct piles on a plate, or separate foods placed next to each other" —
i.e. it keys off *spatial* separation. A protein and a vegetable side that are plated touching,
or with the protein resting on/against the vegetable, read to the model as one scene rather than
two placed-apart piles, even though a user logging this meal thinks of it as two foods: "stewed
cabbage" and "fish", each with its own macros. This under-splitting was not the failure mode the
prior change targeted — that change fixed over-decomposition of one dish into its raw
ingredients (e.g. "beans, corn, tomato" out of a single mixed side) — but the wording it landed
is now also suppressing splits the user does want.

The fix is a boundary change to that same requirement (and the shared Recognize/Clarify prompt
that implements it), moving the test from *spatial* separation to *identity* separation: split
when the photo shows two or more foods that are each independently namable, regardless of
whether they share a role (protein vs. side is the common case, but two different vegetable
sides plated touching split the same way) and regardless of whether they are plated apart or
touching/adjacent — but keep them merged when the visible mass is a single homogeneous
preparation whose ingredients have no independent identity once cooked together (a stew, curry,
stir-fry, sauced mixture, mixed salad), which is the case the prior change was protecting. A
minor garnish or condiment (a lemon wedge, a sprig of herbs, a spoonful of sauce) that isn't
itself a portion-sized food stays folded into the item it accompanies rather than becoming its
own item, even though it is technically independently namable.

## What Changes

- The "Composite Dish Naming" requirement is amended: the split/merge boundary changes from
  "spatially separate on the plate" to "independently identifiable foods", so a protein served
  touching or on top of a vegetable/starch side (e.g. fish on stewed cabbage) is returned as two
  items, while a single homogeneous preparation (a stew, curry, stir-fry, or sauced/mixed dish
  where no individual ingredient is separately identifiable) is still returned as one item,
  exactly as today. The boundary is identity-based, not role-based: two independently namable
  foods split whether or not they play different roles on the plate. A minor garnish or
  condiment stays folded into its main item rather than becoming its own item.
- The shared Recognize/Clarify system prompt (`backend/pkg/vision/openai.go`,
  `recognizeSystemPrompt`) is reworded to state this boundary directly, replacing the
  "distinct piles ... or separate foods placed next to each other" phrasing with guidance keyed
  on whether each component keeps its own identity, with a worked example distinguishing "fish
  served on/next to stewed cabbage" (two items) from "a stir-fry where vegetables and protein
  are cooked and sauced together" (one item).
- No schema, database, or frontend change: `RecognizeResult.Items` is already an array, item
  persistence already loops over N items, and the review UI already renders N items per meal
  (confirmed in this Attempt's research — see Task 1 findings in the originating plan). This is
  a prompt/spec-wording change only.
- `Clarify` shares the same system prompt, so it gets the same boundary automatically; no
  separate prompt is introduced. Whether a clarification round needs additional handling to
  avoid re-merging items a first pass already split is addressed by task 3 below (expected to
  need no code change, since Clarify's schema and item-count constraints are unchanged — but is
  called out explicitly rather than assumed).

## Capabilities

### New Capabilities
(none)

### Modified Capabilities

- `food-photo-recognition`: "Composite Dish Naming" is amended so the item-splitting boundary
  is identity-based (independently namable foods, regardless of role) rather than purely
  spatial, while continuing to keep a single homogeneous preparation as one item and folding
  minor garnishes/condiments into their main item.

## Impact

- Backend: `backend/pkg/vision/openai.go` (`recognizeSystemPrompt` wording only — no schema
  change).
- Tests: `backend/pkg/server/food_upload_test.go` currently has no fixture that constructs a
  multi-item `Recognize` response (confirmed in this Attempt's research); the implementer adds
  one exercising multi-item persistence, which today is structurally supported but untested.
- No frontend, database, or API-schema change.
