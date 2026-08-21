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

### Requirement: BMI reference bands on the weight chart
When the user has at least one `height` record, the `weight` chart SHALL render four WHO BMI
category reference bands — Underweight `(-∞, 18.5)`, Normal `[18.5, 25)`, Overweight `[25, 30)`,
Obese `[30, ∞)` — converted from BMI to kilograms using the user's latest `height` record
(`kg = bmi * heightMeters^2`). Boundaries are lower-inclusive: a BMI of exactly 18.5, 25, or 30
belongs to the higher category, in both the rendered band edges and the BMI readout's category
lookup below, so the two can never disagree at a boundary value. The bands SHALL be clipped
to the chart's existing Y-domain and SHALL NOT themselves expand or otherwise influence that
domain, and SHALL render at every zoom level (Day, Week, Month, and Year) — unlike the dashed
trend projection line, which is restricted to Month/Year (see "Weight trend projection" below).
The chart SHALL also display a BMI readout (value to one decimal place, plus the category
name, via the same category lookup as the bands) computed from the latest raw `weight` record
and the latest `height` record — not from the EMA-smoothed trend value. Both the bands and the
readout SHALL be governed by the same "does a `height` record exist" condition, and SHALL both
be absent when it does not.

#### Scenario: Bands and readout render when height is on file
- **WHEN** a user with at least one `height` record views the `weight` chart
- **THEN** the chart SHALL render all 4 BMI bands clipped to the existing Y-domain, and SHALL show
  a BMI readout (1 decimal place + category name) computed from the latest raw weight and height

#### Scenario: Bands and readout absent when no height is on file
- **WHEN** a user with no `height` record views the `weight` chart
- **THEN** the chart SHALL NOT render BMI bands and SHALL NOT show a BMI readout

#### Scenario: Bands never expand the Y-domain
- **WHEN** the BMI bands' kg range (e.g. spanning roughly BMI 15-35) is wider than the chart's
  actual weight data range
- **THEN** the Y-domain SHALL remain governed by the existing weight-data domain computation, and
  the bands SHALL be visually clipped at the domain's edges rather than widening it

#### Scenario: Bands render at every zoom level
- **WHEN** a user with at least one `height` record switches the chart between Day, Week, Month,
  and Year zoom
- **THEN** the BMI bands SHALL remain visible at all four zoom levels

#### Scenario: Category boundary is lower-inclusive
- **WHEN** the latest raw `weight` and `height` compute to a BMI of exactly 18.5, exactly 25, or
  exactly 30
- **THEN** the BMI readout SHALL show the higher category (Normal at 18.5, Overweight at 25, Obese
  at 30), and the band `ReferenceArea` the point falls within SHALL agree with that category

### Requirement: Goal weight reference line
When the user has at least one `weight_goal` record, the `weight` chart SHALL render a reference
line at the latest `weight_goal` value, at every zoom level. Unlike BMI bands, this value SHALL be
included in the Y-domain computation, so the domain always expands to include the goal line even
if that flattens the visible range of the user's actual weight data.

#### Scenario: Goal line renders and expands the domain
- **WHEN** a user has a `weight_goal` record whose value lies outside their recent weight data's
  own range
- **THEN** the chart SHALL render a reference line at the goal value, and the Y-domain SHALL expand
  to include it

#### Scenario: No goal line when no goal is set
- **WHEN** a user has no `weight_goal` record
- **THEN** the chart SHALL NOT render a goal reference line, and the Y-domain SHALL be computed as
  it was before this requirement existed

### Requirement: Weight trend projection
When the user has at least one `weight_goal` record, the `weight` chart SHALL compute a trend
projection: an ordinary least-squares regression of `(day_offset, ema_value)` over the last 30
calendar days of the existing EMA series, independent of the chart's currently selected zoom
level. The projection SHALL determine the date the regression line crosses the latest
`weight_goal` value, if any, within a 12-month horizon.

Minimum data: if the user has fewer than 5 `weight` records total, or the span between their
earliest and latest `weight` record is under 14 days, the system SHALL display "Not enough data
to project yet" and SHALL NOT render a projection line. This lifetime-history check does not by
itself guarantee data inside the 30-day regression window — the system SHALL also display "Not
enough data to project yet" whenever the 30-day window contains fewer than 2 EMA points (e.g. a
user with old data but no recent activity), since both the regression and the direction
determination below require at least two points in that window.

Already at goal: direction is determined by comparing the 30-day regression window's own boundary
EMA values to the goal — the EMA value at the oldest day in the window versus the EMA value at the
newest — not by the user's entire lifetime history, so an old weight from years before the goal
was set can't flip the determination. If the latest EMA value has already reached or passed the
goal in that direction, the system SHALL display "You've reached your goal weight" and SHALL NOT
render a projection line. This check takes precedence over the flat/diverging check below, since a
flat trend at the goal is success, not failure.

Given sufficient data, and the goal not already reached: if the regression slope does not move
toward the goal (flat or diverging), or the computed crossing date falls beyond the 12-month
horizon, the system SHALL display "Not on track at your current trend" and SHALL NOT render a
projection line. The 12-month horizon SHALL be
a fixed 365-day count from the most recent day in the 30-day regression window (not calendar-month
arithmetic): a crossing at or before day 365 from that point is within the horizon, and a crossing
after day 365 is beyond it. Otherwise the system SHALL render a dashed projection line from the
current EMA value to the computed crossing point, and SHALL display the crossing date as ETA text.

The dashed projection line itself SHALL render only at Month and Year zoom (not Day or Week — a
7-day window cannot show a months-away crossing point). The ETA text (or the "not enough data" /
"not on track" message, whichever applies) SHALL render at every zoom level, so switching zoom
never hides the answer.

#### Scenario: Not enough data
- **WHEN** a user has fewer than 5 `weight` records, or their records span fewer than 14 days
- **THEN** the chart SHALL display "Not enough data to project yet" and SHALL NOT render a
  projection line, at any zoom level

#### Scenario: Not enough data inside the regression window
- **WHEN** a user has at least 5 `weight` records spanning at least 14 days, but fewer than 2 of
  their EMA points fall within the last 30 calendar days (e.g. they stopped logging weight months
  ago)
- **THEN** the chart SHALL display "Not enough data to project yet" and SHALL NOT render a
  projection line, at any zoom level

#### Scenario: On-track projection at Month/Year zoom
- **WHEN** a user has sufficient weight history, a `weight_goal` set, and a 30-day EMA trend moving
  toward the goal with a crossing date within 12 months, viewed at Month or Year zoom
- **THEN** the chart SHALL render a dashed projection line to the computed crossing point and
  SHALL display the crossing date as ETA text

#### Scenario: On-track projection at Day/Week zoom shows text only
- **WHEN** the same on-track projection as above is viewed at Day or Week zoom
- **THEN** the chart SHALL NOT render the dashed projection line, but SHALL still display the ETA
  text

#### Scenario: Wrong-direction or flat trend
- **WHEN** a user's 30-day EMA trend is flat or moving away from their `weight_goal`, and they have
  not already reached it
- **THEN** the chart SHALL display "Not on track at your current trend" and SHALL NOT render a
  projection line

#### Scenario: Already at goal weight
- **WHEN** a user's latest EMA value has already reached or passed their `weight_goal` value (per
  the direction determined from the 30-day regression window's own oldest and newest EMA values,
  not their entire lifetime history), regardless of whether their 30-day trend is currently flat,
  diverging, or still moving toward the goal
- **THEN** the chart SHALL display "You've reached your goal weight" and SHALL NOT render a
  projection line, instead of "Not on track at your current trend"

#### Scenario: Old lifetime history does not falsely trigger "already at goal"
- **WHEN** a user's earliest-ever recorded `weight` sits on the opposite side of their
  `weight_goal` from their current 30-day trend (e.g. they weighed 60kg years ago, have since
  gained to a flat 90kg trend, and just set a 75kg goal to lose weight)
- **THEN** the chart SHALL NOT display "You've reached your goal weight", since direction is
  determined from the 30-day regression window's boundary EMA values, not the lifetime-earliest
  record — and SHALL instead display "Not on track at your current trend" for this flat trend

#### Scenario: Crossing beyond the horizon
- **WHEN** a user's trend moves toward their goal but the computed crossing date is more than 365
  days out from the regression window's most recent day
- **THEN** the chart SHALL display "Not on track at your current trend" and SHALL NOT render a
  projection line

#### Scenario: Crossing exactly at the horizon boundary
- **WHEN** a user's trend moves toward their goal and the computed crossing date is exactly 365
  days from the regression window's most recent day
- **THEN** the chart SHALL render the projection line and ETA text — day 365 is within the horizon,
  not beyond it

#### Scenario: Projection is independent of zoom selection
- **WHEN** a user switches zoom level between Week, Month, and Year
- **THEN** the regression window (last 30 calendar days of EMA data) and the resulting crossing
  date SHALL NOT change

#### Scenario: No projection without a goal
- **WHEN** a user has no `weight_goal` record
- **THEN** the chart SHALL NOT compute or render a projection line or ETA text

