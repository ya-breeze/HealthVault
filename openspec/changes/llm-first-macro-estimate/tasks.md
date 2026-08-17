## 1. Plausibility check on estimated profiles

- [ ] 1.1 Extend `FoodItem.EstimatedProfile()` (`backend/pkg/database/models_food.go`) so that, in addition to the existing non-negativity check, it rejects an estimate where any of protein/carbs/fat per-100g exceeds 100g, sugar or fiber per-100g exceeds carbs per-100g by more than a small tolerance, or declared calories per-100g falls outside a wide tolerance of the Atwater calculation (protein×4 + carbs×4 + fat×9).
- [ ] 1.2 Unit tests: a normal food passes; each individual bound violation (macro >100g, sugar/fiber > carbs, calorie mismatch) is rejected; edge-case legitimate high-macro foods (protein powder ~80g/100g, pure oil ~100g fat/100g, dry gelatin) pass.

## 2. Signal deterministic vs. fuzzy candidates

- [ ] 2.1 Change `retrieveCandidates` (`backend/pkg/server/food_upload.go`) to also return whether the shortlist it built is the exact-name custom-food short-circuit (deterministic) or a fuzzy (ranked-custom-food/OFF/USDA) shortlist.
- [ ] 2.2 Thread that flag through to wherever `Select`'s chosen candidate is bound, so the binding step knows which case it's in per item.

## 3. Flip the binding precedence

- [ ] 3.1 In `resolveItems`, change the post-`Select` binding logic so that: an exact-name deterministic match still calls `ApplyProfile` unconditionally; a fuzzy-matched candidate only calls `ApplyProfile` when the item has no usable estimate (per the new `EstimatedProfile()` check); otherwise a usable estimate calls `ApplyEstimatedProfile` even though a fuzzy candidate was selected; no candidate and no usable estimate remains `macro_source = none`, unchanged.
- [ ] 3.2 Confirm caller-supplied reference paths (PATCH item, item-add, manual meal creation in `food_upload.go`/`food_manual.go`) are untouched — they don't go through `retrieveCandidates`/`Select` and should need no code change; add/keep a test asserting this explicitly so a future refactor can't accidentally route them through the new precedence check.
- [ ] 3.3 Unit/integration tests reproducing the reported bug shape: an item whose `Select` call picks a fuzzy candidate with wildly different macros than its own usable estimate ends up `macro_source = estimated` using the model's numbers, not the fuzzy candidate's.
- [ ] 3.4 Unit/integration test: an item with an implausible estimate (fails the new plausibility check) and a suitable fuzzy candidate ends up `macro_source = reference` using the candidate's numbers.
- [ ] 3.5 Unit/integration test: an item matched via the exact-name custom-food short-circuit uses that food's macros regardless of what its own estimate says.

## 4. Verify no unintended blast radius

- [ ] 4.1 Read `openspec/specs/food-model-calibration` and its implementation to confirm ground-truth comparison there doesn't assume `macro_source = reference` is always the higher-trust source; adjust only if it actually does.
- [ ] 4.2 Grep the frontend for any logic keyed on `macro_source` values beyond display styling (e.g. sort order, badges) to confirm nothing assumes `reference` implies higher confidence than `estimated` going forward.

## 5. Validate end-to-end

- [ ] 5.1 Run `make lint` / `go vet` / existing Go test suite in `backend/`.
- [ ] 5.2 Deploy branch to `hcw-wip` (per repo workflow) and manually re-run a photo upload similar to the reported case (or replay the same image if available) to confirm the item now surfaces the model's own plausible protein estimate.
- [ ] 5.3 Run backend food-logging tests / any relevant e2e coverage against the deployed WIP stack; note explicitly if this project has no E2E suite for this flow per the repo's E2E-testing rule.
