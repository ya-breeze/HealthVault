// Pure computation library for the Logging Gap card (logging-gap capability) — see
// docs/specs/logging-gap.md's "How" section for the full rationale behind each rule below. Kept
// framework-free (no fetch, no React) so every step is unit-testable on its own; the card
// component (task 5) owns fetching the four inputs and wiring these functions together in the
// order the spec's "Silence rule and hard floor" requires.

import { toDayOffset } from './dataTypeMeta';
import { loggedDayKey } from './loggedDay';
import type { DayCompletenessState } from './api';

const KCAL_PER_KG = 7700;
const MAX_RATE_KG_PER_DAY = 2.0;

export const LOGGING_GAP_WINDOW_DAYS = 28;
// Mirrors weight Trend Projection's own lead-in behavior (design.md decision 4): the alpha=0.25
// EMA needs room to converge before the window's first day matters.
export const LOGGING_GAP_LEAD_IN_DAYS = 30;

const HARD_FLOOR_MIN_RAW_SURVIVORS = 2;
const HARD_FLOOR_MAX_REJECTIONS = 3;
const HARD_FLOOR_MIN_VALID_DAYS = 3;
const HARD_FLOOR_FRESHNESS_DAYS = 7;

const FORMULA_ERROR_RATE = 0.10;

export interface DayValueRecord {
  day: number;
  value: number;
}

export interface OutlierRejectionResult {
  kept: DayValueRecord[];
  rejected: DayValueRecord[];
  bootstrapSiblingAmbiguous: boolean;
}

/**
 * Rate-based outlier rejection over raw, un-bucketed weigh-ins (design.md decision 2). Walks
 * `records` in chronological order, rejecting a candidate whose implied rate of change from the
 * last *kept* record exceeds 2.0 kg/day. A same-day candidate is never rate-checked and never
 * becomes the reference for later comparisons. Before the main walk starts, the initial `lastKept`
 * candidate is itself validated against the first later record on a different calendar day,
 * dropping the earlier of a disagreeing pair and repeating until two agree or one remains — this
 * is the "bootstrap" step, and `bootstrapSiblingAmbiguous` reports whether it ever rejected a
 * candidate that had a same-day-exempted sibling (the same-day-sibling suppression rule).
 */
export function rejectOutliers(records: DayValueRecord[]): OutlierRejectionResult {
  if (records.length === 0) {
    return { kept: [], rejected: [], bootstrapSiblingAmbiguous: false };
  }

  const sorted = [...records].sort((a, b) => a.day - b.day);

  // Bootstrap: validate the initial lastKept candidate.
  let candidateIdx = 0;
  let candidateSiblingCount = 0;
  let bootstrapSiblingAmbiguous = false;
  const bootstrapRejectedIndices: number[] = [];
  const bootstrapKeptSiblingIndices: number[] = [];
  let nextIdx = 1;
  let resumeIdx = sorted.length;

  while (nextIdx < sorted.length) {
    const rec = sorted[nextIdx];
    const cand = sorted[candidateIdx];
    if (rec.day === cand.day) {
      bootstrapKeptSiblingIndices.push(nextIdx);
      candidateSiblingCount++;
      nextIdx++;
      continue;
    }
    const rate = Math.abs(rec.value - cand.value) / (rec.day - cand.day);
    if (rate > MAX_RATE_KG_PER_DAY) {
      bootstrapRejectedIndices.push(candidateIdx);
      if (candidateSiblingCount > 0) bootstrapSiblingAmbiguous = true;
      candidateIdx = nextIdx;
      candidateSiblingCount = 0;
      nextIdx++;
    } else {
      resumeIdx = nextIdx;
      break;
    }
  }
  const anchorIdx = candidateIdx;

  // Main walk: standard rate-check against lastKept, starting from wherever the bootstrap left off.
  const keptIndices = new Set<number>([anchorIdx, ...bootstrapKeptSiblingIndices]);
  const rejectedIndices = new Set<number>(bootstrapRejectedIndices);
  let lastKeptIdx = anchorIdx;
  for (let idx = resumeIdx; idx < sorted.length; idx++) {
    const rec = sorted[idx];
    const lastKept = sorted[lastKeptIdx];
    if (rec.day === lastKept.day) {
      keptIndices.add(idx);
      continue;
    }
    const rate = Math.abs(rec.value - lastKept.value) / (rec.day - lastKept.day);
    if (rate > MAX_RATE_KG_PER_DAY) {
      rejectedIndices.add(idx);
    } else {
      keptIndices.add(idx);
      lastKeptIdx = idx;
    }
  }

  return {
    kept: sorted.filter((_, i) => keptIndices.has(i)),
    rejected: sorted.filter((_, i) => rejectedIndices.has(i)),
    bootstrapSiblingAmbiguous,
  };
}

/**
 * Day-buckets outlier-surviving raw weigh-ins by averaging same-day values — design.md decision 4's
 * "outlier-filtered, day-bucketed weight series". Run this only once the rejection walk has
 * finished, never partway through it: `rejectOutliers` rate-checks *raw* records and exempts
 * same-day siblings by design, so bucketing first would silently change which records it rejects.
 *
 * Output is sorted by day and has exactly one entry per distinct day, which is what makes the
 * same-day-sibling exemption safe downstream — a day with five weigh-ins contributes one point to
 * the EMA, not five.
 */
export function bucketByDay(records: DayValueRecord[]): DayValueRecord[] {
  const buckets = new Map<number, number[]>();
  for (const r of records) {
    const vals = buckets.get(r.day);
    if (vals) vals.push(r.value);
    else buckets.set(r.day, [r.value]);
  }
  return [...buckets.entries()]
    .sort(([a], [b]) => a - b)
    .map(([day, vals]) => ({ day, value: vals.reduce((a, v) => a + v, 0) / vals.length }));
}

/**
 * Standard OLS slope standard error from the regression's own residuals — design.md decision 3.
 * `null` when fewer than 3 distinct x-values are given: with n-2 < 1 the standard error is
 * genuinely undefined, not zero (a 2-point fit is a perfect line with zero degrees of freedom).
 *
 * Known and accepted limitation: the `y` values passed here are EMA-smoothed (decision 4), not raw
 * weigh-ins, and OLS assumes independent residuals. The alpha=0.25 EMA both shrinks residual
 * variance and autocorrelates what is left, so this standard error is systematically *optimistic* —
 * the resulting interval is narrower than a textbook reading of decision 3 would suggest, and the
 * "stay silent when the interval covers zero" rule therefore fires slightly less often than it
 * would on raw residuals. It is left as-is because the fixed 10%-of-target term (~250 kcal at a
 * typical target) dominates the quadrature for any reasonably dense series, so correcting it would
 * change the interval by far less than the term it sits beside. Anything that makes the trend term
 * dominant again — a longer window, a smaller formula error — should revisit this first.
 */
export function slopeStandardError(
  points: { x: number; y: number }[],
  slope: number,
  intercept: number
): number | null {
  const distinctXs = new Set(points.map(p => p.x));
  if (distinctXs.size < 3) return null;

  const n = points.length;
  const sumResidualsSq = points.reduce((acc, p) => {
    const residual = p.y - (intercept + slope * p.x);
    return acc + residual * residual;
  }, 0);
  const meanX = points.reduce((a, p) => a + p.x, 0) / n;
  const sumSqDevX = points.reduce((a, p) => a + (p.x - meanX) * (p.x - meanX), 0);

  return Math.sqrt(sumResidualsSq / (n - 2)) / Math.sqrt(sumSqDevX);
}

function parseISODate(s: string): Date {
  return new Date(`${s}T00:00:00Z`);
}

function formatISODate(d: Date): string {
  return d.toISOString().slice(0, 10);
}

function addDaysISO(s: string, days: number): string {
  const d = parseISODate(s);
  d.setUTCDate(d.getUTCDate() + days);
  return formatISODate(d);
}

export interface LoggingGapWindow {
  /** Inclusive start of the visible 28-day window, YYYY-MM-DD. */
  windowStart: string;
  /** Inclusive end of the visible 28-day window (yesterday in the caller's Logged Day), YYYY-MM-DD. */
  windowEnd: string;
  /** Wider start date to fetch weight from, so the EMA has converged before windowStart. */
  leadInStart: string;
  /**
   * UTC instant to pass as the weight fetch's `from`: one day earlier than `leadInStart`'s literal
   * UTC midnight, since `leadInStart` is a Logged Day (caller-local calendar date) and a positive
   * UTC offset's local midnight falls on the *previous* UTC day. Over-fetching by a day is safe —
   * the caller re-derives each record's Logged Day from its real timestamp and `leadInStartDayOffset`
   * trims anything still earlier than the lead-in, so this only ever widens, never narrows, the
   * true range.
   */
  leadInFetchFromUTC: string;
  windowStartDayOffset: number;
  windowLastDayOffset: number;
  leadInStartDayOffset: number;
}

/**
 * Derives the 28-day Logging Gap window (ending yesterday in the caller's Logged Day, per
 * design.md decision 1 — never the last weigh-in) and the wider lead-in range to fetch weight
 * over, so this boundary arithmetic is unit-testable independent of the card component.
 */
export function resolveLoggingGapWindow(now: Date, timezone: string | undefined): LoggingGapWindow {
  const today = loggedDayKey(now, timezone);
  const windowEnd = addDaysISO(today, -1);
  const windowStart = addDaysISO(windowEnd, -(LOGGING_GAP_WINDOW_DAYS - 1));
  const leadInStart = addDaysISO(windowStart, -LOGGING_GAP_LEAD_IN_DAYS);
  return {
    windowStart,
    windowEnd,
    leadInStart,
    leadInFetchFromUTC: `${addDaysISO(leadInStart, -1)}T00:00:00.000Z`,
    windowStartDayOffset: toDayOffset(windowStart),
    windowLastDayOffset: toDayOffset(windowEnd),
    leadInStartDayOffset: toDayOffset(leadInStart),
  };
}

export interface DayWindowData {
  state: DayCompletenessState;
  calories: number;
  /**
   * Count of that day's meals in a status other than `confirmed` (the
   * daily-totals endpoint's `unconfirmed_meals`) — see `isValidDay`.
   */
  unconfirmedMeals: number;
}

/**
 * Whether a day may be averaged into Mean Logged Intake. Two independent
 * conditions, and both matter:
 *
 * - Day Completeness says the day was fully logged (Complete or Confirmed
 *   Complete — `food-day-completeness`'s own gate, read as-is).
 * - Every one of that day's meals actually reached `confirmed`, so its
 *   calorie total is the whole day rather than part of it.
 *
 * The second is not implied by the first: Day Completeness counts Eating
 * Occasions across *every* meal status, while the calorie total sums only
 * `confirmed` rows. A day whose meals were photographed but whose vision
 * calls failed (or were never confirmed) is therefore Complete with a total
 * far below what was eaten — often 0. Averaging that in doesn't merely add
 * noise, it drags Mean Logged Intake down and manufactures a confident
 * Logging Gap out of nothing, with a perfectly healthy weight trend so
 * neither the interval nor the hard floor can catch it.
 */
function isValidDay(day: DayWindowData): boolean {
  if (day.state !== 'complete' && day.state !== 'confirmed_complete') return false;
  return day.unconfirmedMeals === 0;
}

/**
 * The days inside `[windowStartDayOffset, windowLastDayOffset]` that may be averaged into Mean
 * Logged Intake, per `isValidDay`. Shared by `checkHardFloor` (which counts them against the floor
 * of 3) and `computeLoggingGap` (which averages them), so the set the floor vouches for is exactly
 * the set the mean is taken over — the two used to filter `perDayWindowData` differently.
 */
function validDaysInWindow(
  perDayWindowData: Record<number, DayWindowData>,
  windowStartDayOffset: number,
  windowLastDayOffset: number
): DayWindowData[] {
  return Object.entries(perDayWindowData)
    .filter(([dayOffsetKey, day]) => {
      const dayOffset = Number(dayOffsetKey);
      return dayOffset >= windowStartDayOffset && dayOffset <= windowLastDayOffset && isValidDay(day);
    })
    .map(([, day]) => day);
}

/**
 * The weight-only half of the hard floor (design.md decision 5): fewer than two raw survivors,
 * more than three rejections, bootstrap sibling ambiguity, or a last weigh-in older than the
 * freshness window. Split out from `checkHardFloor` because the sustainability rate-of-loss check
 * (`sustainability.ts`'s `loss_too_fast`) depends on weight alone and must not be silenced by a
 * sparse food log — `checkHardFloor`'s remaining condition, the valid-food-day count, has nothing
 * to do with whether the weight trend itself is trustworthy, and gating the rate check behind it
 * would silence the user most likely to be losing too fast unnoticed: one who weighs in daily but
 * logs food only a few days out of twenty-eight.
 */
export function checkWeightFloor(
  kept: DayValueRecord[],
  rejected: DayValueRecord[],
  bootstrapSiblingAmbiguous: boolean,
  mostRecentKeptDayOffset: number,
  windowLastDayOffset: number
): boolean {
  if (kept.length < HARD_FLOOR_MIN_RAW_SURVIVORS) return true;
  if (rejected.length > HARD_FLOOR_MAX_REJECTIONS) return true;
  if (bootstrapSiblingAmbiguous) return true;
  if (windowLastDayOffset - mostRecentKeptDayOffset > HARD_FLOOR_FRESHNESS_DAYS) return true;
  return false;
}

/**
 * Hard floor (design.md decision 5) — evaluated before any EMA/regression/SE is ever computed, so
 * the too-few-points cases below can never reach a regression attempt. `kept`/`rejected` must
 * already be filtered by the caller to the visible window (via `windowStartDayOffset`);
 * `bootstrapSiblingAmbiguous` must NOT be filtered — it's a structural fact about the whole
 * lead-in-extended rejection walk, not scoped to the visible window (see rejectOutliers' own doc
 * comment and design.md decision 2's same-day-sibling suppression rule).
 */
export function checkHardFloor(
  kept: DayValueRecord[],
  rejected: DayValueRecord[],
  bootstrapSiblingAmbiguous: boolean,
  windowStartDayOffset: number,
  perDayWindowData: Record<number, DayWindowData>,
  mostRecentKeptDayOffset: number,
  windowLastDayOffset: number
): boolean {
  if (checkWeightFloor(kept, rejected, bootstrapSiblingAmbiguous, mostRecentKeptDayOffset, windowLastDayOffset)) {
    return true;
  }

  const validDayCount = validDaysInWindow(perDayWindowData, windowStartDayOffset, windowLastDayOffset).length;
  return validDayCount < HARD_FLOOR_MIN_VALID_DAYS;
}

/**
 * The three answers the card can give, and they are genuinely three:
 *
 * - `gap` — a difference large enough to stand outside its own uncertainty.
 * - `on_track` — a difference *smaller* than its uncertainty. Not an absence
 *   of data: the weight trend and the food log were both measured and they
 *   agree, to the limit of what this estimate can resolve. Says nothing about
 *   whether weight is moving toward the goal or whether intake is sensible —
 *   the comparison below tests neither — so copy for this state must claim
 *   only that the log matches the scale.
 * - `not_enough_data` — the computation could not produce a number worth
 *   comparing at all.
 */
export type LoggingGapResult =
  | { kind: 'gap'; value: number; interval: number }
  | { kind: 'on_track' }
  | { kind: 'not_enough_data' };

/**
 * Mean Logged Intake over the valid days inside `[windowStartDayOffset, windowLastDayOffset]`
 * (`validDaysInWindow`, per `isValidDay`) — exported so the sustainability check
 * (`sustainability.ts`'s `intake_below_bmr`) reads the exact same mean `computeLoggingGap` computed
 * its own comparison from, rather than a second average that could drift from it. `null` when no
 * valid day survives, the same case `computeLoggingGap` turns into `not_enough_data`.
 */
export function meanLoggedIntake(
  perDayWindowData: Record<number, DayWindowData>,
  windowStartDayOffset: number,
  windowLastDayOffset: number
): number | null {
  const validDays = validDaysInWindow(perDayWindowData, windowStartDayOffset, windowLastDayOffset);
  if (validDays.length === 0) return null;
  return validDays.reduce((acc, d) => acc + d.calories, 0) / validDays.length;
}

/**
 * Implied Intake, Mean Logged Intake, the Logging Gap value and its interval (design.md decisions
 * 3 and "Silence rule and hard floor") — called only once `checkHardFloor` has already returned
 * `false`, so a regression exists. `se: null` (fewer than 3 distinct EMA days) is treated as an
 * unbounded `trendErrorKcal`, so the interval is infinite and the finiteness guard below returns
 * `not_enough_data` for it — deliberately, and deliberately *before* the interval comparison,
 * which would otherwise read an unmeasurable series as `on_track`.
 *
 * Distinguishing `on_track` from `not_enough_data` is the whole reason this function has three
 * outcomes rather than two. Both used to return `not_enough_data`, which meant the card said "not
 * enough data yet" to a user whose weight trend and food log had been measured for four weeks and
 * agreed — denying the logging that produced the agreement. Where the two are told apart:
 * everything that leaves us with no number to compare is `not_enough_data`; only a real
 * comparison, resolved in favour of "these agree", is `on_track`.
 *
 * Takes the same window bounds `checkHardFloor` does, and scopes `perDayWindowData` with them
 * itself rather than trusting the caller to have pre-filtered — the two functions are exported
 * side by side and must not disagree about which days count. The empty-`validDays` guard is
 * likewise its own: the hard floor already rejects fewer than 3 valid days, but relying on that
 * makes this function's output depend on a call ordering its signature doesn't express, and the
 * failure mode is silent — an empty average is `NaN`, `Math.abs(NaN) <= interval` is `false`, so a
 * `{kind: 'gap'}` carrying `NaN` would render as "NaN–NaN kcal/day".
 *
 * That escape route has more than one entrance, so the final `Number.isFinite(value)` check closes
 * it for all of them rather than only for the empty average: a non-finite `regression.slope` or a
 * missing/non-numeric `nutritionTargetCalories` poisons `impliedIntake` the same way and reaches
 * the same comparison, which lets non-numbers through because every comparison against `NaN` is
 * `false`. `not_enough_data` is the right answer to any of them — there is no gap we can honestly
 * report, and equally no agreement we can honestly claim.
 */
export function computeLoggingGap(
  regression: { slope: number; intercept: number },
  se: number | null,
  nutritionTargetCalories: number,
  perDayWindowData: Record<number, DayWindowData>,
  windowStartDayOffset: number,
  windowLastDayOffset: number
): LoggingGapResult {
  const impliedIntake = nutritionTargetCalories + regression.slope * KCAL_PER_KG;
  const meanIntake = meanLoggedIntake(perDayWindowData, windowStartDayOffset, windowLastDayOffset);
  if (meanIntake === null) return { kind: 'not_enough_data' };
  const value = impliedIntake - meanIntake;

  const formulaError = FORMULA_ERROR_RATE * nutritionTargetCalories;
  const trendErrorKcal = se === null ? Infinity : KCAL_PER_KG * se;
  const interval = Math.sqrt(formulaError * formulaError + trendErrorKcal * trendErrorKcal);

  if (!Number.isFinite(value) || !Number.isFinite(interval)) {
    return { kind: 'not_enough_data' };
  }
  // Order matters: this check must stay *after* the finiteness guard above.
  // `se === null` (fewer than 3 distinct EMA days) makes `interval` infinite,
  // and `Math.abs(value) <= Infinity` is `true` — so a series too sparse to
  // measure at all would report as "on track", which is the one wrong answer
  // this state must never give. The guard above catches it first.
  if (Math.abs(value) <= interval) {
    return { kind: 'on_track' };
  }
  return { kind: 'gap', value, interval };
}

/**
 * Count of raw weigh-ins excluded by outlier rejection within the 28-day window itself — task
 * 3.9, matching the same window-scoped count `checkHardFloor`'s rejection cap uses, so the card can
 * render the outlier note regardless of which content state is showing. `rejected` here is the
 * full (lead-in-extended, unfiltered) array from `rejectOutliers` — this function does the window
 * filtering itself rather than requiring the caller to pre-filter, matching `rejectOutliers`' own
 * doc comment about how callers should scope its output to a window.
 */
export function excludedOutlierCount(
  rejected: DayValueRecord[],
  windowStartDayOffset: number,
  windowLastDayOffset: number
): number {
  return rejected.filter(r => r.day >= windowStartDayOffset && r.day <= windowLastDayOffset).length;
}
