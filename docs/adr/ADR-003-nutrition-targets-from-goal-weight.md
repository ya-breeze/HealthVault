# ADR-003: Nutrition Targets Split Between Measured Weight (calories) and Goal Weight (protein)

## Status
Proposed

## Context and Problem Statement

Food dashboard cards need a daily calorie/macro target to compare intake against and to generate recommendations like "increase protein by X g/day." The Mifflin-St Jeor BMR formula takes a weight input — should every target use the user's latest measured `weight` record, their `weight_goal` (ADR-002), or a mix?

## Decision Drivers

- BMR is a function of actual current body mass. Computing it from `weight_goal` instead of measured weight understates real energy expenditure whenever the goal differs meaningfully from the current weight, producing a far more aggressive calorie deficit than intended (e.g. a 100 kg user with a 70 kg goal would be given a calorie target sized for someone who already weighs 70 kg, not a safe deficit from their actual 100 kg baseline)
- Protein targets are conventionally sized to the body being worked toward, not the current one — using `weight_goal` for the protein-per-kg calculation avoids over- or under-prescribing protein for the target physique, which is exactly the intent behind setting a goal weight in the first place
- Carb/fat targets fall out of whatever's left in the calorie budget once calories and protein are fixed, so they don't need an independent weight choice

## Considered Options

- **Everything from measured `weight`** — metabolically correct for calories, but the protein target then drifts with the very weight the user is trying to change, and never reflects their stated goal.
- **Everything from `weight_goal`** — metabolically wrong for calories/BMR (see driver above); risks recommending an unsafe deficit the further the goal is from the current weight.
- **Split: calories/BMR from measured `weight`, protein g/kg from `weight_goal`** — each target uses the weight that's actually correct for it.

## Decision Outcome

Chosen: **split**. Daily calories = Mifflin-St Jeor BMR (from the latest measured `weight`, height, age, sex) × activity multiplier (Phase 3 profile fields, all new static fields in `UserSettings`). Protein target = a g/kg rate applied to `weight_goal`. Carb/fat targets fill the remaining calorie budget by a standard macro-ratio split.

### Consequences

- This is a softer dependency on Phase 2 than an all-goal-weight design would have been: the calorie target only needs measured `weight` (already available from existing vitals), while only the protein target specifically needs `weight_goal`. Phase 3's own `opsx:propose` still needs to decide what happens to the protein target — and any recommendation text derived from it — when no `weight_goal` is set yet (e.g. fall back to measured weight for protein too, or mark protein as "not yet available" until a goal exists).
- Phase 4's target-comparison and recommendation cards must be able to show calories/carbs/fat even when protein specifically is unavailable, rather than treating the whole Nutrition Target as all-or-nothing.
