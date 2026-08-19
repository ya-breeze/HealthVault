<!-- GENERATED FILE — DO NOT EDIT.
     Regenerate with: make projected-specs
     See openspec/specs.projected.README.md for details. -->

# chart-zoom-aggregation Specification

## Purpose
TBD - created by archiving change dashboard-instrument-panel. Update Purpose after archive.
## Requirements
### Requirement: Zoom level control
Every data page SHALL provide a Day / Week / Month / Year control that determines both the queried time range and the chart's aggregation. Week SHALL be the default zoom level on page load, matching the time range the page used before this change.

Zoom levels map to backend query granularity as follows — note "Week" and "Month" share the same `day` bucket and differ only in time range, so the backend never needs a distinct week-sized bucket:
| Zoom  | Time range   | `?bucket=` param |
|-------|--------------|-------------------|
| Day   | 1 day        | *(omitted — raw records)* |
| Week  | 7 days       | `day`             |
| Month | ~30 days     | `day`             |
| Year  | ~12 months   | `month`           |

#### Scenario: Default zoom on page load
- **WHEN** a user opens a data page without a prior zoom selection
- **THEN** the system SHALL select Week and request the corresponding 7-day range

#### Scenario: Changing zoom level re-queries data
- **WHEN** a user selects a different zoom level
- **THEN** the system SHALL request data for that level's time range and re-render the chart using that level's aggregation

### Requirement: Cumulative type aggregation
For cumulative types (`steps`, `distance`, `active_calories`, `total_calories`, `hydration`, `exercise`, `nutrition`, and `sleep` bucketed by night), the chart SHALL render as follows:
- **Day**: raw records plotted as a line.
- **Week or Month**: one bar per day (per night for `sleep`), showing the sum of that day's values.
- **Year**: one bar per month, showing the sum of that month's values.

`nutrition` has seven value columns (`calories`, `protein_grams`, `carbs_grams`, `fat_grams`, `sugar_grams`, `sodium_grams`, `dietary_fiber_grams`) rather than one. Its chart SHALL apply this same per-bucket sum independently to each column, selectable by the user (default: `calories`), rather than summing all seven into one series.

#### Scenario: Day view shows a raw line for a cumulative type
- **WHEN** a user views `steps` at Day zoom
- **THEN** the chart SHALL plot the raw records for that day as a connected line

#### Scenario: Week view shows daily bars for a cumulative type
- **WHEN** a user views `steps` at Week zoom
- **THEN** the chart SHALL render one bar per day, each bar's height equal to that day's summed step count

#### Scenario: Year view shows monthly bars for a cumulative type
- **WHEN** a user views `steps` at Year zoom
- **THEN** the chart SHALL render one bar per month, each bar's height equal to that month's summed step count, and SHALL NOT plot raw daily records

### Requirement: Point-in-time type aggregation
For point-in-time types (`heart_rate`, `heart_rate_variability`, `weight`, `height`, `blood_pressure`, `blood_glucose`, `oxygen_saturation`, `body_temperature`, `skin_temperature`, `respiratory_rate`, `resting_heart_rate`, `vo2_max`, `body_fat`, `lean_body_mass`, `bone_mass`, `speed`, `basal_metabolic_rate`), the chart SHALL render as follows:
- **Day**: raw records plotted as a line.
- **Week, Month, or Year**: a line of per-bucket averages (per-day for Week/Month, per-month for Year), with a shaded band spanning each bucket's minimum to maximum value.

#### Scenario: Week view shows an averaged line with a band
- **WHEN** a user views `heart_rate` at Week zoom
- **THEN** the chart SHALL plot one averaged point per day connected as a line, with a shaded region behind it spanning each day's min–max range

#### Scenario: Year view does not plot raw records
- **WHEN** a user views `heart_rate` at Year zoom
- **THEN** the chart SHALL plot one averaged point per month with a min–max band, and SHALL NOT request or render the underlying raw records for the year

### Requirement: Blood pressure dual-series aggregation
`blood_pressure` SHALL follow the point-in-time aggregation rule independently for its `systolic` and `diastolic` values, rendering both as separate lines (with independent min–max bands at Week/Month/Year) on the same chart.

#### Scenario: Both series visible at every zoom level
- **WHEN** a user views `blood_pressure` at any zoom level
- **THEN** the chart SHALL display both a systolic and a diastolic line, distinguishable from each other

### Requirement: Chart summary stats follow the active zoom
The stats row beneath the chart (average, maximum, and total or average as applicable to the type's family) SHALL be computed from the same bucketed data driving the chart, not from a separate raw fetch.

#### Scenario: Stats match the displayed range
- **WHEN** a user changes zoom level
- **THEN** the stats row SHALL update to reflect the newly selected range and aggregation

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
at Week and Year zoom SHALL be widened (to at least 14 trailing days for Week, and at least 2
trailing years for Year) beyond that zoom's normal range, so the trend has stabilized before the
first displayed bucket; only the trend values within the zoom's own visible range SHALL be
plotted. Widened lookback buckets that fall outside the visible range SHALL NOT otherwise be
rendered or included in the chart's stats. This requirement applies to `weight` only, and does not
apply at Day zoom.

#### Scenario: Trend line rendered at Week zoom
- **WHEN** a user views `weight` at Week zoom
- **THEN** the chart SHALL render a trend line alongside the existing averaged line and min-max
  band, computed from at least 14 days of bucketed data even though only 7 days are displayed

#### Scenario: Trend line rendered at Month zoom
- **WHEN** a user views `weight` at Month zoom
- **THEN** the chart SHALL render a trend line computed from that zoom's own 30-day bucketed data
  range, with no additional widening (30 daily buckets already exceed the ~14-16 period minimum)

#### Scenario: Trend line rendered at Year zoom
- **WHEN** a user views `weight` at Year zoom
- **THEN** the underlying bucketed data SHALL be widened to at least 2 trailing years (rather than
  the zoom's normal ~1 year), since ~12-13 monthly buckets alone fall short of the ~14-16 periods
  the EMA needs to converge, and the chart SHALL render a trend line computed from that widened
  series but only plotted across the zoom's own ~12-13 visible monthly buckets

#### Scenario: No trend line at Day zoom
- **WHEN** a user views `weight` at Day zoom
- **THEN** the chart SHALL NOT render a trend line

#### Scenario: No trend line for other point-in-time metrics
- **WHEN** a user views a point-in-time metric other than `weight` (e.g. `heart_rate`) at any zoom
  level
- **THEN** the chart SHALL NOT render a trend line

### Requirement: Chart display precision

Numeric values rendered by a chart (Y-axis tick labels, tooltip values, and line/band series including the weight trend line) SHALL be rounded to that metric's established display precision before being shown to the user. Rounding SHALL apply only to the rendered/displayed value; the underlying aggregated or computed value (including averages, min/max band bounds, and the EMA-smoothed trend series) SHALL remain full precision for computation.

Display precision per metric SHALL match the precision already used for that metric elsewhere in the app (the vitals-grid card and the stats row below the chart), so a metric never shows two different numbers of decimal digits in different parts of the UI.

#### Scenario: Weight trend line shows one decimal

- **WHEN** a user views the `weight` chart at Week, Month, or Year zoom
- **THEN** the trend line's tooltip value and any Y-axis tick derived from it SHALL display with exactly 1 decimal digit, matching the precision already used for `weight` in the stats row

#### Scenario: Whole-number metrics show no decimals

- **WHEN** a user views a chart for a metric whose display precision is 0 decimals (e.g. `heart_rate`, `steps`, `oxygen_saturation`)
- **THEN** the Y-axis ticks and tooltip values SHALL display as whole numbers with no decimal point

#### Scenario: Underlying computation stays full precision

- **WHEN** the chart computes an averaged point, a min/max band, or the EMA trend series for display
- **THEN** the computation SHALL use full floating-point precision, and only the final rendered text SHALL be rounded

