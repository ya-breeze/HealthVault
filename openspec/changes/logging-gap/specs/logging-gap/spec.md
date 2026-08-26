## ADDED Requirements

### Requirement: Logging Gap window and inputs
The system SHALL compute, for the authenticated caller, a Logging Gap over a trailing 28-calendar-day
window ending the day before the caller's current Logged Day (per `food-day-completeness`'s Local
Day boundary) — the current, in-progress day SHALL NEVER be included in the window.

The computation SHALL use exactly these inputs, none stored, all fetched fresh on each computation:

- The caller's Nutrition Target `calories` figure (`GET /api/users/me/nutrition-target`).
- The caller's `weight` records covering the window plus enough additional lead-in days for the
  Trend Weight EMA to converge before the window's first day (mirroring the existing weight Trend
  Projection's own lead-in behavior).
- The caller's Day Completeness state for each day in the window (`GET /api/food/completeness`).
- The caller's daily logged-calorie totals for each day in the window (`GET /api/food/daily-totals`).

#### Scenario: Today is excluded from the window
- **GIVEN** a caller's current Logged Day is 2026-08-26
- **WHEN** the system computes their Logging Gap
- **THEN** the 28-day window runs from 2026-07-29 through 2026-08-25 inclusive, and no data from
  2026-08-26 is used

### Requirement: Outlier rejection on raw weigh-ins
Before computing the Trend Weight EMA, the system SHALL walk the window's (lead-in-extended) raw
`weight` records in chronological order and reject a record when the implied rate of change from
the previous **kept** record exceeds 2.0 kg per day: `|value[i] - lastKept.value| / (day[i] -
lastKept.day) > 2.0`. A rejected record SHALL NOT become the new `lastKept` reference for
evaluating subsequent records. Rejected records SHALL NOT be modified or removed from the
underlying `weight` data and SHALL continue to appear on the weight chart unchanged — rejection
applies only to this capability's own regression input.

#### Scenario: A single implausible reading is excluded from the trend
- **GIVEN** a caller's weigh-ins are 91.0 kg, then 91.2 kg the next day, then a 75.0 kg reading the
  day after that, then 91.4 kg the day after that
- **WHEN** the system computes the Logging Gap
- **THEN** the 75.0 kg reading is excluded from the trend regression, the 91.4 kg reading is
  evaluated against 91.2 kg (the last kept reading) rather than against 75.0 kg, and the 75.0 kg
  reading is unaffected everywhere else in the product

#### Scenario: A gradual multi-day change is not rejected
- **GIVEN** a caller's weigh-ins fall by 1.5 kg over 3 consecutive days (0.5 kg/day)
- **WHEN** the system computes the Logging Gap
- **THEN** none of those weigh-ins are rejected as outliers

### Requirement: Trend Weight and slope
The system SHALL compute Trend Weight as the exponential moving average (alpha 0.25) of the
outlier-filtered, day-bucketed `weight` series, using the same EMA function used elsewhere in the
product for weight trend rendering. The system SHALL then fit an ordinary least-squares regression
of `(day_offset, trend_weight_value)` over exactly the last 28 calendar days of that EMA series,
producing a slope in kg/day.

#### Scenario: Slope reflects a sustained loss
- **GIVEN** a caller's Trend Weight falls steadily by roughly 0.3 kg/day across the 28-day window
- **WHEN** the system fits the regression
- **THEN** the resulting slope is negative and approximately -0.3

### Requirement: Implied Intake and Logging Gap
The system SHALL compute:

```
Implied Intake = Nutrition Target calories + (trend slope kg/day * 7700)
Mean Logged Intake = average of GET /api/food/daily-totals' calories field,
                      restricted to days in the window whose Day Completeness state
                      (per food-day-completeness) is complete or confirmed_complete
Logging Gap = Implied Intake - Mean Logged Intake
```

Days in the window whose Day Completeness state is `unconfirmed` or `incomplete` SHALL be excluded
entirely from Mean Logged Intake — never averaged in as a zero-intake day.

Neither "TDEE" nor "total calories" SHALL be used as an identifier or user-facing label anywhere
this capability's code, API, or UI surfaces this computation — `total_calories` is an existing,
unrelated table (exercise calories).

#### Scenario: Only Complete/Confirmed Complete days count toward Mean Logged Intake
- **GIVEN** a 28-day window with 10 Complete days, 2 Confirmed Complete days, 6 Unconfirmed days,
  and 10 Incomplete (zero-occasion) days
- **WHEN** the system computes Mean Logged Intake
- **THEN** only the 12 Complete/Confirmed Complete days' logged totals are averaged, and the 16
  Unconfirmed/Incomplete days contribute nothing to either the sum or the divisor

### Requirement: Uncertainty interval
The system SHALL compute an uncertainty interval for the Logging Gap as:

```
formulaError = 0.10 * Nutrition Target calories
trendErrorKcal = 7700 * SE(slope)
interval = sqrt(formulaError^2 + trendErrorKcal^2)
```

where `SE(slope)` is the standard ordinary-least-squares slope standard error computed from the
same 28-day `(day_offset, trend_weight_value)` points used for the regression:
`sqrt(sum(residuals^2) / (n - 2)) / sqrt(sum((x - mean(x))^2))`. This is defined only when the
window contains at least 3 distinct days with a Trend Weight value (`n >= 3`, so `n - 2 >= 1`).
When fewer than 3 distinct days are available, `trendErrorKcal` SHALL be treated as unbounded,
which — combined with the silence rule below — SHALL always suppress a Logging Gap output for that
computation. Logged intake's own systematic estimation bias SHALL NOT be included as a term in this
formula.

#### Scenario: Interval combines both known error sources
- **GIVEN** a Nutrition Target of 2750 kcal (formulaError = 275) and a computed slope standard
  error whose kcal-equivalent is 195
- **WHEN** the system computes the interval
- **THEN** the result is `sqrt(275^2 + 195^2)`, approximately 337

#### Scenario: Fewer than 3 distinct trend days suppresses output
- **GIVEN** a 28-day window whose weigh-ins, after outlier rejection, fall on only 2 distinct days
- **WHEN** the system computes the Logging Gap
- **THEN** the interval is treated as unbounded and the silence rule below applies

### Requirement: Hard floor and silence rule
Before computing anything else, the system SHALL apply a hard floor: if fewer than 2 raw weigh-ins
survive outlier rejection within the window, or if fewer than 3 days in the window are Complete or
Confirmed Complete, the system SHALL NOT compute a slope, Implied Intake, Mean Logged Intake,
Logging Gap, or interval, and SHALL report the "not enough data yet" state directly.

The 3-valid-day minimum matches the precedent `food-day-completeness`'s own "Downstream coverage
contract" requirement already set for this exact kind of feature (it names "an adaptive-TDEE
computation" as an example, scoped to a rolling 7-Logged-Day window). That contract's window length
doesn't literally apply here — this feature's window is 28 days, not 7 — but its valid-day floor is
adopted directly rather than re-derived, because the interval formula's two terms (`formulaError`,
`trendErrorKcal`) carry no term for Mean Logged Intake's own sampling variance: a single Complete
day that happens to be a plausible-but-atypical eating day would not necessarily widen the interval
at all, so the silence rule alone cannot be relied on to catch a thin-logged-days case the way it
catches a thin-weigh-ins case. A 3-day floor does not fully close that gap (see design.md "Risks")
but keeps this feature's minimum no weaker than the one already established for its category.

When the hard floor is not hit, the system SHALL compute the Logging Gap and interval as above, and
SHALL suppress the Logging Gap output — reporting the same "not enough data yet" state — whenever
`abs(Logging Gap) < interval`, i.e. the range `[Logging Gap - interval, Logging Gap + interval]`
includes zero. No separate minimum-valid-day threshold beyond the hard floor's 3-day minimum SHALL
be applied — a thin window is expected to produce a wide interval that triggers this rule on its
own.

#### Scenario: Hard floor with a single weigh-in
- **GIVEN** a caller has only 1 `weight` record inside the 28-day window
- **WHEN** the system computes the Logging Gap
- **THEN** it reports "not enough data yet" without computing a slope or Implied Intake

#### Scenario: Hard floor with too few valid days
- **GIVEN** a caller has 5 weigh-ins across the window but only 2 days in the window are Complete or
  Confirmed Complete (the rest are Unconfirmed or Incomplete)
- **WHEN** the system computes the Logging Gap
- **THEN** it reports "not enough data yet" without computing Mean Logged Intake

#### Scenario: Statistical silence when the interval covers zero
- **GIVEN** a computed Logging Gap of 150 kcal/day and an interval of 310 kcal/day
- **WHEN** the system evaluates the silence rule
- **THEN** it reports "not enough data yet" instead of showing 150, since `[-160, 460]` includes zero

#### Scenario: A gap clearly outside the interval is shown
- **GIVEN** a computed Logging Gap of 900 kcal/day and an interval of 310 kcal/day
- **WHEN** the system evaluates the silence rule
- **THEN** it shows the Logging Gap and its interval, since `[590, 1210]` does not include zero

#### Scenario: A gap exactly equal to the interval is shown
- **GIVEN** a computed Logging Gap of 310 kcal/day and an interval of 310 kcal/day
- **WHEN** the system evaluates the silence rule
- **THEN** it shows the Logging Gap and its interval, since the comparison is strict (`abs(Logging
  Gap) < interval`) and equality does not qualify as covering zero

### Requirement: Nutrition Target unavailable is a distinct state
When `GET /api/users/me/nutrition-target` returns HTTP 422, the system SHALL report a state distinct
from "not enough data yet" — directing the caller to complete their profile and/or goal weight —
rather than the data-volume message, since the blocker is a missing precondition, not insufficient
history.

#### Scenario: Missing goal weight shows a distinct message
- **GIVEN** a caller has a complete profile and measurements but no goal weight set
- **WHEN** the system attempts to compute their Logging Gap
- **THEN** it reports a state directing them to set a goal weight, not "not enough data yet"

### Requirement: Logging Gap Card content and placement
The system SHALL render a Logging Gap Card as a Food Card, registered in the dashboard's existing
card order/visibility mechanism (see `dashboard-ui`), defaulting to visible.

The card SHALL render exactly one of the following states:
- A computed Logging Gap with its interval, expressed as a kcal/day range (e.g. "590–1210 kcal/day
  not logged"), never as a bare point estimate.
- "Not enough data yet" (covers both the hard floor and statistical silence cases).
- A distinct "complete your profile/goal weight" state when the Nutrition Target is unavailable
  (422).

When at least one raw weigh-in inside the window was excluded by outlier rejection, the card SHALL
additionally show a note that a reading was excluded, regardless of which of the three states above
is showing.

The card SHALL disclose, as static copy, that logged intake is estimated from photo recognition and
may carry its own bias not reflected in the shown interval.

#### Scenario: Outlier note shows alongside a computed gap
- **GIVEN** a Logging Gap is computed and shown, and one weigh-in in the window was excluded as an
  outlier
- **WHEN** the card renders
- **THEN** it shows both the Logging Gap range and the outlier-excluded note

#### Scenario: Outlier note shows alongside "not enough data yet"
- **GIVEN** the hard floor was hit and one of the caller's few weigh-ins was excluded as an outlier
- **WHEN** the card renders
- **THEN** it shows "not enough data yet" together with the outlier-excluded note
