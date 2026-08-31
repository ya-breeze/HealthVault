# ADR-011: Manual Food Entry's Primary Path Is Model Recognition, Not Reference-Database Lookup

## Status
Proposed

## Context and Problem Statement

Idea #23 reported that manual food entry — the path a user takes when logging a meal without a
photo — is the weakest part of the app precisely where it is used most. `/food/manual/` led with
"Search food," a `GET /api/food/search` query against USDA's American-English generic-food index;
its only alternative was "Enter macros," where the user types all seven macro values by hand. For
a Russian user this meant: a search only worked if the translation step happened to land on the
right USDA vocabulary term, and even a successful match returned an English name ("Egg, quail,
whole, raw") with a portion weight the user had to supply in grams themselves. A composite dish
with no single reference-food row ("борщ со сметаной и два куска хлеба") had no path through
search at all.

Meanwhile the photo path already solved this exact problem: `vision.Client.Recognize` splits a
meal into items, names each one in the user's Display Language, and estimates its own per-100g
`estimated_profile` — used as the primary macro source, with reference-database candidates binding
only on an exact custom-food name match, and USDA/Open Food Facts skipped outright for a
non-English Display Language. All of that machinery already existed; the only thing manual entry
lacked was a text-only entry point into it. How should manual entry be changed to use it, and what
happens to the existing structured (search/macros) form?

## Decision Drivers

- The reference-database-first design cannot serve a non-English Display Language or a composite
  dish, and both are common cases, not edge cases — a Russian user describing a home-cooked meal is
  the profile this app's Display Language feature was built for in the first place.
- The photo path already has a working, shipped answer to this exact problem (model-driven
  recognition with an estimate-first macro fallback); manual entry needed a new entry point into
  it, not a new mechanism.
- The structured form is still the only way to enter exact macros copied off a package label, and
  it is a load-bearing seed path for the E2E suite (`food.spec.ts`, `completeness.spec.ts` both
  call `POST /api/food/meals/manual` directly) — removing it would regress both a real capability
  and existing test infrastructure.
- Reanalyze's Expert mode already established the precedent of forcing Display Language to `"en"`
  for user-typed, non-recognition text (`runExpertAnalysis`), because that gate exists to keep an
  English-vocabulary reference database from being skipped for a vision-recognized foreign-script
  name — a rationale that doesn't apply to text the user typed directly. The new describe path
  needed its own, opposite decision for the same gate.

## Considered Options

- **Keep search-first, improve translation instead.** Rejected: the underlying problem is that
  USDA/Open Food Facts are English-vocabulary reference indexes with no representation for most
  home-cooked, composite, or regional dishes — a better translation step still can't produce a row
  that doesn't exist, and still can't split a described dish into items or estimate portions from
  text.
- **Add free-text entry as a new resolution mode alongside Search/Manual, reusing only the search
  endpoint's matching.** Rejected: this still routes through the same English-vocabulary indexes
  and the same one-row-per-food model; it would not gain item-splitting, per-item weight estimation,
  or a language-aware macro fallback without duplicating what `Recognize`/`resolveItems` already do.
- **Add a text-only `Describe` call reusing the entire existing recognition pipeline
  (`RecognizeResult`, `processRecognition`, `resolveItems`, the clarify loop, the review screen),
  and demote the structured form to a secondary, collapsed option.** Chosen — described below.
- **Force Display Language to `"en"` on the describe path, mirroring `runExpertAnalysis`.**
  Rejected: `runExpertAnalysis`'s forcing exists because a vision-recognized item name is
  English-vocabulary-unmatchable text in a foreign script, so forcing `"en"` there restores USDA/OFF
  matching for names the user actually typed in English (ingredient lists). A Meal Description is
  the opposite case — the user's own free-text account, in whatever language they chose, is exactly
  the thing that should reach the model and come back in that same language. Forcing `"en"` here
  would silently reproduce the original bug this idea reports.

## Decision Outcome

Chosen: **manual entry's primary path is model recognition from a user-written Meal Description,
and the structured item-by-item form is kept, not removed, as a demoted secondary option.**

- `vision.Client` gains `Describe(ctx, description, displayLanguage) (*RecognizeResult, error)` —
  text-only, no image, sharing `Recognize`'s response shape and JSON schema outright so every
  downstream stage (`processRecognition`, `resolveItems`, `persistAnalysis`, the clarify loop, the
  review screen) works on its result with no modification. It gets its own system prompt,
  `describeSystemPrompt`, because the evidence is different: `Recognize`'s prompt repeatedly
  instructs the model to judge from visible photo evidence, which is meaningless for a text
  description; the new prompt keeps the shared item-splitting/merging rules and replaces the visual
  instructions with quantity-reading, portion-estimation, and "always produce `estimated_profile`"
  rules.
- `POST /api/food/meals/describe` follows the photo path's commit-first rule exactly: the
  `FoodMeal` row is created `processing`, with the description stored and no `PhotoPath`, before
  the model call runs — so a failed or timed-out call can never lose what the user typed.
  `runDescribeAnalysis`/`analyzeDescribedMeal` mirror `analyzeMeal`'s vision-timeout-plus-`failMeal`
  wrapper exactly.
- **The user's real Display Language is passed to `Describe`** — not forced to `"en"`. This is the
  substance of the fix: for a non-English Display Language, the Display Name comes back in that
  language, `retrieveCandidates` correctly skips USDA/Open Food Facts (the same gate the photo path
  already has), the fuzzy custom-food match still runs unaffected (it's language-agnostic), and
  macros come from the model's own `estimated_profile` — exactly the photo path's existing
  estimate-first behavior, now reachable without a photo.
- **The structured form is kept, not deleted**, demoted behind a collapsed disclosure on the same
  page. It remains the only way to enter exact macros copied off a package label, and
  `POST /api/food/meals/manual`'s contract is completely unchanged — including for the E2E suite's
  seeding helpers, which call it directly and never went through the UI rework.
- `FoodMeal.Description` is a new, deliberately unexposed column: left out of
  `storage_impl.go`'s `columnAllowlist` for `food_meals`, so the family-visible generic
  `GET /api/data/food_meal` path keeps returning exactly the columns it returns today.

### Consequences

- A described meal has no photo to fall back on if `Describe` fails — its only recoverable state
  is the stored description text, which is why persisting it before the model call, and replaying
  it on retry (`RetryMeal` now branches on `PhotoPath` vs. `Description`), is load-bearing rather
  than incidental.
- Reanalyze remains photo-only and 409s for a described meal, same as it already does for a
  structured-manual one — `ReviewClient` already hides the control when `photo_path` is empty, so
  nothing user-visible regresses. A "reanalyze from an edited description" follow-up is a coherent
  future idea, not part of this change.
- The manual-entry page now renders partly in the user's Display Language (the description path)
  and partly in English (the demoted structured form) — an intentional, documented split, not an
  oversight; see `CONTEXT.md`'s Meal Description entry and `frontend/lib/i18n/en.ts`'s header
  comment.
