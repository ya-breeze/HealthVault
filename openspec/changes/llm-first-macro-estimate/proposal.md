## Why

Investigating a real production bug (meal `ad5e12e6-a84e-4120-a8ea-1090ab1d3cc9`, 2026-08-17) showed that a "Lean white fish" item was correctly estimated by the vision model at 26g protein/100g, but the candidate-selection call then matched it to USDA `fdc_id 170130` ("Squash, winter, butternut, cooked, baked, with salt", 0.9g protein/100g) — a shortlist pick that made no nutritional sense, even though several genuine fish entries ranked higher in the very same shortlist. Under the current precedence, any Select-LLM match unconditionally overrides the model's own estimate, so this single bad pick silently understated the meal's protein by ~25x. A DB sweep over the full `food_items` table found this is the only occurrence of a >4x estimate-vs-reference divergence historically — a rare but real Select-step failure mode, not a systemic retrieval bug (the query construction and ranking in `usda/query.go` worked correctly; the good candidates were present and ranked above the bad one).

## What Changes

- **BREAKING (behavior)**: Flip the default macro precedence for items resolved through automatic candidate selection (the `Select` LLM call over USDA/OFF/ranked-custom-food candidates). The vision model's own `estimated_profile` (already parsed and persisted today) becomes the primary macro source whenever it is present and passes a plausibility check — a selected reference candidate is now consulted only as a *fallback*, used when the item carries no usable estimate, or its estimate fails the plausibility check.
- Add a resolution-time plausibility check for `estimated_profile` beyond today's non-negativity check: protein, carbs, and fat must each be at most 102g/100g and total at most 102g/100g (the same 2g rounding allowance applied to both); sugar plus fiber together may exceed carbs by at most 2g/100g (checked as a combined sum, since both are subsets of carbs — checking them independently would miss a profile where each passes alone but their sum doesn't); and declared calories may fall below the protein/carbs/fat Atwater calculation by at most the larger of 25 kcal or 15% of that calculation. An estimate failing this check is treated the same as "no usable estimate" for automatic-resolution precedence purposes.
- Preserve unconditional precedence for the two cases where a match is a *deterministic identity match* rather than a fuzzy Select-LLM guess:
  - An exact case-insensitive match against one of the user's own saved `CustomFood` entries (already short-circuits before `Select` is even meaningfully exercised).
  - Any reference explicitly supplied by the caller (`fdc_id`, `off_code`, or `custom_food_id` on `PATCH .../items/{id}`, item-add, or manual meal creation) — these never went through `Select` at all and are untouched by this change.
- A *ranked* (fuzzy, non-exact-name) custom-food candidate that `Select` picks is no longer treated as authoritative either — it now follows the same new precedence as a USDA/OFF pick, since it is still a model guess rather than an asserted identity. This is a deliberate extension of the same principle and is called out explicitly for review.
- No new `macro_source` value or DB migration is needed: `estimated` already means "scaled from Recognize's own persisted per-100g estimate," which remains true regardless of *why* a reference candidate wasn't used.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `food-photo-recognition`: "Macro Estimate Fallback for Unmatched Items" changes from "the estimate is used only when nothing matched" to "the estimate is used by default whenever present and plausible; a Select-matched candidate is the fallback." Adds a plausibility check as a new precondition for treating an estimate as usable.
- `usda-nutrition-database`: "Match Selection and Explicit Non-Match" changes the "Scenario: Candidate selected from shortlist" outcome — selecting a candidate no longer unconditionally binds and scales from it; it now applies only when the item's own estimate is absent or implausible, except for the exact-name custom-food candidate, which keeps today's unconditional behavior.
- `data-model`: Updates the `estimated` macro-source definition from an unmatched-only fallback to a photo-derived estimate selected during automatic resolution, and makes explicit that a discarded fuzzy candidate leaves all reference identity fields unset.

## Impact

- `backend/pkg/server/food_upload.go`: `resolveItems`, `retrieveCandidates` (needs to signal when a returned candidate list is the exact-name custom-food short-circuit vs. a fuzzy shortlist), and the post-`Select` binding step.
- `backend/pkg/database/models_food.go`: add a resolution-time plausibility helper alongside the existing `EstimatedProfile()` accessor; `ApplyProfile` / `ApplyEstimatedProfile` call sites get reordered per the new precedence, no schema change. Keeping the existing accessor's validity semantics preserves correct rescaling for already-persisted `macro_source = estimated` rows after deployment.
- No frontend changes — the review UI already renders whatever `macro_source` and macros ended up on the item, source-agnostic.
- `openspec/specs/food-model-calibration`: calibration tooling compares predictions against ground truth per item; worth a quick check that it doesn't hardcode an assumption that a "reference" match is always the ground-truth-preferred source, but no functional change to the tooling itself is expected.
