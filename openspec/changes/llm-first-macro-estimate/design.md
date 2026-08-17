## Context

Today's automatic food-logging pipeline (`backend/pkg/server/food_upload.go`, `resolveItems`) does, per recognized item:

1. `retrieveCandidates` builds a shortlist: an exact case-insensitive `CustomFood` name match short-circuits as the sole candidate; otherwise a frequency/recency-ranked custom food plus USDA or Open Food Facts (OFF) fuzzy search results are combined.
2. One `Select` LLM call picks a `candidate_index` (or -1) per item from its shortlist.
3. If a candidate was picked, `ApplyProfile` scales the item's macros from that candidate's DB profile and sets `macro_source = reference` — unconditionally, discarding the model's own `estimated_profile` even if one exists (`models_food.go:265` `ApplyEstimatedProfile` is currently only called for items where `Select` returned no match at all).

The bug this change addresses: for one real meal, `Select` picked a USDA row (butternut squash) for an item recognized and correctly estimated as fish, even though several correct fish rows ranked higher in the same shortlist it was given. The model's own estimate (26g protein/100g) was right; the reference bind it was still made to use was wrong. This wasn't a retrieval/ranking bug — it was the `Select` call itself making a bad pick from a good shortlist — and a DB sweep confirms it's rare (the only >4x divergence in the historical data), not systemic.

## Goals / Non-Goals

**Goals:**
- Make the vision model's own per-item estimate the macro source of record whenever it exists and is plausible, for every item resolved through automatic candidate selection.
- Demote a `Select`-picked reference candidate to a fallback, used only when the item has no usable estimate.
- Add a plausibility check so an estimate that's *itself* nonsensical (e.g. impossible per-100g values) doesn't get used just because it's present — falling through to the reference fallback in that case instead.
- Leave every deterministic identity match (exact custom-food name; any caller-supplied `fdc_id`/`off_code`/`custom_food_id`) exactly as authoritative as it is today — this change is scoped to matches that came from the `Select` LLM guessing over a fuzzy shortlist.

**Non-Goals:**
- No barcode-scanning capability is being added. Note for reviewers: there is no automatic barcode-from-photo path in this app today — the OFF branch of automatic resolution (`h.off.Search(name, brand, ...)`) is a fuzzy name+brand search, not a lookup by scanned code (`ByCode` is only ever called for a caller-supplied `off_code`). So "OFF should stay authoritative for a barcode match" collapses to "caller-supplied `off_code` stays authoritative," which is already true and needs no code change.
- No change to shortlist construction, ranking, or the `Select` prompt itself. The shortlist and ranking already worked correctly in the investigated incident; the fix is about what happens *after* `Select` answers, not about improving its answer.
- No new `macro_source` value and no DB migration. `estimated` already means "scaled from Recognize's own persisted per-100g estimate" — that meaning doesn't change, only *when* it's chosen does.
- No change to manual meal creation, item PATCH, or item-add endpoints — those bind directly to a caller-supplied reference today and are untouched.

## Decisions

**Decision 1: Signal "deterministic vs. fuzzy" explicitly from `retrieveCandidates`, don't infer it.**
`retrieveCandidates` already knows, at the point it returns, whether it took the exact-name-match short-circuit branch or the fuzzy ranked/OFF/USDA branch. Change its return shape to carry that explicitly (e.g. a wrapping struct/second return value such as `exact bool`) rather than letting the caller guess from candidate-list shape (e.g. "length 1" is not a safe proxy — a fuzzy USDA search can also legitimately return exactly one candidate). The post-`Select` binding step uses this flag: an exact-match bind stays unconditionally authoritative; any other bind is subject to the new precedence check against the item's own estimate.

**Decision 2: Precedence check lives at the same point `ApplyProfile`/`ApplyEstimatedProfile` are currently chosen between, in `resolveItems`.**
For each item after `Select` returns:
- If the bind is an exact-identity match → `ApplyProfile` (unconditional, unchanged).
- Else if the item has a usable, plausible estimate → `ApplyEstimatedProfile` (new default for the fuzzy-match case).
- Else if `Select` picked a fuzzy candidate → `ApplyProfile` (fallback, same call as today, just reached under a narrower condition).
- Else (no candidate, no usable estimate) → `macro_source = none`, unchanged.

This keeps the change localized to the binding decision in `resolveItems`; `applyScaledProfile`, `ApplyProfile`, and `ApplyEstimatedProfile` themselves don't need to change beyond Decision 3 below.

**Decision 3: Extend `FoodItem.EstimatedProfile()`'s usability check with a plausibility bound, not a separate method.**
`EstimatedProfile()` (`models_food.go:154`) already returns `(profile, false)` for a present-but-invalid (negative) estimate, which every caller already treats as "no usable estimate." Adding the plausibility check to the same method means every existing caller (including the unresolved-item fallback loop, which has no code changes) picks up the stricter check for free, and "usable" keeps one definition instead of two.

Plausibility bounds (deliberately generous — this is a backstop against nonsense, not a precision filter):
- No per-100g macro (protein, carbs, fat) exceeds 100g — a food can't contain more than 100g of a macro per 100g of itself.
- `sugar_per_100g` does not exceed `carbs_per_100g`, and `dietary_fiber_per_100g` does not exceed `carbs_per_100g`, each by more than a small tolerance (e.g. 2g) to absorb model rounding — both are subsets of carbs.
- Declared `calories_per_100g` is within a wide tolerance (e.g. the larger of ±30% or ±50 kcal) of the Atwater estimate `protein*4 + carbs*4 + fat*9`. This is the check that would have caught the fish/squash-adjacent class of error if it had occurred on the *estimate* side instead of the *reference* side — it exists here for symmetry and because a model estimate can independently be internally inconsistent (e.g. plausible-looking numbers whose calorie total doesn't add up).

**Decision 4: Ranked (non-exact-name) custom-food candidates are treated as fuzzy, not exempted.**
The proposal calls this out as an extension of the same principle beyond what was literally reported: a frequency/recency-ranked custom food is still a `Select` guess, not an assertion that this specific plate is that specific saved food. Flagged explicitly for reviewer sign-off since it wasn't the reported symptom, but the alternative (special-casing it) would reintroduce exactly the ambiguity — "the model picked something plausible-looking that isn't actually right" — this change exists to fix.

## Risks / Trade-offs

- **[Risk] A genuinely correct `Select` match now loses to a slightly-off model estimate more often** (e.g. model says 24g protein/100g for a food where the true reference value is 27g/100g — both plausible, but the reference was actually more accurate). → **Mitigation**: this is an accepted trade-off directly requested by the product owner: the observed failure mode (reference match catastrophically wrong while estimate was right) is judged worse than the new one (estimate mildly imprecise while a correct reference existed), and the user still reviews/edits items before confirming — see `food-nutrition-logging` "Item Resolution" for the existing correction path (which also lets a correction be saved as a reusable `CustomFood`, improving future exact-name matches).
- **[Risk] Plausibility bounds are heuristic and could reject a legitimately unusual food** (e.g. protein powder near 80g protein/100g is still under the 100g bound and fine, but something built from odd chosen tolerances could false-positive). → **Mitigation**: bounds are intentionally wide (backstop, not precision filter); tasks include unit tests against real high-protein/high-fat foods (protein powder, pure oil, dry gelatin) to confirm the bounds don't reject legitimate values.
- **[Risk] `food-model-calibration` tooling may implicitly assume "reference" is always the higher-trust source when scoring predictions against ground truth.** → **Mitigation**: task to grep/read that tooling and confirm; no functional change expected there since ground-truth comparison is presumably keyed on the final resolved profile/macro values, not on `macro_source`, but this needs verifying rather than assuming.

## Migration Plan

No data migration. This changes binding *logic* only; existing persisted `food_items` rows keep whatever `macro_source` they already have. Deploy as a normal backend release. No rollback complexity beyond reverting the code change — no schema or data shape changes to undo.

## Open Questions

None outstanding — the two precedence ambiguities (fallback trigger definition; barcode-match handling) were resolved with the user before this design was written: fallback triggers on "no estimate or implausible estimate," and there is no automatic-barcode case to special-case (see Non-Goals).
