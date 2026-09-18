# ADR-014: A deterministic, per-ingredient USDA reference sum as a background comparison field

## Status
Proposed

## Context and Problem Statement

Every previously-logged FoodItem either resolves to a single USDA/OFF/CustomFood reference (macro
source `reference`) or falls back to Luna's own per-photo guess (macro source `estimated`). A
composite/merged dish — a salad, a curry, a stew, anything the model keeps as one served item per
`split-distinct-plated-foods` — essentially never has a single USDA/OFF row that matches its whole
name, so it is permanently `estimated`, with nothing to check the model's own guess against. A
same-day audit of `hcw-prod` found 90% of active items in exactly this state.

## Decision Drivers

- The owner wants to accumulate a trustworthy, USDA/OFF-grounded comparison point for composite
  dishes too, not only the minority of items that happen to bind to a single reference food.
- The comparison field must never change what the user sees today, nor which values drive the
  Healthiness Label, advice, or chat — this is data collection for a future decision, not a
  feature.
- No second model round-trip: whatever breakdown Recognize/Describe provide must come back in the
  same structured-output call as everything else.
- Resolution against USDA must be deterministic — no additional LLM call — per explicit direction.

## Considered Options

- **Do nothing; wait for Idea #640's canonical-name fix alone.** That fix (a separate, narrower
  change handled outside this repo change) only helps items whose *whole* name already matches a
  single reference food. It does nothing for genuinely composite dishes, which are the majority of
  what gets logged.
- **Extend `vision.Select`'s item-indexed contract to also address individual ingredients**,
  reusing the same LLM disambiguation call the whole-item flow already makes (batched, so still one
  call per meal). Higher precision, since it uses the exact mechanism this codebase already trusts
  for "which of several similar candidates is actually right" — the documented reason plain search
  rank is not good enough on its own (`usda.go`'s `DefaultCandidates` comment: the correct food
  ranked 12th for "chicken breast" in a real measurement). Rejected for this round: it means
  compound-addressing a shared, tested LLM contract for a feature whose own premise is "collect
  data now, decide what to improve once there is enough of it" — a bigger change than a shadow
  field justifies before anyone has looked at a single real comparison.
- **Deterministic search with a real precision guard, not blind top-rank** (chosen). A stricter
  acceptance rule than plain FTS rank — matching food name and leading term, with light plural
  stemming — walked down the shortlist until something clears it, or the ingredient is left
  unresolved. Lower recall than an LLM call would give, honestly documented as such, but zero
  additional model cost and no changes to a shared, tested contract.

## Decision Outcome

Chosen: deterministic search with the stricter acceptance rule (`ingredientCandidateMatches`), all
new storage kept as an inert shadow (`HasIngredientReference` + `IngredientReference*Per100g`),
and an all-or-nothing gate — the shadow profile is only ever stored when every ingredient in the
breakdown resolved, never a partial sum, so it is always directly comparable to
`Estimated*Per100g` without a caveat threaded through every future comparison query.

A real, previously-unknown limitation of this codebase's own search surfaced while implementing
this: FTS5 (`usda/query.go`'s `sanitizeFTSQuery`) does exact-token matching with no stemming, so a
bare singular ingredient name returns zero rows against SR Legacy's actual plural descriptions
("tomato" against "Tomatoes, red, ripe, raw, ..."). This is not new to this feature — it is a
property of the search index the existing whole-item flow also depends on — but this feature is
the first thing in the codebase to search on bare, undecorated single-word food names rather than
names already shaped by a model that tends to echo FDC's own vocabulary. Fixed with an additive
naive-plural query hint (`ingredientSearchTerm`); recorded here because it is exactly the kind of
gap a future extension of this same search surface should know already exists.

## Consequences

- **Positive:** every composite item Recognize is willing to decompose now accumulates a real,
  independently-sourced comparison point, at zero additional LLM cost, with no change to anything
  the user currently sees.
- **Negative:** the deterministic matcher will resolve fewer ingredients than an LLM-assisted one
  would, and some composite items will simply never get a fully-resolved shadow profile at all
  (`HasIngredientReference` stays `false`). Accepted: a missing data point costs nothing here; a
  wrong one would actively undermine the comparison this field exists to enable.
- **Follow-up, not committed to:** if the eventual comparison shows the deterministic matcher's
  recall is too low to be useful, the natural next step is the rejected Select-based option above,
  now informed by real data on how often the simple approach actually resolves something.
