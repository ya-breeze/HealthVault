# Nutrition card middle row: a sustainability warning for under-eating and too-fast weight loss
Idea: ya-breeze/idea-forge#177

## Why

The dashboard's nutrition card (registry id `logging_gap`, titled "Питание" / "Nutrition") has three rows. The first — today's calories and macros against the Nutrition Target — and the third — the logging-gap line — shipped in `docs/specs/nutrition-card-today-and-on-track.md`. The middle row is empty.

The card can already see something it never says. An over-aggressive deficit predicts a relapse, and every number needed to detect one is on screen or one division away:

- **Intake below BMR.** `computeNutritionTarget` (`backend/pkg/server/nutrition_target.go`) computes the Mifflin-St Jeor BMR and immediately multiplies it by the activity multiplier, returning only the product. Mean Logged Intake over the 28-day window is computed inside `computeLoggingGap` (`frontend/lib/loggingGap.ts`) and thrown away after the subtraction. Neither number leaves the function that produces it.
- **Loss faster than the sustainable band.** `LoggingGapCard.tsx` already fits an OLS regression over the EMA-smoothed, day-bucketed weight series and reads its slope in kg/day. Expressed as a percentage of body weight per week, that slope is the whole rate-of-loss check. The usual sustainable band tops out at 1%/week.

The production data that prompted this exercises both branches:

| quantity | value |
|---|---|
| BMR | 1799 kcal/day |
| TDEE (× 1.2, sedentary) | 2159 kcal/day |
| mean logged intake, 28 days | 1617 kcal/day |
| weight trend | −0.585 kg/week on 90.2 kg = 0.65%/week |
| Logging Gap | −102 ± 222 → `on_track` |

The rate check does not fire: 0.65%/week is inside the band. The intake check does: 1617 is 182 kcal below the 1799 BMR. A user in exactly this state gets nothing from the card today.

**The interaction that makes this worth a spec rather than an afternoon.** Logged intake is only trustworthy when the weight trend corroborates it. If the gap line reports 500 kcal/day of unlogged food, then "you are eating below your BMR" is false — the user is *logging* below BMR. Shipping the intake check ungated would fire a starvation warning at everyone who simply does not log everything, which is most users most of the time. The gap line already computes the corroboration and reports it as `on_track`; the intake check has to be gated on it. The rate check needs no such gate, because weight is measured rather than self-reported.

The card's copy standard, set by the shipped `on_track` line, is to say only what was actually measured. That line reads "your log matches your weight" rather than "all good", deliberately, because the computation checks neither goal direction nor whether intake is sane. This change fills exactly that hole, and inherits the standard.

## How

### Precedence in the middle row

The middle row is one slot, and three things want it: this warning, the Healthiness Label, and the LLM advice lines. The owner settled the order — **sustainability warning first, outranking the label; then the label; then the advice.** Neither of the other two exists yet, so this change owns the row unconditionally and leaves the precedence rule written down for the changes that follow it: whatever renders the label renders it only when `evaluateSustainability` returns an empty array.

### Two checks, one pure module

A new `frontend/lib/sustainability.ts` holds the whole decision, framework-free and unit-testable, matching how `frontend/lib/loggingGap.ts` is structured:

```ts
export const MAX_SUSTAINABLE_LOSS_PCT_PER_WEEK = 1.0;
export const BMR_SHORTFALL_MARGIN = 0.05;

export type SustainabilityWarning =
  | { kind: 'loss_too_fast'; percentPerWeek: number }
  | { kind: 'intake_below_bmr'; meanLoggedIntake: number; bmr: number };

export interface SustainabilityInputs {
  gap: LoggingGapResult;
  meanLoggedIntake: number | null;
  bmr: number | null;
  trend: { slope: number; se: number | null; weightAtWindowEnd: number } | null;
}

export function evaluateSustainability(inputs: SustainabilityInputs): SustainabilityWarning[];
```

**`loss_too_fast`** fires when `trend` exists, `trend.se` is not `null`, `weightAtWindowEnd` is finite and positive, the slope is negative, and the loss rate clears the band. The reported rate is the point estimate, `(-slope * 7) / weightAtWindowEnd * 100`. The *firing test* uses the conservative end of a one-standard-error interval instead — `(-(slope + se) * 7) / weightAtWindowEnd * 100 > 1.0` — so a noisy series whose slope only looks steep does not warn. `se === null` (fewer than three distinct EMA days) suppresses the check entirely, the same way it forces `not_enough_data` on the gap line. `slopeStandardError`'s own doc comment records that this standard error is systematically optimistic because the EMA autocorrelates residuals; that makes the guard weaker than a textbook one, not absent, and it is still far better than firing on the bare slope.

`weightAtWindowEnd` is the regression's fitted value at the window's last day, `intercept + slope * windowLastDayOffset`. Using the fitted trend weight rather than the last raw weigh-in keeps numerator and denominator on the same line, so a single noisy morning reading cannot move the percentage.

**`intake_below_bmr`** fires when `gap.kind === 'on_track'`, `meanLoggedIntake` and `bmr` are finite, `bmr > 0`, and `meanLoggedIntake < bmr * (1 - BMR_SHORTFALL_MARGIN)`. The 5% margin exists because Mean Logged Intake carries the photo-estimation bias the card's own `caveatPhoto` hint already warns about; a shortfall of a few kcal is indistinguishable from that bias and is not a finding. On the worked example the margin puts the threshold at 1709 against a mean of 1617, so it fires with room to spare.

The `on_track` gate does more work than it looks like. `on_track` means Implied Intake and Mean Logged Intake agree inside the interval, and Implied Intake is `target + slope * 7700`. A user logging 1617 kcal while their weight holds steady lands on `gap`, not `on_track`, so the gate already excludes the "logs little, loses nothing" case without a separate check for it.

Both warnings can fire together. The returned array is ordered `loss_too_fast` first, because it rests on measured weight while the intake check rests on self-reported food that a gate had to vouch for.

### Getting BMR to the client

`summaryTargetPayload` (`backend/pkg/server/summary_today.go`) carries calories and the three macros and nothing else, and `computeNutritionTarget` discards the BMR after multiplying it. Rather than have the client divide rounded calories by an activity multiplier the summary does not carry either, the backend returns the number it already computed:

- `computeNutritionTarget` gains a `bmr` return value, unrounded like its siblings.
- `nutritionTargetValues` gains `BMR int \`json:"bmr"\``, rounded once at the response boundary through `roundToInt`. This is additive to `GET /api/users/me/nutrition-target`.
- `summaryTargetPayload` gains the same field, set alongside the four existing numbers whenever the target is available. **No `omitempty`**, for the reason already written into that struct's doc comment: `omitempty` drops a zero, and absence must mean "no target", which `available` already says.

The frontend's `TodaySummaryTarget` available branch and `NutritionTarget` gain `bmr: number` to match.

### Reaching the inputs from the card

Two small extractions in `frontend/lib/loggingGap.ts`, both behaviour-preserving:

- **`meanLoggedIntake(perDayWindowData, windowStartDayOffset, windowLastDayOffset): number | null`** is exported and `computeLoggingGap` calls it, so the mean the card reads is byte-for-byte the mean the gap was computed from. `null` when no valid days survive, which is the case `computeLoggingGap` already turns into `not_enough_data`.
- **`checkWeightFloor(kept, rejected, bootstrapSiblingAmbiguous, mostRecentKeptDayOffset, windowLastDayOffset): boolean`** is split out of `checkHardFloor`, carrying the four weight-only conditions: fewer than two raw survivors, more than three rejections, bootstrap sibling ambiguity, and a stale last weigh-in. `checkHardFloor` keeps its exact signature and semantics, becoming `checkWeightFloor(...) || validDayCount < HARD_FLOOR_MIN_VALID_DAYS`.

The split matters because the two checks have different appetites. The rate-of-loss check depends on weight alone, so gating it behind the full hard floor would silence it for a user who weighs in daily but logs food three days out of twenty-eight — the user most likely to be losing too fast unnoticed. The intake check needs the full floor, and gets it transitively: it is gated on `on_track`, which only `computeLoggingGap` can produce, which only runs when the full floor passes.

`LoggingGapCard.tsx`'s effect restructures accordingly: compute the EMA, regression and standard error whenever `checkWeightFloor` is `false`, keep them as the `trend` input, and compute the gap from that same trend only when the full `checkHardFloor` is also `false`. Because `checkHardFloor` false implies `checkWeightFloor` false, the gap line's behaviour is unchanged.

When the three gap-feeding requests fail and the gap line falls back to its row-level `retrieval_error`, there is no trend and no mean, so `evaluateSustainability` returns an empty array and the middle row does not render. The card never warns from a partial fetch.

### Rendering and copy

`ContentState`'s `ready` variant gains `warnings: SustainabilityWarning[]`. The middle row renders between the today row and the gap line, behind its own divider, only when the array is non-empty — an empty row would cost the card vertical space to say nothing, which is the failure the shipped rows were designed away from.

Testids follow the existing names: `nutrition-sustainability` on the block, `nutrition-sustainability-loss-rate` and `nutrition-sustainability-below-bmr` on the lines.

New keys keep the `loggingGap.` prefix, for the reason already recorded: the prefix is internal, and renaming it is deferred to whenever the card fully becomes a nutrition card so the churn happens once. Both dictionaries get:

- `loggingGap.lossTooFast` — "Вы теряете {percent}% веса в неделю — быстрее устойчивого темпа в 1%." / "You are losing {percent}% of body weight a week, faster than the sustainable 1%."
- `loggingGap.intakeBelowBmr` — "В среднем {intake} ккал в день — ниже вашего базового обмена ({bmr} ккал)." / "You average {intake} kcal a day, below your BMR of {bmr} kcal."
- `loggingGap.sustainabilityDetail` — the hint paragraph, shown inside the existing disclosure whenever any warning fires. It names the framing: too steep a deficit is hard to hold, and a relapse is the usual outcome. It says the numbers are estimates from the same trend and the same log the rest of the card uses, and that this is not medical advice.

`{percent}` renders to one decimal place. Both lines state the measured numbers rather than instructing the reader — no calorie prescription, no "eat more", which is the line between adherence framing and medical advice and is where the copy standard puts it.

### ADR

Gating a user-facing assertion on a corroborating signal is a cross-cutting rule this repo has recorded before (ADR-007, day-completeness-gated assertion). This change adds `docs/adr/ADR-012-sustainability-warning-gated-on-logging-gap.md`, created `Proposed` and flipped to `Accepted` as the last commit of the PR: the intake check is gated on `on_track`, the rate check is not, and the middle row's precedence puts this warning above the not-yet-built Healthiness Label. ADR-004 stays `Proposed` — nothing it decides ships here.

### Deliberately excluded

- **The Healthiness Label, the LLM advice lines, and the nutrition chat.** They are the other three parts of the idea's own four-way split, and the owner asked for this one first precisely because it settles the row's precedence rule for them.
- **Any calorie prescription.** The warning reports two measurements and stops. Telling a user what to eat is medical advice, and no part of this computation is qualified to give it.
- **A configurable band.** 1%/week and the 5% BMR margin are constants in `sustainability.ts`. A settings screen for them would be a second feature serving one user.
- **Reworking `slopeStandardError`'s optimism.** It is documented and unchanged; the constant 10%-of-target term still dominates the gap's interval, and the rate check's use of it is a guard rather than a claim.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e`

## Ground rules
This spec is implemented by an automated pass running unattended. **There is no approval step and nothing is waiting for one** — do not look for a tick, a marker, or a sign-off anywhere, and do not wait for one.

Tick the boxes in this file as the work is completed; they are the record of progress, and the pipeline reads them to decide whether the change is finished.

Out of scope, deliberately: do NOT mark the pull request ready for review and do NOT merge it. Those are the pipeline's own final steps, run once the task list is complete. The operator reviews the pull request and merges it themselves; that is the only gate this work passes through, so leave it in a state worth reading.

### Task 1: Return BMR from the backend
- [x] Add a `bmr` return value to `computeNutritionTarget` in `backend/pkg/server/nutrition_target.go`, unrounded like the other four, and update its doc comment
- [x] Add `BMR int \`json:"bmr"\`` to `nutritionTargetValues`, set from `roundToInt(bmr)` in `computeNutritionTargetForProfile`
- [x] Add the same `BMR int \`json:"bmr"\`` field to `summaryTargetPayload` in `backend/pkg/server/summary_today.go`, set only in the available branch, and extend that struct's existing doc comment so the no-`omitempty` rule covers the new field for the same reason
- [x] Assert in `backend/pkg/server/nutrition_target_test.go` (or the internal test) that `bmr` matches the Mifflin-St Jeor value for a known profile, and that `calories` equals `bmr * activity_multiplier` rounded
- [x] Extend `backend/pkg/server/summary_today_test.go`'s raw-JSON key assertion so `bmr` is present whenever `available` is true, and absent from neither branch's decoding
- [x] Mark completed

### Task 2: Expose Mean Logged Intake and the weight-only floor
- [x] Export `meanLoggedIntake(perDayWindowData, windowStartDayOffset, windowLastDayOffset): number | null` from `frontend/lib/loggingGap.ts`, returning `null` for an empty valid-day set, and have `computeLoggingGap` use it instead of its inline average
- [x] Split `checkWeightFloor(kept, rejected, bootstrapSiblingAmbiguous, mostRecentKeptDayOffset, windowLastDayOffset)` out of `checkHardFloor`, carrying the four weight-only conditions; leave `checkHardFloor`'s signature and result unchanged by composing it as `checkWeightFloor(...) || validDayCount < HARD_FLOOR_MIN_VALID_DAYS`
- [x] Document on `checkWeightFloor` why the two exist separately: the rate-of-loss check depends on weight alone and must not be silenced by a sparse food log
- [x] Add unit tests in `frontend/lib/loggingGap.test.ts` covering `meanLoggedIntake`'s `null` case and window scoping, and covering a fixture where `checkWeightFloor` is `false` while `checkHardFloor` is `true` (enough weigh-ins, fewer than three valid food days)
- [x] Mark completed

### Task 3: The sustainability computation
- [x] Create `frontend/lib/sustainability.ts` with `MAX_SUSTAINABLE_LOSS_PCT_PER_WEEK`, `BMR_SHORTFALL_MARGIN`, the `SustainabilityWarning` union, `SustainabilityInputs`, and `evaluateSustainability`, framework-free like `loggingGap.ts`
- [x] Implement `loss_too_fast`: require a trend with a non-null `se`, a finite positive `weightAtWindowEnd` and a negative slope; fire on the conservative `slope + se` bound clearing the band, and report the point-estimate percentage
- [x] Implement `intake_below_bmr`: require `gap.kind === 'on_track'`, finite `meanLoggedIntake`, finite `bmr > 0`, and a shortfall past `BMR_SHORTFALL_MARGIN`
- [x] Return the warnings ordered `loss_too_fast` first, and document why that order and why the intake check alone is gated
- [x] Add `frontend/lib/sustainability.test.ts` covering: the worked example (rate silent at 0.65%/week, intake firing at 1617 against a 1799 BMR under `on_track`); the same intake numbers with `gap.kind === 'gap'` returning no intake warning; the same with `not_enough_data`; `se === null` suppressing the rate check; a positive slope suppressing it; a slope inside the band suppressing it; a slope past the band whose `slope + se` bound is not suppressing it; a shortfall inside the margin suppressing the intake check; both warnings firing together in order
- [x] Mark completed

### Task 4: Wire the card and render the middle row
- [ ] Add `bmr: number` to `TodaySummaryTarget`'s available branch and to `NutritionTarget` in `frontend/lib/api.ts`
- [ ] Restructure `LoggingGapCard.tsx`'s effect to compute the EMA, regression and standard error whenever `checkWeightFloor` is `false`, and to compute the gap from that same trend only when `checkHardFloor` is also `false`, leaving the gap line's behaviour identical
- [ ] Call `evaluateSustainability` with the gap result, `meanLoggedIntake`, `target.bmr` and the trend, passing `weightAtWindowEnd` as `intercept + slope * windowLastDayOffset`; return an empty array on the gap-only `retrieval_error` path
- [ ] Add `warnings: SustainabilityWarning[]` to `ContentState`'s `ready` variant and render the middle row between the today row and the gap line, behind its own divider, only when the array is non-empty
- [ ] Give the block and its two lines the `nutrition-sustainability`, `nutrition-sustainability-loss-rate` and `nutrition-sustainability-below-bmr` testids, and show `loggingGap.sustainabilityDetail` inside the existing hint disclosure whenever any warning fires
- [ ] Mark completed

### Task 5: Copy in both languages
- [ ] Add `loggingGap.lossTooFast`, `loggingGap.intakeBelowBmr` and `loggingGap.sustainabilityDetail` to `frontend/lib/i18n/ru.ts` and `frontend/lib/i18n/en.ts`, keeping the `loggingGap.` prefix
- [ ] Keep both lines to stated measurements with no calorie prescription, and put the adherence framing plus the "not medical advice" note in the detail key only
- [ ] Render `{percent}` to one decimal place and round `{intake}` and `{bmr}` to whole kcal at the call site, matching how the today row rounds
- [ ] Confirm the two dictionaries have identical key sets
- [ ] Mark completed

### Task 6: End-to-end coverage
- [ ] Add a `bmr` field to `mockLoggingGapApis`'s target fixture in `e2e/tests/logging-gap.spec.ts`, defaulting low enough that every existing fixture keeps its current assertions unchanged
- [ ] Add a too-fast-loss case: a weight series losing about 1.4%/week, asserting `nutrition-sustainability-loss-rate` is visible and carries the percentage
- [ ] Add a below-BMR case: `on_track` numbers whose mean logged intake sits clearly under the mocked `bmr`, with a loss rate inside the band, asserting `nutrition-sustainability-below-bmr` is visible and `nutrition-sustainability-loss-rate` is not
- [ ] Add the gating regression: the same intake and BMR with logged calories low enough that the gap line reports a gap rather than `on_track`, asserting `nutrition-sustainability-below-bmr` is absent
- [ ] Assert the middle row is absent entirely for the existing `on_track` and `not_enough_data` fixtures, and on the gap-only `retrieval_error` path
- [ ] Mark completed

### Task 7: Documentation and validation
- [ ] Write `docs/adr/ADR-012-sustainability-warning-gated-on-logging-gap.md` with `Status: Proposed`, recording the `on_track` gate on the intake check, the absence of a gate on the rate check, and the middle row's precedence order
- [ ] Update `todo.md`'s Phase 4 section so it records the sustainability warning as shipped and names what remains of the middle row
- [ ] Run `make lint` and `make test` and fix everything they report
- [ ] Deploy the branch to the WIP stack and run `make test-e2e` against it, fixing every failure rather than recording it as pre-existing
- [ ] Flip ADR-012 from `Proposed` to `Accepted` as the last commit
- [ ] Mark completed
