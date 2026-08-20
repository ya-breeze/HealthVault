## Why

Chart Y-axis labels, tooltips, and the weight trend line render raw floating-point values with no formatting, so users see numbers like `72.384615384615` instead of `72.4`. Every other numeric display in the app (the stats row under each chart, `vitals.ts`-driven displays) already rounds to a sensible per-metric precision — the chart itself is the one place this was missed, most visibly since the recent EMA trend line landed on the weight chart.

## What Changes

- Add display-only rounding to chart Y-axis ticks, tooltip values, and line/band labels in `DataTypeClient.tsx`, for every charted metric (not just weight).
- Reuse the existing per-metric precision convention from `frontend/lib/vitals.ts` (1 decimal for weight/distance/sleep-hours; whole numbers for heart rate, HRV, oxygen saturation, steps, etc.) rather than introducing a new precision rule.
- No change to underlying computation: the EMA trend series (`emaSeries()` in `dataTypeMeta.ts`) and backend `AVG`/`MIN`/`MAX` aggregation queries keep full floating-point precision — only what is rendered is rounded.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `chart-zoom-aggregation`: adds a display-precision requirement — chart-rendered values (axis ticks, tooltips, line/band labels) SHALL be rounded to each metric's established display precision before rendering, without altering the underlying aggregated/computed values.

## Impact

- `frontend/app/data/[type]/DataTypeClient.tsx` — add `tickFormatter`/tooltip `formatter` props to `YAxis`/`Tooltip`/`Line` elements, sourcing precision from `vitals.ts`.
- `frontend/lib/vitals.ts` — likely exposes (or gains) a shared formatting helper so the chart and the existing stats row (`.toFixed(1)`) use one source of truth instead of duplicating precision rules.
- No backend changes, no API changes, no migration.
