# ADR-010: Report a Logging Gap, Not an Adaptive TDEE

## Status
Proposed

## Context and Problem Statement

Idea #10 started as "adaptive TDEE from energy balance (replaces activity multiplier)": use the
weight trend plus logged intake to derive a measured Total Daily Energy Expenditure, and use it in
place of ADR-006's steps-inferred activity multiplier. Grilling the idea (idea-forge issue,
"Grilled — the feature changed shape" comment, 2026-08-25) found that the data this app actually
has cannot support that framing: TDEE is expenditure, and computing expenditure from a weight trend
requires an *independent* intake figure to solve `TDEE = intake - trend-implied deficit/surplus`
against. This app's only intake source is the food log itself — the same log the feature would need
to distrust (under-logging is the whole reason to build this) to treat as ground truth for solving
the other side of the equation. There is no third, independent expenditure signal anywhere in the
app's data (no wearable-measured burn, no lab test) to break that circularity. How should this
feature be reframed so it reports something the data can actually support, and how should its
several safety mechanisms be recorded so a future reader doesn't have to reconstruct them from the
grilling comment or `design.md`'s prose?

## Decision Drivers

- Energy balance alone cannot separate expenditure from an intake-logging gap without a third,
  independent signal this app doesn't have — reporting "TDEE" would imply a precision the math
  doesn't deliver.
- The feature must never suggest a false point estimate: an unquantified error (photo-portion
  estimation bias, the activity multiplier's own unmeasured error) is easy to mistake for precision
  if presented as a single number.
- A future reader of the code or this file should not have to re-derive from `design.md`'s prose
  which architectural choices here are load-bearing (the outlier rule's rejection cap, the
  interval's quadrature formula, the distinct fetch-failure state) versus incidental detail.
- ADR-006's steps-inferred activity multiplier is a working, shipped mechanism computed from a
  different signal (steps) entirely; nothing this feature reports is precise enough to safely
  replace it.

## Considered Options

- **Compute and report a TDEE, replacing ADR-006's activity multiplier** — the idea's original
  framing. Rejected: without an independent expenditure signal, any number produced this way is
  actually a residual of "what the trend implies minus what got logged," not expenditure — labeling
  it TDEE would misrepresent what the app can actually measure.
- **Report a TDEE but keep ADR-006's multiplier as a separate, parallel figure** — rejected for the
  same reason; the label is wrong regardless of whether it also replaces something.
- **Report the Logging Gap — `Implied Intake - Mean Logged Intake` — as its own quantity, with an
  explicit uncertainty interval, and leave ADR-006 untouched** — chosen. This is exactly the
  quantity the available data can support: a signed estimate of how much the food log and the
  weight trend disagree, not a claim about expenditure.

## Decision Outcome

Chosen: **the feature computes and surfaces a Logging Gap, not a TDEE, and never touches ADR-006's
activity multiplier.** Concretely, on the Logging Gap Card:

- **Terminology.** Neither "TDEE" nor "total calories" is used as an identifier or user-facing
  label anywhere in this capability's code, API, or UI — `total_calories` is already an existing,
  unrelated table (exercise calories), and "TDEE" specifically claims a measurement this feature
  doesn't make. See `CONTEXT.md`'s Implied Intake / Logging Gap / Trend Weight glossary entries.
- **Silence over a clamp is the safety mechanism**, not a floor/ceiling on the reported number. Two
  independent gates run before anything is shown: a hard floor (fewer than 2 surviving raw
  weigh-ins, more than 3 rejected within the window, a same-day-sibling-ambiguous bootstrap
  rejection, fewer than 3 valid logging-complete days, or the most recent weigh-in more than 7 days
  stale) skips the computation entirely; failing that, statistical silence suppresses the computed
  Logging Gap whenever its own interval still covers zero. Both render the same "not enough data
  yet" text — the distinction (too little data vs. enough data but inconclusive) isn't something a
  user needs to act on differently. See `design.md` decision 5, `specs/logging-gap/spec.md`'s "Hard
  floor and silence rule."
- **Rate-based outlier rejection, not a flat per-point delta.** A raw weigh-in is rejected against
  the last *kept* weigh-in when the implied rate exceeds 2.0 kg/day, with a bootstrap step to avoid
  anchoring on a bad leading reading, same-day records exempted from the rate check (diluted into
  their day's bucket instead), and a same-day-sibling suppression rule that forces the hard floor
  when a bootstrap rejection has a surviving same-day sibling that was never itself rate-validated.
  The actual bound against a persistent wrong cluster (e.g. a shared scale) is an explicit
  **rejection cap**: more than 3 raw rejections within the 28-day window trips the hard floor rather
  than trusting whichever cluster survived. This closes the failure mode only when the genuine
  cluster logs often enough (roughly weekly or more) — a known, accepted residual gap, not solved
  here. See `design.md` decision 2 for the full reasoning and its two documented residual
  limitations (the bootstrap's local pairwise walk, and the cap's own blind spot against a sparse
  genuine cadence).
- **Interval: quadrature of two independent, fixed/derived error terms** —
  `sqrt(formulaError^2 + trendErrorKcal^2)`, where `formulaError` is a fixed 10% of Nutrition Target
  calories (Mifflin-St Jeor's own cited BMR accuracy) and `trendErrorKcal` is `7700 * SE(slope)`
  from the 28-day OLS fit's own standard error, treated as unbounded when fewer than 3 distinct
  Trend Weight days exist. Neither term is derived from this app's own users; both are accepted as
  the best available proxy rather than an invented, equally-unvalidated alternative. See
  `design.md` decision 3.
- **A distinct "temporarily unavailable" state** for a non-422 failure (network error, 5xx) on any
  of the card's four independent requests (weight, Nutrition Target, Day Completeness, Daily
  Totals) — never folded into "not enough data yet." Unlike a `VitalCard`, this card doesn't read
  from the dashboard's shared `dataMap`, so one request failing has no reason to correlate with any
  other card's state; telling the user to "gather more data" when the real problem is a broken
  endpoint would be misleading. See `design.md` decision 6.
- **Dashboard card registry generalized from `DataType`-only to `DataType | 'logging_gap'`**
  (`CardId`), since the Logging Gap Card has no `/api/data/{type}` presence signal to gate on — its
  "nothing to show" cases are its own internal content states, not the presence system's. A small,
  additive widening; existing `DataType`-backed cards are unaffected. See `design.md` decision 8.

### Consequences

- ADR-006's activity multiplier remains the only expenditure input to the Nutrition Target; nothing
  in this change alters, recomputes, or supersedes it. A future feature that *does* have an
  independent expenditure signal (e.g. a wearable integration) would be the one to revisit ADR-006,
  not this one.
- The Logging Gap can be silent (no output) for extended periods for a user with sparse weigh-ins,
  sparse food logging, or a persistently ambiguous weigh-in pattern — by design, since a suppressed
  number is preferred over a confident, wrong one. Users are not shown a "why is nothing shown"
  breakdown of which gate suppressed it; the card's copy is uniform "not enough data yet" text.
- The two unquantified error sources this feature knowingly excludes from the interval — photo
  intake estimation bias, and the activity multiplier's own error — are disclosed as static card
  caveats instead of being folded into the number, so a user reading only the range still sees the
  disclosure. If either is ever quantified from real usage data, the interval formula would need
  revisiting (see `design.md` "Risks").
