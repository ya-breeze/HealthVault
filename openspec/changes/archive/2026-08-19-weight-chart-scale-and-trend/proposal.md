## Why

The point-in-time metric chart (weight, heart rate, blood pressure, etc. — the shared
`ComposedChart`/`LineChart` rendering in `DataTypeClient.tsx`) lets its Y-axis default toward
zero. For metrics whose meaningful range sits far from zero — most visibly `weight`, where a
person's readings might span 90-94 kg — this flattens real week-to-week changes into a barely
visible wiggle near the top of the chart. A user comparing this to other weight-tracking apps
(e.g. Libra, whose CSV exports HealthVault already imports via `libra-import`) noted those apps
scale the axis to the data itself, and separately show a smoothed trend line to see through
day-to-day noise (water weight, meal timing) that raw daily readings carry.

## What Changes

- Y-axis domain for all point-in-time metric types (`weight`, `height`, `heart_rate`,
  `heart_rate_variability`, `blood_pressure`, `blood_glucose`, `oxygen_saturation`,
  `body_temperature`, `skin_temperature`, `respiratory_rate`, `resting_heart_rate`,
  `basal_metabolic_rate`, `body_fat`, `lean_body_mass`, `vo2_max`, `bone_mass`, `speed`), at every
  zoom level, now fits the visible data's own min/max instead of defaulting toward zero. A small
  padding keeps the line off the chart edges, and a minimum-span floor keeps a near-constant
  reading period from zooming in absurdly tight. Cumulative-family metrics (`steps`, `distance`,
  `active_calories`, `total_calories`, `hydration`, `exercise`, `sleep`, `nutrition`), which
  render as bars, are unchanged — zero is a meaningful baseline for a bar chart.
- A smoothed trend line is added for `weight` only, at Week/Month/Year zoom (not Day — Day's
  x-axis is hourly, and a multi-day trend value has no natural place on it). It's computed
  client-side as an exponential moving average (EMA) over the same per-day/per-month bucketed
  averages already fetched for those zooms, using at least 14 trailing days of bucketed data so
  the EMA has stabilized before the visible window starts (Week zoom's own fetch widens from 7 to
  14 days for this reason; Month and Year already exceed 14 days).
- No backend changes: both pieces are computed in the frontend from data already returned by the
  existing `?bucket=` API.

Goal weight, BMI category reference bands, a projected/dashed trend-to-goal line, and trend lines
for metrics other than weight were considered (they're what the reference screenshot's app also
shows) and deliberately deferred — see `todo.md` — since goal weight has no persistence layer in
HealthVault today and BMI needs a height-derived calculation not otherwise needed here.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `chart-zoom-aggregation`: adds a Y-axis domain rule for point-in-time types, and a weight-only
  trend-line rendering rule for Week/Month/Year zoom.

## Impact

- `frontend/app/data/[type]/DataTypeClient.tsx`: Y-axis `domain` prop on all point-in-time
  `<YAxis>` instances; trend-line computation and an added `<Line>` series for `weight` at
  Week/Month/Year; widened bucketed-data fetch range for `weight` at Week zoom.
- No backend, database, or API changes.
- No new external dependencies.
