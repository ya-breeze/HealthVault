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
- Else if the item has a resolution-usable, plausible estimate → `ApplyEstimatedProfile` (new default for the fuzzy-match case). Crucially, the item's `FdcID`/`OffCode`/`CustomFoodID` fields are left nil in this case — `Select`'s pick is discarded entirely, not partially recorded. If a fuzzy-matched `CustomFoodID` were stamped onto the item anyway (macros from the estimate, identity from the discarded candidate), it would make that custom food count as "used" for `rankedCustomFoodCandidates`'s frequency/recency scoring (`food_upload.go:404-419`, which filters on `custom_food_id IS NOT NULL AND status=confirmed`, not on `macro_source`) even though its macros were never actually applied — quietly reinforcing a match that was in fact rejected, the same class of problem the existing "an unconfirmed match does not inflate its own future ranking" scenario already guards against, just through a different door.
- Else if `Select` picked a fuzzy candidate → `ApplyProfile` (fallback, same call as today, just reached under a narrower condition).
- Else (no candidate, no usable estimate) → `macro_source = none`, unchanged.

This keeps the change localized to the binding decision in `resolveItems`; `applyScaledProfile`, `ApplyProfile`, and `ApplyEstimatedProfile` themselves don't need to change beyond Decision 3 below.

**Decision 3: Keep persisted-estimate validity separate from automatic-resolution plausibility.**
`EstimatedProfile()` (`models_food.go:154`) remains the accessor for a present, valid (non-negative) persisted estimate. Add a separate resolution-time helper (for example, `PlausibleEstimatedProfile()`) that first calls `EstimatedProfile()` and then applies the bounds below. `resolveItems` uses the new helper when deciding whether an estimate beats a fuzzy candidate.

This distinction preserves existing rows safely. A row already stored with `macro_source = estimated` can predate this heuristic and can therefore fail the new plausibility bounds. The weight-only PATCH path calls `ApplyEstimatedProfile()` and currently ignores its boolean return; making `EstimatedProfile()` globally stricter would let that PATCH persist a new weight while silently retaining macro totals scaled for the old weight. Leaving `EstimatedProfile()`/`ApplyEstimatedProfile()` validity semantics unchanged means legacy estimated rows continue to rescale consistently, while every new automatic-resolution decision still goes through the stricter helper. Clarification may continue carrying the persisted profile forward; the subsequent `resolveItems` pass re-applies the plausibility check before choosing it as the macro source.

Plausibility bounds (deliberately generous — this is a backstop against nonsense, not a precision filter):
- No individual per-100g macro (protein, carbs, fat) exceeds 102g, and their sum does not exceed 102g. Both use the same 2g rounding tolerance on the 100g physical ceiling — an earlier draft left the individual cap at exactly 100g with no tolerance while the combined cap had one, which would have rejected a legitimate ~100g/100g pure-oil estimate (fat ≈100.4g after rounding) on the individual check alone even though it passes the combined one. The combined bound still rejects internally impossible profiles such as 80g protein + 80g carbs + 80g fat per 100g of food.
- `sugar_per_100g + dietary_fiber_per_100g` does not exceed `carbs_per_100g` by more than 2g. Sugar and fiber are both subsets of total carbs, so they must be checked together, not separately: checking each against `carbs_per_100g` independently (as an earlier draft did) lets a profile like carbs=10g/sugar=12g/fiber=12g pass both individual checks (12 ≤ carbs+2 each) while sugar+fiber=24g is still physically impossible for a 10g-carb food.
- Let `atwater = protein*4 + carbs*4 + fat*9` and `calorie_tolerance = max(25 kcal, atwater*0.15)`. Reject only when `calories_per_100g < atwater - calorie_tolerance`. This check is intentionally one-sided: declared calories exceeding the macro-derived figure is normal and expected (alcohol contributes ~7 kcal/g and isn't tracked as a macro here, and there's ordinary rounding slop), so an estimate is never rejected for having *more* calories than its P/C/F imply — only for having *fewer*, which is the internally-inconsistent direction. A two-sided tolerance was considered and rejected because it flags correct estimates for wine, beer, and alcohol-reduction sauces purely because their alcohol calories aren't captured by P/C/F.

**Decision 4: Ranked (non-exact-name) custom-food candidates are treated as fuzzy, not exempted.**
The proposal calls this out as an extension of the same principle beyond what was literally reported: a frequency/recency-ranked custom food is still a `Select` guess, not an assertion that this specific plate is that specific saved food. Flagged explicitly for reviewer sign-off since it wasn't the reported symptom, but the alternative (special-casing it) would reintroduce exactly the ambiguity — "the model picked something plausible-looking that isn't actually right" — this change exists to fix.

## Risks / Trade-offs

- **[Risk] A genuinely correct `Select` match now loses to a slightly-off model estimate more often** (e.g. model says 24g protein/100g for a food where the true reference value is 27g/100g — both plausible, but the reference was actually more accurate). → **Mitigation**: this is an accepted trade-off directly requested by the product owner: the observed failure mode (reference match catastrophically wrong while estimate was right) is judged worse than the new one (estimate mildly imprecise while a correct reference existed), and the user still reviews/edits items before confirming — see `food-nutrition-logging` "Item Resolution" for the existing correction path (which also lets a correction be saved as a reusable `CustomFood`, improving future exact-name matches).
- **[Risk] Plausibility bounds are heuristic and could reject a legitimately unusual food** (e.g. protein powder near 80g protein/100g is still under the 100g bound and fine, but something built from odd chosen tolerances could false-positive). → **Mitigation**: bounds are intentionally wide (backstop, not precision filter); tasks include unit tests against real high-protein/high-fat foods (protein powder, pure oil, dry gelatin) to confirm the bounds don't reject legitimate values.
- **[Risk] `food-model-calibration` tooling may implicitly assume "reference" is always the higher-trust source when scoring predictions against ground truth.** → **Mitigation**: task to grep/read that tooling and confirm; no functional change expected there since ground-truth comparison is presumably keyed on the final resolved profile/macro values, not on `macro_source`, but this needs verifying rather than assuming.

## Migration Plan

No data migration. This changes binding *logic* only; existing persisted `food_items` rows keep whatever `macro_source` they already have. Because the new plausibility predicate is separate from `EstimatedProfile()`/`ApplyEstimatedProfile()`, an existing estimated row continues to rescale from its stored profile on a later weight edit even if that profile would not pass the new automatic-resolution predicate. Deploy as a normal backend release. No rollback complexity beyond reverting the code change — no schema or data shape changes to undo.

## Open Questions

None outstanding — the two precedence ambiguities (fallback trigger definition; barcode-match handling) were resolved with the user before this design was written: fallback triggers on "no estimate or implausible estimate," and there is no automatic-barcode case to special-case (see Non-Goals).
