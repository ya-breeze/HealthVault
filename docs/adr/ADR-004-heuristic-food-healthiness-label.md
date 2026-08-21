# ADR-004: Food Healthiness Label Computed by Heuristic, Not LLM

## Status
Proposed

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
