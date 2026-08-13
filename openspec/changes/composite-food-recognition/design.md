## Context

The photo food pipeline (`backend/pkg/server/food_upload.go`) runs two LLM calls per upload:

1. **Recognize** (`backend/pkg/vision/openai.go`) — sends the photo, returns structured `vision.Item{Name, Preparation, State, Brand, WeightGrams, Confidence}` per recognized item, or clarification questions if ambiguous. Its system prompt currently instructs the model to identify "each distinct food item," which is why a single composite dish reliably comes back as several ingredient-level items.
2. **Select** (`backend/pkg/vision/openai.go`) — a separate, text-only, stateless call. For each recognized item, `retrieveCandidates` builds a shortlist (custom food exact-name match, else Open Food Facts, else USDA — per `usda-nutrition-database`'s "Match Selection and Explicit Non-Match") and Select picks an index from that shortlist, or -1 for no match.

There is no "meal vs. dish vs. ingredient" level in the domain model — only `FoodMeal` (the logged photo) and a flat `FoodItem` list. `CustomFood` is a per-user, per-100g nutrient profile matched today only by exact case-insensitive name (`usda-nutrition-database` spec). Because Recognize already decomposes into ingredients, a saved composite `CustomFood` such as "Mexican vegetable mix" essentially never gets a chance to match — there is rarely a recognized item named that.

## Goals / Non-Goals

**Goals:**
- Make composite-dish naming the default outcome of Recognize, without removing its ability to decompose a visibly multi-component plate.
- Let a user-entered `CustomFood` (e.g., macros copied from a package label) get reused automatically on a later, differently-worded photo of a similar dish — including dishes eaten only monthly, not just daily repeats.
- Give every item *some* usable macro value even when nothing matches, rather than leaving it fully unresolved.

**Non-Goals:**
- No new user-facing granularity toggle for the initial upload. Recognize's own visual judgment decides granularity; the existing post-hoc Expert reanalysis path is untouched.
- No new "recipe" or "meal template" entity with an internal ingredient breakdown kept alongside a composite display name. `CustomFood`'s existing single-profile shape is reused as-is.
- No denormalized usage counters (e.g. `usage_count` on `CustomFood`). Frequency/recency is computed by querying existing `FoodItem`/`FoodMeal` rows at candidate-retrieval time; this app has one family's worth of data, so a join at upload time is cheap and avoids an extra write-path to keep consistent.
- No third LLM call for macro estimation. It is folded into the existing Recognize call's response schema.

## Decisions

**1. Composite-dish naming is a prompt/schema change to Recognize, not a new mode.**
The system prompt changes from "identify each distinct food item" to instruct the model to name what it sees as a whole dish by default, and to only emit multiple items when it visually observes separately identifiable components (e.g., distinct piles on a plate, clearly separate foods). This is a judgment call left to the model on every photo — consistent with the user's explicit preference that granularity not be a manual per-upload choice.

**2. Custom-food candidate retrieval is widened, the exact-name short-circuit is kept.**
`retrieveCandidates` keeps its current case-insensitive exact-name lookup (if it hits, it still short-circuits directly to that `CustomFood`, unchanged — this is the cheap, unambiguous case). When it does not hit, the candidate list Select receives is extended with the user's own custom foods ranked by a frequency-weighted score, instead of jumping straight to Open Food Facts/USDA. Ranking query: for each of the user's `CustomFood` rows, count and last-use timestamp are computed from `FoodItem` rows (joined through `FoodMeal.user_id`) referencing that `custom_food_id`. Score = `usage_count * frequency_weight + recency_score`, with frequency weighted higher than recency specifically so a dish eaten once a month still outranks a one-off high-recency but never-repeated item; the exact weighting constants are an implementation detail tuned during `tasks.md`, not a spec-level requirement. Top-N (e.g. 5) by score are added to the candidate shortlist alongside any Open Food Facts/USDA candidates, and Select (already an LLM call reasoning over text descriptions, not exact string matching) picks among all of them. This means a "Mexican veggie mix" recognized item can still match a saved "Mexican vegetable mix" `CustomFood` even though the names differ, because Select reasons semantically, not lexically.

**3. Macro-source priority: custom food match > reference database match > model estimate.**
For the exact-name case this is a hard structural short-circuit, unchanged from today: an exact match is bound directly and no other source is even queried. For the frequency/recency-ranked case, the user's custom-food candidates are combined into the same shortlist as whatever Open Food Facts/USDA candidates the existing brand-based routing would already offer, and Select — a semantic, not lexical, judgment — is instructed to prefer the user's own previously-used food when it is a reasonable match, since it reflects verified real-world macros rather than a generic reference profile. This keeps the pipeline at one candidate-retrieval pass and one Select call per batch, unchanged from today. The estimate fallback (decision 4) only fires when Select returns no match at all from that combined shortlist.

**4. New `macro_source = estimated` value, folded into the same Recognize call.**
When Select finds no match for an item, the system falls back to a per-item macro estimate that Recognize already produced in its structured output (an optional per-item nutrient-profile field, requested unconditionally to avoid a second round-trip when no match will be found). This estimate is only actually used when no candidate exists; when a match is found, the estimate is populated but discarded. `MacroSource` becomes `reference | manual | estimated | none` (`none` now only occurs if Recognize itself fails to produce a per-item estimate, which existing error handling already treats as a resolvable/reviewable state). `estimated` counts as a usable macro source for the meal aggregate, exactly like `reference` and `manual` (`food-nutrition-logging`'s existing rule already excludes only `none`).

**5. UI surfaces `estimated` distinctly.**
The confirm-review screen, which already differentiates `reference`/`manual`/`none` items, gains a fourth visual treatment for `estimated` — communicating "AI guess, not a database fact" so the user is nudged to verify or correct it (and, if they correct it with real per-100g values, save it as a `CustomFood` so decision 2's ranking picks it up going forward).

## Risks / Trade-offs

- **[Risk]** The vision model may not reliably observe the "decompose only when visually separate" instruction, either still over-decomposing or now under-decomposing genuinely mixed plates. → Mitigation: this is a prompt-tuning risk inherent to any vision-prompt change; validate against the existing model-calibration samples (`food-model-calibration`) before merging, and the Expert reanalysis path remains available as a correction if a specific meal comes out wrong.
- **[Risk]** Frequency/recency ranking could resurface a stale or wrong custom food if the user has renamed their eating habits. → Mitigation: Select still reasons over the item's actual recognized description, not a blind top-1 pick, and the user can always override on the confirm-review screen.
- **[Risk]** `estimated` macros are model guesses and may be inaccurate, and users might not notice the distinct UI treatment and log a bad estimate as-is. → Mitigation: this is a strict improvement over the current behavior of contributing zero macros for an unmatched item; the confirm-review screen already requires an explicit confirm step where the estimate is visible before it's aggregated.
- **[Trade-off]** Asking Recognize for a per-item macro estimate on every call (even when a match will likely be found and the estimate discarded) costs some extra output tokens on every request, in exchange for avoiding a third LLM call only on the fallback path. Given Recognize already returns structured per-item JSON, this is a marginal cost accepted for simplicity.

## Migration Plan

No database migration: `macro_source` is a string-valued GORM field, not a DB-enforced enum, so adding `estimated` as a new value requires no schema change, only backend validation/handling updates and a frontend UI-label addition. This ships as a single backend+frontend deploy with no phased rollout — mismatched behavior between an old frontend and a new backend during deploy would only manifest as an unstyled/unlabeled (but functionally correct) `estimated` item, not a broken state, since `estimated` items still carry real numeric macros and existing generic item rendering already displays any macro values.

## Open Questions

- None outstanding; the frequency/recency weighting constants are an implementation detail to tune in `tasks.md`, not a design-level unknown.
