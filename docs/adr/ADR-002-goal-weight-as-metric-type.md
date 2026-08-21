# ADR-002: Goal Weight Modeled as a Metric Type, Not a User Setting

## Status
Proposed

## Context and Problem Statement

The weight-chart backlog (see `todo.md`, "Weight chart: goal weight, BMI bands, and trend projection") needs somewhere to store the user's target body weight. `UserSettings` (a per-user JSON-blob settings table) now exists, which is the obvious place a reader would expect a single scalar "goal" value to live. Where should Goal Weight actually be stored?

## Decision Drivers

- HealthVault already has a metric-type pattern (`weight`, `height`, ...) for point-in-time values with full history and a "latest wins" read
- A goal is not static forever — a user revises it, and losing the history of past goals (e.g. for the trend-projection chart, or just "what was I aiming for last month") would be a regression the JSON-blob pattern doesn't give for free
- `UserSettings` is intended for genuinely static profile attributes (see ADR-003), not a second place time-series-like values live

## Considered Options

- **A field in `UserSettings`** — reads as the obvious choice given the table's existence, but stores only the current value; changing your goal overwrites history with no record of the previous one, and it's a second, differently-shaped mechanism for what is otherwise a metric.
- **A new metric type, `weight_goal`** — reuses the existing per-type timestamped-record pattern (`TYPE_META`, the data API's `?bucket=`) and its "latest record wins" read semantics exactly match what a goal needs.

## Decision Outcome

Chosen: **`weight_goal` as a new metric type**, alongside `weight` and `height`, not a `UserSettings` field. It renders as a horizontal reference line on the weight chart (current value) and a dashed projection is extrapolated from the existing weight trend line to where it crosses the goal.

### Consequences

- BMI category bands (WHO thresholds) are a separate, goal-independent feature — they only need the latest `height` record and render regardless of whether a goal is set; they're hidden entirely if no `height` is on file.
- Nutrition targets in Phase 3 (see ADR-003) read Goal Weight, not the latest measured `weight`, as their weight input — a deliberate choice made explicit there.
