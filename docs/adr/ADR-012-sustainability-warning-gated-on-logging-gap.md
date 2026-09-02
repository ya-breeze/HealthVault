# ADR-012: Sustainability Warning Gated on the Logging Gap's Corroboration

## Status
Proposed

## Context and Problem Statement

The nutrition card (registry id `logging_gap`, "Питание"/"Nutrition") already computes, on the way
to its shipped first and third rows, two numbers it never surfaces: the Mifflin-St Jeor BMR
(`computeNutritionTarget`, discarded after the activity multiplier is applied) and Mean Logged
Intake (`computeLoggingGap`, discarded after the gap subtraction). Both feed a sustainability check
worth adding to the card's empty middle row: logged intake below BMR, and a weight-loss rate faster
than the usual sustainable 1%/week band. The production data that motivated this — BMR 1799, mean
logged intake 1617, Logging Gap `-102 ± 222 → on_track` — hits exactly the intake case.

The two checks do not have the same relationship to trust, though. The rate-of-loss check reads a
measured weight trend; nothing about it depends on whether the food log is complete. The intake
check reads Mean Logged Intake, which is only as trustworthy as the log it's drawn from — and this
app's whole reason for having a Logging Gap line is that a food log commonly under-reports. Shipping
"you are eating below your BMR" ungated would fire on every user who simply doesn't log everything,
which is most users most of the time; it isn't a health finding, it's an artifact of incomplete
logging. How should the intake check be prevented from firing on unlogged food while the rate check
stays available to a user whose weight trend alone already says they're losing too fast?

## Decision Drivers

- The logging-gap line already computes exactly the corroborating signal the intake check needs:
  `on_track` means Implied Intake (from the weight trend) and Mean Logged Intake agree inside the
  gap's own uncertainty interval — i.e., the log is complete enough to trust for this purpose. There
  is no reason to invent a second corroboration mechanism when one already exists and is already
  computed on this same card.
- The rate check has no analogous trust problem: weight is measured by a scale, not self-reported
  and summed by the user, so gating it behind the food log's completeness would silence the exact
  user most at risk — someone who weighs in daily but logs food only a few days out of twenty-eight.
- The card's copy standard, set by the shipped `on_track` line ("your log matches your weight," not
  "all good"), is to state only what was actually measured. A gated intake check that still fires is
  consistent with that standard; an ungated one that fires on logging gaps rather than genuine
  under-eating is not.
- `checkHardFloor` (the existing gate before any regression is attempted) mixes a weight-only
  condition set with a food-log condition (fewer than 3 valid logging-complete days) into one
  boolean, which is the right shape for the logging gap line but the wrong shape for a rate check
  that must not depend on the food log at all.

## Considered Options

- **Gate both checks on `on_track`.** Rejected: this would silence the rate-of-loss check for
  exactly the sparse-logger-but-daily-weigher case the check exists to catch, since a sparse food
  log routinely fails to reach `on_track` (or reaches `not_enough_data`) regardless of how fast
  weight is actually moving.
- **Gate neither check.** Rejected for the intake check specifically: an under-logged food total is
  by far the most common reason logged intake looks low, and firing on it would be a false
  starvation warning aimed at ordinary under-logging rather than at genuine under-eating.
- **Invent a separate corroboration signal for the intake check**, rather than reusing the logging
  gap's own `on_track` result. Rejected: the logging gap line already answers exactly this question
  (does the log agree with the trend), so a second mechanism would duplicate it for no benefit and
  risk disagreeing with the row directly above it on the same card.
- **Gate the intake check on `on_track`, leave the rate check ungated, and split `checkHardFloor`
  into a weight-only `checkWeightFloor` plus the food-log condition** — chosen. This lets the rate
  check run behind the more permissive weight-only floor while the intake check inherits the full
  floor transitively, through its `on_track` gate (only `computeLoggingGap`, which requires the full
  hard floor to have already passed, can produce `on_track`).

## Decision Outcome

Chosen: **the intake check (`intake_below_bmr`) is gated on `gap.kind === 'on_track'`; the
rate-of-loss check (`loss_too_fast`) is not gated on the food log at all.** Concretely:

- `frontend/lib/loggingGap.ts`'s `checkHardFloor` is decomposed into `checkWeightFloor` (the four
  weight-only conditions: too few raw survivors, too many rejections, bootstrap sibling ambiguity, a
  stale last weigh-in) composed with the food-log condition (`checkWeightFloor(...) ||
  validDayCount < HARD_FLOOR_MIN_VALID_DAYS`), leaving `checkHardFloor`'s own signature and result
  unchanged. `LoggingGapCard.tsx` computes the EMA/regression/standard-error trend whenever
  `checkWeightFloor` alone is false, and computes the gap from that same trend only when the full
  `checkHardFloor` is also false.
- `frontend/lib/sustainability.ts`'s `evaluateSustainability` takes the trend, the gap result, Mean
  Logged Intake and BMR as independent inputs and applies each check's own gate: `loss_too_fast`
  fires on a conservative `slope + se` bound clearing 1%/week, with no reference to the gap;
  `intake_below_bmr` fires only when `gap.kind === 'on_track'` and the shortfall clears a 5% margin
  (accounting for the photo-estimation bias Mean Logged Intake already carries).
- The middle row's precedence, settled for the changes that follow this one (the Healthiness Label,
  the LLM advice lines): this warning renders unconditionally when either check fires, outranking
  the label; whatever renders the label must do so only when `evaluateSustainability` returns `[]`.

### Consequences

- A user who logs sparsely but weighs in daily now gets the rate-of-loss warning even though the
  card's gap line simultaneously reports `not_enough_data` — the two rows can disagree on what they
  have enough data for, and that disagreement is intentional rather than a bug to reconcile.
- The intake check inherits every one of the full hard floor's requirements transitively rather than
  restating them, so a future change to `checkHardFloor`'s food-log condition changes the intake
  check's behavior too, silently. A reader auditing the intake check's gate needs to know this rather
  than finding a single self-contained condition.
- ADR-004 (heuristic Food Healthiness Label) stays `Proposed`: nothing it decides ships in this
  change, and this ADR only records the precedence slot the label will occupy once it does.
