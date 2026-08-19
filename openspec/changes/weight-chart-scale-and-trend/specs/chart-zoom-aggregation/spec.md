## ADDED Requirements

### Requirement: Point-in-time Y-axis domain
For point-in-time types (the same set covered by "Point-in-time type aggregation"), the chart's
Y-axis domain SHALL be computed from the values driving the chart at the active zoom level (raw
values at Day zoom; the `min`-to-`max` band range at Week/Month/Year zoom) rather than defaulting
toward zero. The domain SHALL be padded by 10% of the data range on each side, or — when the data
range is zero (a single value, or several identical values) — by 2% of the data's own magnitude,
so a flat series still renders a sensible band instead of a hairline-thin one.

#### Scenario: Narrow-range metric does not default to zero
- **WHEN** a user views `weight` at any zoom level, with readings clustered between 90 kg and 94
  kg
- **THEN** the Y-axis domain SHALL be computed from that 90-94 kg range (plus padding), and SHALL
  NOT extend down to 0

#### Scenario: Flat data does not collapse the axis
- **WHEN** a user views a point-in-time metric whose visible values are all identical (or there is
  only one data point)
- **THEN** the Y-axis domain SHALL still span a visible range around that value, padded by 2% of
  its magnitude, rather than rendering a zero-height domain

#### Scenario: Cumulative types are unaffected
- **WHEN** a user views a cumulative type (e.g. `steps`) at any zoom level
- **THEN** the Y-axis domain SHALL be unchanged by this requirement, retaining its existing
  zero-anchored behavior

### Requirement: Weight trend line
At Week, Month, or Year zoom, the `weight` chart SHALL additionally render a smoothed trend line,
computed as an exponential moving average (alpha = 0.25) over the same per-bucket average values
(`avg`) already driving that zoom level's chart. The underlying bucketed data fetched for `weight`
at Week zoom SHALL cover at least 14 trailing days (rather than the 7-day Week zoom range) so the
trend has stabilized before the first displayed day; only the trend values within the zoom's own
visible range SHALL be plotted. This requirement applies to `weight` only, and does not apply at
Day zoom.

#### Scenario: Trend line rendered at Week zoom
- **WHEN** a user views `weight` at Week zoom
- **THEN** the chart SHALL render a trend line alongside the existing averaged line and min-max
  band, computed from at least 14 days of bucketed data even though only 7 days are displayed

#### Scenario: Trend line rendered at Month and Year zoom
- **WHEN** a user views `weight` at Month or Year zoom
- **THEN** the chart SHALL render a trend line computed from that zoom's own bucketed data range,
  with no additional widening (the existing 30-day/~12-month ranges already exceed the 14-period
  minimum)

#### Scenario: No trend line at Day zoom
- **WHEN** a user views `weight` at Day zoom
- **THEN** the chart SHALL NOT render a trend line

#### Scenario: No trend line for other point-in-time metrics
- **WHEN** a user views a point-in-time metric other than `weight` (e.g. `heart_rate`) at any zoom
  level
- **THEN** the chart SHALL NOT render a trend line
