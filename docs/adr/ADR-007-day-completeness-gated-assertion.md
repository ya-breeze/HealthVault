# ADR-007: Day Completeness as a Heuristic-Gated User Assertion

## Status
Accepted

## Context and Problem Statement

Phase 4 (`todo.md`'s dashboard/food-tracking initiative) needs a rolling-window nutrition signal,
but the backend has no concept of "was a day's food logging actually complete" — it only has a raw
count of `FoodMeal` rows, no timezone, and no way to tell a genuinely light-eating day from a day
the user just forgot to log. 17 days of real food-log history (24 meals, zero manual entries, five
zero-entry days) showed a plain `≥3 meals` heuristic can both under- and over-fire: a single sitting
photographed in three follow-up shots 3 minutes apart passes the threshold on its own. How should
"is this day complete enough to feed a downstream feature" be decided and stored?

## Decision Drivers

- A day with genuinely few Eating Occasions (a light day, a fasting day, a day eaten out with no
  photos) is indistinguishable from an under-logged day by row count alone — only the user knows
  which one it was.
- A single meal split across several follow-up photos must not inflate the occasion count.
- The signal needs to be queryable per-day by a future feature (Phase 4) without that feature
  re-deriving occasion-collapsing or confirmation logic itself.
- The existing metric-type registry (`typeRegistry`, used by every vital/chart) is built for
  time-series measurements with a value and a unit — a boolean day-flag doesn't fit that shape and
  would surface meaninglessly in charts/vitals cards if forced into it (the same reasoning ADR-002
  used to keep Goal Weight a metric type: right shape wins over reusing an existing registry).

## Considered Options

- **Automatic-only: a day is Complete iff occasion count ≥ threshold, full stop, no user input** —
  simplest, but the 17-day sample shows this both misses days that were actually fine (light-eating,
  no threshold reached) and never lets a user tell the system "I really did eat only twice today."
- **A full user-editable completeness flag, settable and unsettable regardless of occasion count** —
  most flexible, but no longer "the heuristic decides when to ask" — it becomes a general-purpose
  self-report the user has to remember to maintain, closer to a habit-tracker checkbox than a signal
  derived from what's already logged.
- **Heuristic-gated assertion: the threshold auto-classifies Complete days and asks nothing; only
  below-threshold days expose a confirm control, and an auto-Complete day exposes none** — chosen.
  The heuristic's only job is deciding *when to ask*, not judging the day itself once asked.
- **Store completeness as a new metric type in `typeRegistry`** — rejected: duplicates the type
  name across nine files (per the existing registry's own fan-out) and would surface a boolean in
  charts and vitals cards where it is meaningless; it isn't a time-series measurement.

## Decision Outcome

Chosen: **Day Completeness is a user assertion, gated by a heuristic that only decides when to
ask.** Concretely:

- **Eating Occasion collapsing**: a pure function groups a Logged Day's `FoodMeal.LoggedAt`
  timestamps by proximity — a new occasion starts whenever the gap to the previous timestamp
  exceeds 10 minutes. This replaces raw meal-row count everywhere Day Completeness is computed, so
  a single sitting logged as 2-3 rows is not double-counted.
- **Storage**: a dedicated `FoodDayCompletion` table (`UserID`, `LocalDate` unique per user,
  `ConfirmedAt`) — row presence is the assertion, deleting it retracts. Not a metric type: this is a
  flag on a date, not a time-series measurement, and forcing it into `typeRegistry` would duplicate
  the type name across nine files for no benefit, per the drivers above.
- **No override of an automatic Complete day.** A day that reaches Usual Meals Per Day exposes no
  confirm/retract control in either direction — the heuristic decided the day needs no question, so
  none is asked, even if the user knows they under-logged a large meal that day. Accepted as a known
  gap (see Consequences), not solved here by inventing a downgrade mechanism that would turn "the
  heuristic decides when to ask" into "the heuristic decides when to ask, except when it's wrong,
  which the user judges" — a materially different, unbuilt feature.
- **3-of-7 minimum downstream coverage.** A rolling-7-day consumer (Phase 4) only computes and shows
  a number once at least 3 of the most recent 7 calendar days are Complete or Confirmed Complete;
  below that it states "not enough data." Chosen because it's the number already used as a worked
  example in the idea's own grilling comment, so this codifies it once rather than leaving every
  future consumer to re-derive or disagree on the same number independently.
- **Threshold changes recompute past days on read, not at write/confirm time.** Usual Meals Per Day
  is read fresh on every Day Completeness computation; there is no per-day snapshot of "what the
  threshold was then." Changing it immediately re-evaluates every past day's auto-Complete
  classification. This matches how BMI bands and the weight trend projection already treat their own
  inputs (recomputed on read, not memoized at write time) — one mental model for "does changing a
  setting rewrite history" across the app, and simpler to implement than tracking a
  threshold-as-of per day.

### Consequences

- A user who logged, say, 3 occasions but knows they missed a large dinner has no way to flag that
  day as actually incomplete — accepted as a real, currently-unaddressed gap. Revisit only if
  false-positive Complete days turn out to be common in practice; nothing in the 17-day sample
  (2/17 auto-Complete at threshold 3) suggested urgency at proposal time.
- Changing Usual Meals Per Day is a global, retroactive reclassification of every past day, not a
  point-in-time setting — a user cannot see "what my completeness looked like under my old
  threshold" after changing it.
- `FoodDayCompletion` is scoped strictly to food logging; it does not attempt to generalize into a
  cross-metric-type "day completeness" concept for other data (e.g. weight logging frequency) —
  that would be a separate, unbuilt feature if ever wanted.
