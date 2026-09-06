# ADR-004: Food Healthiness Label Computed by Heuristic, Not LLM

## Status
Accepted

> **Update (`docs/specs/healthvault-nutrition-card-middle-row-th.md`, 2026-09-03):** the heuristic
> half of this decision has shipped — the nutrition card's middle row now renders a deterministic
> Healthiness Label (Good / Fair / Needs attention) computed by `frontend/lib/healthiness.ts` over
> a pooled 7-day window of the daily-totals endpoint's macro/sugar/sodium sums
> (`backend/pkg/database/food_daily_totals.go`), with no LLM call in the path. The thresholds this
> ADR deliberately left open (Consequences, below) now live as exported constants in
> `healthiness.ts`, each with its own rationale documented alongside it. What this ADR also
> decides — the LLM's role *downstream* of the label (cached daily recommendation generation, an
> on-demand refresh, and the chat affordance) — is still unbuilt; `summaryTodayResponse.Recommendation`
> stays `null` and reserved. Flipped to `Accepted` because the decision the ADR records — heuristic,
> not LLM, for the label itself — has now shipped; the still-unbuilt LLM-downstream half doesn't
> block that, since ADR status tracks whether the *decision* was acted on, not whether every
> consequence of it has been built.
>
> **Update (`docs/specs/nutrition-card-today-and-on-track.md`, 2026-08-31):** the *packaging* of
> this label has changed, though nothing about how it is computed has. Phase 4 originally placed it
> on its own dashboard card ("Card B"), beside a separate Card A for today's intake versus target.
> With the Logging Gap card already shipped, that would have put three nutrition cards on one
> dashboard; ya-breeze asked for one card instead. Today's-intake and the logging-gap line have now
> shipped as the first and third rows of the existing card (registry id `logging_gap`, retitled
> "Nutrition"), and the Healthiness Label plus its advice lines are its planned middle row rather
> than a card of their own.

## Context and Problem Statement

Food logging already calls an LLM for photo recognition (`ClarifyRound`/`Log` on `FoodMeal`), so an LLM-judged "how healthy has your recent eating been" score is a natural-seeming extension of the same pattern. Should the dashboard's healthiness label be computed the same way, by an LLM call, or deterministically?

## Decision Drivers

- The label is read on every dashboard load; an LLM call in that path adds latency and cost to a screen that's viewed far more often than any single meal is logged
- All the inputs it needs (macros, sugar, sodium) are already numeric fields on `FoodMeal`, populated regardless of whether a meal was logged by photo, manual entry, or barcode
- A deterministic label is reproducible — the same 7 days of logged food always yields the same label, which an LLM judgment would not guarantee
- The LLM is still used, just for a different, better-suited part of the feature: turning the label into personalized recommendation text, and answering follow-up questions in the chat affordance

## Considered Options

- **LLM-judged score per recent period** — most nuanced, but adds a paid, latency-bearing call to routine dashboard loads, and produces a score that could vary between two structurally identical weeks of eating.
- **Nutri-Score (Open Food Facts)** — only covers barcode-sourced items; the majority of logged meals are photo or manual entries with no Nutri-Score to read.
- **Deterministic heuristic over already-logged macros** — a rolling-7-day comparison of macro-calorie share against normal ranges, plus sugar/sodium thresholds, using fields already on `FoodMeal`. No LLM call, works for every entry source, same input always yields the same label.

## Decision Outcome

Chosen: **a deterministic heuristic computes the 3-level label** (Good / Fair / Needs attention, rolling 7-day window). The **LLM is used downstream of the label**, not to produce it: once-daily cached generation of the 1-2 recommendation lines shown under the label, a user-triggered "get advice" refresh, and a chat affordance for follow-up questions — all of which can reference the label and the Phase-3 nutrition targets (ADR-003) as context without needing to recompute them.

### Consequences

- The heuristic's exact thresholds (macro-share ranges, sugar/sodium cutoffs) are not fixed by this decision — they're deferred to Phase 4's own `opsx:propose`.
- If the heuristic and an LLM-generated recommendation ever disagree in tone (e.g. label says "Fair" but the LLM text reads alarmed), that's a prompt-design problem to solve in Phase 4, not a reason to move the label itself to the LLM.
