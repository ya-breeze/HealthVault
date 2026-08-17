## 1. Plausibility check on estimated profiles

- [x] 1.1 Add a resolution-time plausibility helper alongside `FoodItem.EstimatedProfile()` (`backend/pkg/database/models_food.go`). Keep `EstimatedProfile()` and `ApplyEstimatedProfile()`'s existing present/non-negative semantics so legacy `macro_source = estimated` rows still rescale on weight-only PATCH. The new helper SHALL reject an estimate when: any of protein/carbs/fat exceeds 102g/100g (same 2g rounding tolerance as the combined check, not a bare 100g cap); their sum exceeds 102g/100g; sugar plus fiber together (summed, not checked independently — both are subsets of carbs) exceed carbs by more than 2g/100g; or declared calories are below `atwater - max(25, atwater×0.15)`, where `atwater = protein×4 + carbs×4 + fat×9`. The calorie check is one-sided — declared calories exceeding Atwater is not a violation because alcohol-containing foods can legitimately have calories the tracked macros do not capture.
- [x] 1.2 Unit tests: a normal food passes; each individual bound violation (individual macro >102g, combined macros >102g, sugar+fiber sum > carbs+2g, calories below the exact one-sided threshold) is rejected; both sides of each tolerance boundary are covered; a profile where sugar and fiber each individually satisfy ≤carbs+2g but their sum does not (e.g. carbs=10g, sugar=12g, fiber=12g) is rejected by the combined check; and legitimate foods pass — protein powder (~80g protein/100g), pure oil (~100g fat/100g, ~100.4g after rounding, within the 102g individual tolerance), dry gelatin, and a food whose calories exceed its Atwater figure (e.g. wine: ~83 kcal/100g, ~0g protein/fat, ~2.6g carbs).
- [x] 1.3 Regression test: a legacy item already carrying `macro_source = estimated` and a persisted profile that fails the new resolution-time plausibility helper can still be weight-edited and has all macros rescaled from that persisted profile rather than retaining totals from the old weight.

## 2. Signal deterministic vs. fuzzy candidates

- [x] 2.1 Change `retrieveCandidates` (`backend/pkg/server/food_upload.go`) to also return whether the shortlist it built is the exact-name custom-food short-circuit (deterministic) or a fuzzy (ranked-custom-food/OFF/USDA) shortlist.
- [x] 2.2 Thread that flag through to wherever `Select`'s chosen candidate is bound, so the binding step knows which case it's in per item.

## 3. Flip the binding precedence

- [x] 3.1 In `resolveItems`, change the post-`Select` binding logic so that: an exact-name deterministic match still calls `ApplyProfile` unconditionally; a fuzzy-matched candidate only calls `ApplyProfile` when the item has no resolution-usable estimate (per the new plausibility helper); otherwise a resolution-usable estimate calls `ApplyEstimatedProfile` even though a fuzzy candidate was selected — in this case leave the item's `FdcID`/`OffCode`/`CustomFoodID` nil (do not stamp the discarded candidate's identity), so a rejected ranked-custom-food pick can't inflate that food's future `rankedCustomFoodCandidates` usage score once the meal is confirmed (see design.md Decision 2); no candidate and no resolution-usable estimate remains `macro_source = none`, unchanged.
- [x] 3.2 Confirm caller-supplied reference paths (PATCH item, item-add, manual meal creation in `food_upload.go`/`food_manual.go`) are untouched — they don't go through `retrieveCandidates`/`Select` and should need no code change; add/keep a test asserting this explicitly so a future refactor can't accidentally route them through the new precedence check.
- [x] 3.3 Unit/integration tests reproducing the reported bug shape: an item whose `Select` call picks a fuzzy candidate with wildly different macros than its own usable estimate ends up `macro_source = estimated` using the model's numbers, not the fuzzy candidate's.
- [x] 3.4 Unit/integration test: an item with an implausible estimate (fails the new plausibility check) and a suitable fuzzy candidate ends up `macro_source = reference` using the candidate's numbers.
- [x] 3.5 Unit/integration test: an item matched via the exact-name custom-food short-circuit uses that food's macros regardless of what its own estimate says.
- [x] 3.6 `TestCreateMeal_MatchedCandidateDiscardsUnusedEstimate` (`backend/pkg/server/food_upload_test.go:787-819`) currently uses an implausible estimate (500 protein/cal per 100g) to assert a matched candidate wins — under the new logic it will still pass, but for the "implausible estimate falls back" reason, not the "matched candidate always wins" reason its name/comment currently states. Rename/re-comment it accordingly, and add a companion test with a *plausible* competing estimate to actually cover the new default-to-estimate behavior (this duplicates 3.3 in spirit — fine, keep both for clarity at the call site being modified).

## 4. Verify no unintended blast radius

- [x] 4.1 Read `openspec/specs/food-model-calibration` and its implementation to confirm ground-truth comparison there doesn't assume `macro_source = reference` is always the higher-trust source; adjust only if it actually does.
- [x] 4.2 Grep the frontend for any logic keyed on `macro_source` values beyond display styling (e.g. sort order, badges) to confirm nothing assumes `reference` implies higher confidence than `estimated` going forward.

## 5. Validate end-to-end

- [x] 5.1 Run `make lint` / `go vet` / existing Go test suite in `backend/`.
- [ ] 5.2 Deploy branch to `hcw-wip` (per repo workflow) and manually re-run a photo upload similar to the reported case (or replay the same image if available) to confirm the item now surfaces the model's own plausible protein estimate.
- [ ] 5.3 Run backend food-logging tests / any relevant e2e coverage against the deployed WIP stack; note explicitly if this project has no E2E suite for this flow per the repo's E2E-testing rule.
