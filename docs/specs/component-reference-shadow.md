# Per-ingredient USDA/OFF breakdown as a background comparison field for composite dishes

Idea: ya-breeze/idea-forge#641

## Why

A composite or merged FoodItem (a salad, a curry, a stew — anything the model keeps as one item
under the "served as its own separate portion" rule from `split-distinct-plated-foods`) almost
never has a clean single USDA/OFF match for its whole name, so it stays `macro_source = estimated`
permanently: Luna's own per-100g guess (`HasEstimate`/`Estimated*Per100g`) is the only nutrient
data it will ever carry. That is most of what gets logged — a same-day audit of `hcw-prod` found
257 of 285 active items (90%) sitting at `estimated`, with zero way to check whether Luna's guess
on any of them is any good, unlike a `reference`-bound item, which already carries both Luna's own
estimate and the reference-database value on the same row for free (`applyScaledProfile` only
overwrites the main fields, never the `Estimated*` shadow — confirmed by inspection before this
spec was written, and the precedent this change extends).

The owner's proposal, discussed live 2026-09-18: have Recognize also return a rough
ingredient breakdown for a composite item — e.g. "cucumber and tomato salad" also returns
`ingredients: [{canonical_name_en: "cucumber", weight_grams: 100}, {canonical_name_en: "tomato",
weight_grams: 100}]` — purely so each ingredient can be searched against USDA/OFF individually and
the weighted sum stored as a **second, independent, USDA-grounded per-100g estimate**, directly
comparable to Luna's own `Estimated*Per100g`. This is explicitly a data-collection feature for
later ("накопится достаточно данных — сравнить и подумать что и как можно улучшить"), not a live
behavior change: it must not alter what the user sees, or which `macro_source`/values drive the
Healthiness Label, advice, chat, or any UI today. Same posture as `HasEstimate` already has.

A companion, narrower fix (Idea #640 — USDA/OFF search is silently skipped entirely for a
non-English `display_language`) and a one-off historical backfill are handled separately, outside
this repo change, by the owner directly — not part of this spec.

## How

### The ingredient breakdown itself

`vision.Item` gains `Ingredients []IngredientEstimate` (`name`, `canonical_name_en`,
`weight_grams`), requested in the **same** Recognize/Describe/Clarify structured-output call as
everything else — no second model round-trip. `recognizeSystemPrompt` and `describeSystemPrompt`
each gain one paragraph, right after the `estimated_profile` instructions: for a composite/merged
item (the same test already used to decide it stays one item), also emit a rough ingredients list
with English names and estimated weights; leave it empty for an already-atomic item (one apple).
`ingredients` is a required array property in `recognizeJSONSchema` (empty array, not a nullable
object — OpenAI's strict mode requires every property listed in `required`, and an array's own
emptiness already signals "no breakdown", the same way `estimated_profile: null` signals "no
estimate"). `toIngredientEstimates` trims each entry but deliberately keeps one whose
`canonical_name_en` comes back blank rather than dropping it (an early version dropped it — found
in code review to silently shrink the breakdown and let the all-or-nothing gate below pass over a
dish it never actually covered). A blank name simply cannot be searched, so it is left in the
persisted list for `resolveIngredientReference` to naturally fail to resolve, which is what
correctly keeps `HasIngredientReference` false.

### Persistence

`FoodItem` gains `IngredientsJSON` (a JSON blob of `IngredientReferenceEntry{Name,
CanonicalNameEN, WeightGrams, Resolved, FdcID, OffCode}`) — a blob, not a child table, following
`FoodMeal.ClarifyLog`'s existing precedent for "a small point-in-time list that's only ever read
back whole." `SetIngredients` treats an empty list as "clear the field" rather than storing `"[]"`,
so an atomic item and a pre-change row are indistinguishable by design — both just have no
breakdown. Persisted once at row-creation time (`newUnresolvedItem`, alongside
`SetEstimatedProfile`), then persisted again with each ingredient's resolution outcome once
`resolveIngredientReference` runs.

`FoodItem` also gains `HasIngredientReference` plus eight `IngredientReference*Per100g` fields —
the actual shadow profile, parallel to `Estimated*Per100g` but from a different source. All new
columns are `NOT NULL DEFAULT 0`/`false`, the same convention every prior nutrient column in this
domain uses (see `SaturatedFatGrams`'s own comment on why — SQLite's `ALTER TABLE ADD COLUMN`
rejects `NOT NULL` with no default against an already-populated table).

**All-or-nothing, deliberately.** `HasIngredientReference` is set only when *every* ingredient in
the breakdown resolved. A partial sum's per-100g figure would silently omit whatever fraction of
the dish its unresolved ingredients represent, while still presenting as "per 100g of the whole
dish" — directly comparable-looking to `Estimated*Per100g` while actually measuring something
narrower. Each ingredient's own resolution outcome (`Resolved`, `FdcID`) is still persisted
regardless, so a partial breakdown is fully visible for debugging; it just never becomes the
comparison figure.

### Resolution: deterministic, no model call — and what that actually requires

Per the owner's own instruction, ingredient resolution does not call `vision.Select` (an LLM
call) — unlike the whole-item candidate flow, which does exactly that, for a documented reason:
`usda.go`'s own `DefaultCandidates` comment records that plain search rank is unreliable even for
common foods ("chicken breast" ranked 12th, "white rice" ranked 17th" in a real dataset), which is
exactly why that flow trusts a Select call over rank. Going deterministic here without inheriting
that same failure mode needed a real substitute, not just "take the top search result":

- **`ingredientCandidateMatches`** requires every (lightly stemmed) word of the ingredient's own
  name to appear among the candidate description's own words, *and* the candidate's leading word —
  FDC descriptions always state the base food name before their comma-separated qualifiers — to
  equal the ingredient's own leading word. This rejects a same-topic-but-wrong-food candidate
  ("Turkey breast, chicken-fried" for the query "chicken breast") as well as one sharing only an
  unrelated word, while still accepting "Chicken, broiler or fryers, breast, ..." for "chicken
  breast". `stemmedWords` is a deliberately light plural stemmer (strip a trailing `-s`/`-es`), not
  a real one — enough to stop "tomato" failing against "Tomatoes, red, ripe, raw" over a bare
  English plural, nothing more.
- **`bestIngredientMatch`** walks the search shortlist in rank order and returns the first
  candidate that clears `ingredientCandidateMatches` — never the literal top rank unchecked.
- **A real search-recall bug, found empirically while writing this, not by inspection**: FTS5
  (`usda/query.go`'s `sanitizeFTSQuery`) does exact-token OR matching with no stemming at the index
  level either. A bare singular query like `"tomato"` returns **zero rows** against SR Legacy's
  actual `"Tomatoes, red, ripe, raw, ..."` description — not a ranking problem, a recall problem,
  and it would have silently zeroed out resolution for a wide swath of ordinary produce
  ingredients regardless of how good `ingredientCandidateMatches` was. `ingredientSearchTerm`
  fixes this the same way `QueryFor` already adds preparation/state as extra OR'd hint terms: it
  appends a naive plural of the ingredient's own last word (`naivePlural`) to the query, giving
  `Search`'s existing OR-join a second exact token to match — additive, never a filter, so it can
  only add recall.
- Not routed through `retrieveCandidates`/OFF: OFF's own `Search` needs a brand to be useful, and a
  rough ingredient like "cucumber" never has one.

**The trade-off is real and stated, not hidden.** This heuristic is not as good as an LLM Select
call — extending Select's item-indexed contract to also address individual ingredients was
considered and rejected for this round as a larger change than a shadow field's own scope
justifies (a compound item+ingredient addressing scheme inside a shared, tested LLM-facing
contract, for a feature whose own premise is "accumulate data now, decide what to improve once
there's enough of it"). Whenever nothing in an ingredient's shortlist clears the bar, it is left
unresolved rather than bound to a guess — the safe failure mode for a field whose entire purpose is
being a trustworthy comparison point.

### The per-100g figure

Each resolved ingredient's profile is scaled by `weight_grams/100` into plain nutrient grams and
summed across every ingredient (`ingredientTotals` — a distinct type from `NutrientProfile`
precisely so the running total, mid-sum, isn't held in fields literally named `...Per100g`). Once
every ingredient has resolved, the total is divided by `totalWeight/100` to land back in
per-100g-of-the-combined-ingredients terms — directly comparable to `Estimated*Per100g`, which is
also per-100g of the whole item.

### Not wired into anything user-visible

No display code, no `MacroSource` interaction, no Healthiness Label/advice/chat participation, no
data-table column allowlist entry. Purely two new persisted fields a future change can read.

### Comparing Luna vs. the ingredient-reference sum, later

Recorded here rather than rediscovered, matching `saturated-fat-signal.md`'s own precedent: once
enough `HasIngredientReference = true` rows accumulate, `Estimated*Per100g` vs.
`IngredientReference*Per100g` on the same row is a direct, apples-to-apples comparison — no
schema change needed to run it.

## Validation Commands

- `make lint`
- `make test`

### Task 1: Recognize/Describe return an optional ingredient breakdown

- [x] Add `Ingredients []IngredientEstimate` to `vision.Item`; extend `recognizeSystemPrompt` and
      `describeSystemPrompt` with the composite-item breakdown instructions
- [x] Add `ingredientSchema` and the required `ingredients` array property to
      `recognizeJSONSchema`; add `recognizeSchemaIngredient` and wire it through
      `recognizeSchemaItem`
- [x] `toIngredientEstimates` converts and trims, keeping (not dropping) an entry with a blank
      `canonical_name_en` so it correctly fails resolution downstream instead of vanishing
- [x] Cover schema-requiredness and parsing (present, empty, blank-name-kept) in
      `vision/openai_test.go`
- [x] Mark completed

### Task 2: Persist the breakdown and its resolution outcome

- [x] Add `IngredientsJSON`, `IngredientReferenceEntry`, `SetIngredients`/`Ingredients`,
      `HasIngredientReference` + the 8 `IngredientReference*Per100g` fields, and
      `SetIngredientReferenceProfile` to `models_food.go`
- [x] Call `SetIngredients` from `newUnresolvedItem`, at the same point `SetEstimatedProfile` is
      called
- [x] Cover `SetIngredients`/`Ingredients` round-trip (including the empty-clears and
      corrupt-JSON cases) and `SetIngredientReferenceProfile` in `models_food_test.go`
- [x] Mark completed

### Task 3: Resolve ingredients against USDA, deterministically

- [x] `resolveIngredientReference`: search each ingredient via `ingredientSearchTerm`, accept only
      a candidate clearing `ingredientCandidateMatches`, sum weighted profiles, and set
      `HasIngredientReference` only when every ingredient resolved
- [x] `ingredientSearchTerm`/`naivePlural`: fix the FTS5 exact-token plural-recall gap found while
      testing this against a real built USDA index
- [x] Call `resolveIngredientReference` from `resolveItems`'s per-item loop, independent of the
      whole-item candidate/Select flow
- [x] Cover `ingredientCandidateMatches`, `naivePlural`, and `resolveIngredientReference` (full
      resolution, partial resolution, no-breakdown no-op, nil-index no-op) in
      `food_upload_internal_test.go`, the latter against a real `usda.NewBuilder`-built index
- [x] Mark completed

### Task 4: Record and validate the result

- [x] Update CONTEXT.md with a short glossary entry
- [x] Run every validation command against the final branch and deployed WIP stack
- [x] Run the Review Gate and resolve every valid finding
- [x] Confirm that no task box remains unticked
- [ ] Mark completed
