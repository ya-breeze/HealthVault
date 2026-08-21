# ADR-003: Nutrition Targets Computed From Goal Weight, Not Measured Weight

## Status
Proposed

## Context and Problem Statement

Food dashboard cards need a daily calorie/macro target to compare intake against and to generate recommendations like "increase protein by X g/day." The Mifflin-St Jeor BMR formula takes a weight input — should that be the user's latest measured `weight` record, or their `weight_goal` (ADR-002)?

## Decision Drivers

- A user's stated intent, when they set a goal weight, is to eat for the body they're working toward, not the body they currently have
- Using measured weight would make targets drift with the very metric the user is trying to change (e.g. losing weight would continuously lower the protein target, working against a cutting goal)
- Requires `weight_goal` to exist before targets can be computed — an explicit dependency between Phase 3 and Phase 2

## Considered Options

- **Latest measured `weight`** — always available (assuming any weight has ever been logged), and is the "obvious" input a reader might expect for a BMR formula. But targets would shift every time a new weight is logged, independent of any actual goal.
- **`weight_goal`** — stable target that only changes when the user deliberately revises their goal; matches the intent behind setting a goal weight in the first place.

## Decision Outcome

Chosen: **`weight_goal` is the weight input to the full Mifflin-St Jeor formula** (BMR × activity multiplier from the Phase 3 profile fields — age/birthdate, sex, activity level, all new static fields in `UserSettings`), producing target daily calories and protein/carb/fat grams.

### Consequences

- If no `weight_goal` has been set, nutrition targets cannot be computed — Phase 4's target-comparison and recommendation cards have no meaningful fallback and must handle "no goal set" as a distinct state (e.g. prompting the user to set one) rather than silently substituting measured weight.
- Phase 3 must ship after Phase 2 for this reason; the two are not independently orderable despite Phase 3's other profile fields having no such dependency.
