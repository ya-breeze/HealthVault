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

A candidate record sharing its calendar day with `lastKept` SHALL NOT be rate-checked against it
(the day-gap denominator is undefined at zero); such a record is always provisionally kept, and it
SHALL NOT update `lastKept` — the reference for the next different-day candidate remains whichever
record most recently passed a rate check (or the bootstrap-selected initial anchor), never a record
that was merely exempted from one. Only after this entire rejection walk (including the initial-anchor bootstrap below) has finished do the
surviving raw records get collapsed into calendar days by the day-bucketing step — day-bucketing
never runs partway through the walk, so every step of this requirement, including the bootstrap
below, operates on raw, un-bucketed records in chronological order.

Before evaluating the third and later records, the system SHALL first validate the initial
`lastKept` candidate, considering only raw records and skipping same-day pairs per the rule above
(a same-day pair is neither rejected nor treated as "agreeing" here — the search simply continues to
the next record on a different calendar day): if the first two chronological records that fall on
different calendar days exceed the 2.0 kg/day rate against each other, the earlier one SHALL be
rejected (per the rule above) and the check SHALL repeat against the next different-day record until
two consecutive different-day records agree or only one remains, which becomes the initial
`lastKept`. This prevents a single bad leading reading from becoming an unvalidated reference that
wrongly rejects several subsequent good readings.

This validation tracks a single bootstrap candidate, initialized to the first raw record, which
persists unchanged across any number of same-day-exempted records: each record sharing the
candidate's calendar day is skipped in turn (per the same-day rule above) without advancing the
candidate, so the next different-day record is always rate-checked against that same candidate, not
against whichever same-day record was walked most recently. When a candidate is rejected as "the
earlier one," any same-day sibling of that rejected candidate is unaffected by the rejection — it was
already provisionally kept under the same-day rule and is not itself rejected or reconsidered as a
replacement candidate; like any other same-day-exempted record, it survives into its calendar day's
bucket once day-bucketing runs — collapsed with any other surviving same-day records the same way any
day-bucket is, or, when the rejected candidate was its only same-day companion, passed through as
that day's sole bucketed value, unaveraged with anything, since a rejected record never enters the
bucket. A leading cluster of mutually-agreeing same-day records can therefore still reach the EMA as
that day's bucketed value even when the bootstrap rejects a different, non-same-day leading record.
A lone same-day sibling of a bootstrap-rejected record would otherwise reach the EMA exactly as an
un-rejected leading record would have, not softened by averaging and never rate-checked against
whatever record actually displaced its rejected candidate — not a case the downstream
interval-widening/statistical-silence mechanism (decision 3/4 of design.md) is guaranteed to catch,
since with the minimum `n = 3` EMA days that mechanism needs to produce a defined `SE(slope)` at
all, such a sibling can land close enough to collinear with the following genuine readings that the
interval is not reliably widened past decision 5's silence threshold (see design.md decision 2's
"residual limitation of the same-day exemption" for the full analysis).

**Same-day-sibling suppression.** When the bootstrap validation above rejects its candidate ("the
earlier one") and that candidate has one or more same-day-exempted siblings, the system SHALL treat
the window's weigh-in data as ambiguous and apply the hard floor below ("not enough data yet"), the
same as the rejection cap, instead of letting the surviving sibling reach the EMA unvalidated. This
closes the residual limitation above without re-validating the sibling against anything and without
reopening the order-dependence problem the same-day rule exists to avoid: the trigger is the
structural fact that a bootstrap rejection has a same-day sibling at all, not a comparison between
the sibling and any other record, so no new record-to-record check is introduced. See design.md
decision 2 for the full rationale, including the accepted cost of over-suppression this trades for.

This condition SHALL be determined directly by the rejection walk itself (an explicit output
alongside `kept`/`rejected`, set once during the bootstrap step), not reconstructed by a caller from
`kept` and `rejected` after the fact. Two consequences follow. First, the condition SHALL fire
regardless of whether the rejected candidate's day or its sibling's day falls inside the visible
28-day window or the lead-in extension — the bootstrap runs on the chronologically first records of
the whole lead-in-extended range, so the pair it concerns is typically in the lead-in, not the
visible window, and a check that only sees the visible-window-filtered `kept`/`rejected` arrays
(as the rejection cap does) would silently miss it. Second, this condition SHALL NOT be
approximated as "some calendar day appears in both the `kept` and `rejected` arrays": the ordinary
rejection walk, independent of the bootstrap, can produce a kept record and a rejected record on
the same calendar day with no same-day-sibling relationship between them at all (e.g. `lastKept` at
91.0 kg on day 1; day 2 carries two records, 94.0 kg and 91.2 kg, each independently rate-checked
against the day-1 `lastKept` since neither shares `lastKept`'s day — 94.0 kg fails and is rejected,
91.2 kg passes and is kept). That day-2 overlap is not the condition this rule targets and SHALL NOT
by itself trigger the hard floor.

**Rejection cap.** If more than 3 raw records within the 28-day window itself (excluding the
lead-in extension) are rejected by this rule, the system SHALL treat the window's weigh-in data as
ambiguous and apply the hard floor below ("not enough data yet") rather than compute a trend from
whichever records survived. This bounds the case where two internally-consistent but
mutually-disagreeing clusters of readings exist in the window (e.g. a shared scale used by two
people, per design.md decision 2): a lone bad reading is rejected at most once or twice (the bad
reading itself, plus at most one bootstrap rejection), while a sustained disagreement between two
clusters rejects one entire side of it for as long as it persists — which trips this cap once that
rejected side contributes more than 3 different-day readings within the window. This closes the
failure mode when the genuine cluster logs often enough to cross that threshold (the ~3.33-day mean
weigh-in gap this design otherwise assumes yields roughly 8 readings per 28 days, well past it), but
not in general: a genuine cadence sparse enough to stay at or under roughly one reading per 7 days
never crosses the cap even while a denser wrong cluster persists throughout, since only the
non-`lastKept` cluster's readings are ever rejected. See design.md decision 2's "residual limitation
of the cap" for this accepted, disclosed gap.

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

#### Scenario: A bad leading reading does not poison the trend for days
- **GIVEN** a caller's first weigh-in in the window is 75.0 kg, followed by 91.0 kg the next day,
  92 kg five days later, and 91.5 kg three days after that, all inside the true ~91 kg trend
- **WHEN** the system computes the Logging Gap
- **THEN** the leading 75.0 kg reading is rejected (as the earlier of the disagreeing first pair),
  91.0 kg becomes the initial `lastKept`, and neither of the two later 91-92 kg readings is
  rejected

#### Scenario: Same-day records are never rate-checked against each other
- **GIVEN** a caller has two weigh-ins on the same calendar day, 91.0 kg and 91.4 kg
- **WHEN** the system computes the Logging Gap
- **THEN** neither reading is rejected by the outlier rule, since the day-gap denominator between
  them is zero and same-day candidates are never rate-checked against `lastKept`

#### Scenario: A same-day record never becomes the reference for later comparisons
- **GIVEN** `lastKept` is a 91.0 kg record from 2026-08-10; the first 2026-08-11 record (91.2 kg) is
  on a different calendar day than `lastKept`, so it IS rate-checked against it, passes, and becomes
  the new `lastKept`; a second, same-day 2026-08-11 record (91.5 kg) shares its calendar day with
  that new `lastKept`, so it is exempt and only provisionally kept; a 91.6 kg record follows on
  2026-08-12
- **WHEN** the system evaluates the 2026-08-12 record
- **THEN** it is rate-checked against the first 2026-08-11 91.2 kg record (the one that actually
  passed a rate check and became `lastKept`), not against the second, provisionally kept 2026-08-11
  91.5 kg record, and not against the 2026-08-10 91.0 kg record either — only a record that is
  itself sharing a calendar day with the *current* `lastKept` is exempt from the rate check; a
  record's own day differing from `lastKept`'s day means it is rate-checked and, if it passes,
  becomes the new `lastKept` in turn

#### Scenario: A same-day sibling of a bootstrap-rejected leading record triggers suppression
- **GIVEN** a caller's first two weigh-ins are both on 2026-08-01, 75.0 kg and 75.2 kg, followed by
  91.0 kg on 2026-08-02 and 91.2 kg on 2026-08-03
- **WHEN** the system computes the Logging Gap
- **THEN** the bootstrap candidate is 75.0 kg (the first record); the 75.2 kg record is same-day
  exempted and does not advance the candidate; 91.0 kg is rate-checked against 75.0 kg (not against
  75.2 kg), disagrees, and 75.0 kg is rejected as the earlier of the pair; the 75.2 kg record is
  never itself rejected — it remains provisionally kept per the same-day rule — but because the
  rejected 75.0 kg candidate has this same-day sibling, the same-day-sibling suppression rule fires
  and the system reports "not enough data yet" rather than computing a trend from 91.0 kg, 91.2 kg,
  and the surviving 75.2 kg bucket

#### Scenario: A persistent wrong cluster trips the rejection cap instead of producing a wrong trend
- **GIVEN** a caller's weigh-ins within the 28-day window alternate between a true ~91 kg series and
  a second, internally consistent ~75 kg cluster (e.g. a shared scale), such that outlier rejection
  discards 4 raw records within the window
- **WHEN** the system computes the Logging Gap
- **THEN** it reports "not enough data yet" via the rejection cap, without computing a trend from
  either cluster

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

`formulaError`'s 10% figure is Mifflin-St Jeor's own BMR accuracy claim, applied here as a fixed
proxy for the whole Nutrition Target figure (BMR times ADR-006's steps-inferred activity
multiplier). The activity multiplier's own error is not separately quantified or added as a term —
see design.md's "Risks" for the accepted consequence.

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
survive outlier rejection within the window, or if more than 3 raw weigh-ins within the window were
rejected by outlier rejection (the rejection cap above), or if the bootstrap validation rejected a
candidate that had one or more same-day-exempted siblings (the same-day-sibling suppression rule
above — this condition is a structural fact about the rejection walk over the whole
lead-in-extended range and fires regardless of whether the rejected candidate or its sibling falls
inside the visible window or the lead-in), or if fewer than 3 days in the window are Complete or
Confirmed Complete, or if the most recent surviving weigh-in inside the window is more than 7 days
before the window's last day, the system SHALL NOT compute a slope, Implied Intake, Mean Logged
Intake, Logging Gap, or interval, and SHALL report the "not enough data yet" state directly.

The 7-day freshness condition exists because a stale-but-dense-and-near-linear weigh-in series can
otherwise produce a small `SE(slope)` (see the Uncertainty interval requirement) despite not
covering the days closest to the window's end — reduced spread from fewer recent points tends to
widen the interval but does not reliably do so on its own (see design.md decision 1). 7 days matches
`food-day-completeness`'s Downstream Coverage Contract window (see below) and is roughly twice the
~3.33-day mean weigh-in gap observed in idea #10's grilling data, so ordinary cadence never trips it.

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
`abs(Logging Gap) <= interval`, i.e. the closed range `[Logging Gap - interval, Logging Gap +
interval]` includes zero. At exact equality (`abs(Logging Gap) == interval`), zero sits exactly on
the boundary and is treated as covering, not resolved — the comparison includes equality (`<=`),
matching the "if the range includes zero" framing this rule is derived from literally, rather than
carving out the single point where the range's own edge touches zero (see the "gap exactly equal to
the interval is suppressed" scenario below). No separate minimum-valid-day threshold beyond the hard
floor's 3-day minimum SHALL be applied — a thin window is expected to produce a wide interval that
triggers this rule on its own.

#### Scenario: Hard floor with a single weigh-in
- **GIVEN** a caller has only 1 `weight` record inside the 28-day window
- **WHEN** the system computes the Logging Gap
- **THEN** it reports "not enough data yet" without computing a slope or Implied Intake

#### Scenario: Hard floor with a stale last weigh-in
- **GIVEN** a caller has 10 densely and near-linearly spaced weigh-ins early in the 28-day window,
  none within the last 8 days before the window's last day
- **WHEN** the system computes the Logging Gap
- **THEN** it reports "not enough data yet" without computing a slope, regardless of how narrow the
  regression's own standard error would otherwise be

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

#### Scenario: A gap exactly equal to the interval is suppressed
- **GIVEN** a computed Logging Gap of 310 kcal/day and an interval of 310 kcal/day
- **WHEN** the system evaluates the silence rule
- **THEN** it reports "not enough data yet", since the comparison includes equality (`abs(Logging
  Gap) <= interval`) and the range `[0, 620]` has zero on its own boundary, which counts as
  covering

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

The card SHALL render exactly one of the following content states:
- A computed Logging Gap with its interval, expressed as a kcal/day range, never as a bare point
  estimate. When the gap is positive (Implied Intake exceeds Mean Logged Intake), the range and
  copy SHALL read as unlogged intake (e.g. "590–1210 kcal/day not logged"). When the gap is
  negative (Mean Logged Intake exceeds Implied Intake), the copy SHALL read as logged intake
  exceeding what the trend implies, using the absolute values of the (sign-flipped) range rather
  than displaying a negative-to-negative range (e.g. a computed range of `[-1210, -590]` SHALL
  render as something to the effect of "590–1210 kcal/day logged more than the trend implies", not
  "-1210–-590 kcal/day not logged").
- "Not enough data yet" (covers both the hard floor and statistical silence cases).
- A distinct "complete your profile/goal weight" state when the Nutrition Target is unavailable
  (422).
- A distinct "temporarily unavailable" state when a non-422 failure (network error, 5xx, or a
  Nutrition Target error other than the 422 above) occurs on any of the card's four underlying
  requests — never conflated with "not enough data yet", since the two describe different problems
  (a retrieval failure vs. insufficient history) that call for different user reactions.

A loading state — distinct from all four content states above — SHALL be shown while any of the
card's four underlying requests (weight, Nutrition Target, Day Completeness, Daily Totals) is still
in flight, so the card never renders "not enough data yet" before its first fetch resolves.

When at least one raw weigh-in inside the window was excluded by outlier rejection, the card SHALL
additionally show a note that a reading was excluded, regardless of which of the four content
states above is showing (including "temporarily unavailable", if the weight request itself
succeeded and a different one of the four requests is what failed).

The card SHALL disclose, as static copy, two caveats about what the shown interval does not cover:
that logged intake is estimated from photo recognition and may carry its own bias, and that the
Nutrition Target feeding Implied Intake includes ADR-006's steps-inferred activity multiplier, whose
own error is not separately quantified or included in `formulaError` — see design.md's "Risks". A
systematic misestimate of activity can present as unlogged (or over-logged) intake that is really
activity mis-estimation.

#### Scenario: Outlier note shows alongside a computed gap
- **GIVEN** a Logging Gap is computed and shown, and one weigh-in in the window was excluded as an
  outlier
- **WHEN** the card renders
- **THEN** it shows both the Logging Gap range and the outlier-excluded note

#### Scenario: Outlier note shows alongside "not enough data yet"
- **GIVEN** the hard floor was hit and one of the caller's few weigh-ins was excluded as an outlier
- **WHEN** the card renders
- **THEN** it shows "not enough data yet" together with the outlier-excluded note

#### Scenario: A negative gap is rendered direction-aware, not as a negative range
- **GIVEN** a computed Logging Gap of -900 kcal/day and an interval of 310 kcal/day (Mean Logged
  Intake exceeds Implied Intake, range `[-1210, -590]`)
- **WHEN** the card renders
- **THEN** it shows a range of 590–1210 kcal/day framed as logged intake exceeding what the trend
  implies, and never renders a range with two negative numbers

#### Scenario: A loading state precedes any content state
- **GIVEN** the card has just mounted and its four requests have not yet resolved
- **WHEN** the card renders
- **THEN** it shows a loading state, not "not enough data yet" or any other content state

#### Scenario: A non-422 fetch failure shows a distinct "temporarily unavailable" state
- **GIVEN** the Day Completeness request fails with a 500 error, while the other three requests
  succeed
- **WHEN** the card renders
- **THEN** it shows a "temporarily unavailable" state, distinct from both "not enough data yet" and
  an unhandled error
