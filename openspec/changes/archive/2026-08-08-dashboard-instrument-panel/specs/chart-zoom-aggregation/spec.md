## ADDED Requirements

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
