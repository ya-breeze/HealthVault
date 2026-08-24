## ADDED Requirements

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
a fixed 365-day count from **today** (not calendar-month arithmetic, and not from the most recent
day that happens to have data): a crossing at or before day 365 from today is within the horizon,
and a crossing after day 365 is beyond it. A crossing at or before today SHALL be treated as not
on track, so a user whose last weigh-in is weeks old is never shown an ETA that has already
passed. Otherwise the system SHALL render a dashed projection line from the
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
- **WHEN** a user has at least 5 `weight` records spanning at least 14 days, but their EMA points
  inside the last 30 calendar days number fewer than 5 or span fewer than 14 days (e.g. they
  stopped logging weight months ago, or logged a handful of times in one week)
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

#### Scenario: A height can be recorded from the weight page
- **WHEN** an authenticated user views their own weight page and has no `height` record on file
- **THEN** the page SHALL offer a shortcut that opens the Add-record form targeted at `height`,
  and the shortcut SHALL disappear once a height exists. `height` is a secondary type, and the
  dashboard's More Data list — the only place secondary type pages are linked — is filtered by
  data presence, so without this shortcut a user with no height has no route to the page where a
  height could be added, and the BMI bands and readout are permanently unreachable for exactly
  the users they are gated on.

#### Scenario: Projection status is not asserted before it is known
- **WHEN** the weight history backing the projection is still loading, or its fetch failed
- **THEN** the chart SHALL NOT display "Not enough data to project yet", since that states a fact
  about the user's records that is not yet known; while loading it SHALL display no projection
  message, and on failure it SHALL indicate that the history could not be loaded

#### Scenario: ETA never falls in the past
- **WHEN** a user's most recent `weight` record is several weeks old and the fitted trend line
  crosses the goal at a date earlier than today
- **THEN** the chart SHALL display "Not on track at your current trend" and SHALL NOT render a
  projection line or an ETA date

#### Scenario: Projection point count is bounded
- **WHEN** an on-track projection's crossing is far enough ahead that daily spacing would produce
  more synthetic points than real buckets
- **THEN** the system SHALL widen the spacing between synthetic points so their count stays
  bounded, keeping the crossing point itself on the chart, rather than compressing the real
  weight series into a sliver of the categorical x-axis

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

## MODIFIED Requirements

### Requirement: Point-in-time type aggregation
For point-in-time types (`heart_rate`, `heart_rate_variability`, `weight`, `height`, `weight_goal`, `blood_pressure`, `blood_glucose`, `oxygen_saturation`, `body_temperature`, `skin_temperature`, `respiratory_rate`, `resting_heart_rate`, `vo2_max`, `body_fat`, `lean_body_mass`, `bone_mass`, `speed`, `basal_metabolic_rate`), the chart SHALL render as follows:
- **Day**: raw records plotted as a line.
- **Week, Month, or Year**: a line of per-bucket averages (per-day for Week/Month, per-month for Year), with a shaded band spanning each bucket's minimum to maximum value.

`weight_goal`'s own `/data/weight_goal` page follows this same rule like any other point-in-time type; this is separate from the goal reference line rendered on the `weight` chart (see "Goal weight reference line" above).

#### Scenario: Week view shows an averaged line with a band
- **WHEN** a user views `heart_rate` at Week zoom
- **THEN** the chart SHALL plot one averaged point per day connected as a line, with a shaded region behind it spanning each day's min–max range

#### Scenario: Year view does not plot raw records
- **WHEN** a user views `heart_rate` at Year zoom
- **THEN** the chart SHALL plot one averaged point per month with a min–max band, and SHALL NOT request or render the underlying raw records for the year
