# ADR-003: Nutrition Targets Split Between Measured Weight (calories) and Goal Weight (protein)

## Status
Accepted

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

- This is a **hard** dependency on Phase 2, not a softer one: `GET /api/users/me/nutrition-target`
  requires a `weight_goal` record unconditionally (`missing_goal_weight`, one of its four 422
  precondition reasons) — there is no fallback to measured weight for protein when no goal is set.
  A user with no goal weight gets no target at all, not a partial one.
- The whole Nutrition Target is all-or-nothing: there is no partial response (e.g. calories without
  protein). Carb/fat targets are computed from whatever calorie budget remains once protein is
  fixed, so a response missing protein would have nothing left to compute carbs/fat from either —
  Phase 4's cards therefore render the full four-value target or none of it, never a subset.
- The three profile fields and the activity-tier scheme this ADR deferred are now settled (see
  [ADR-006](ADR-006-steps-inferred-activity-level.md) for the activity-level mechanism): 5 tiers
  (Sedentary/Lightly active/Moderately active/Very active/Extra active, multipliers
  1.2/1.375/1.55/1.725/1.9), a protein rate of 1.6 g/kg applied to `weight_goal`, and a carb/fat
  split of the remaining calories 50/50 by kcal with a 0.8 g/kg fat floor (also basis
  `weight_goal`). `birthdate`, `sex`, and an optional `activity_override` are the three new
  `UserSettings` profile fields.
