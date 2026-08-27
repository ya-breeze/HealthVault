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
 * Standard OLS slope standard error from the regression's own residuals — design.md decision 3.
 * `null` when fewer than 3 distinct x-values are given: with n-2 < 1 the standard error is
 * genuinely undefined, not zero (a 2-point fit is a perfect line with zero degrees of freedom).
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
  windowStartDayOffset: number;
  windowLastDayOffset: number;
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
    windowStartDayOffset: toDayOffset(windowStart),
    windowLastDayOffset: toDayOffset(windowEnd),
  };
}

export interface DayWindowData {
  state: DayCompletenessState;
  calories: number;
}

function isValidDayState(state: DayCompletenessState): boolean {
  return state === 'complete' || state === 'confirmed_complete';
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
  if (kept.length < HARD_FLOOR_MIN_RAW_SURVIVORS) return true;
  if (rejected.length > HARD_FLOOR_MAX_REJECTIONS) return true;
  if (bootstrapSiblingAmbiguous) return true;

  const validDayCount = Object.entries(perDayWindowData).filter(([dayOffsetKey, day]) => {
    const dayOffset = Number(dayOffsetKey);
    return dayOffset >= windowStartDayOffset && dayOffset <= windowLastDayOffset && isValidDayState(day.state);
  }).length;
  if (validDayCount < HARD_FLOOR_MIN_VALID_DAYS) return true;

  if (windowLastDayOffset - mostRecentKeptDayOffset > HARD_FLOOR_FRESHNESS_DAYS) return true;

  return false;
}

export type LoggingGapResult =
  | { kind: 'gap'; value: number; interval: number }
  | { kind: 'not_enough_data' };

/**
 * Implied Intake, Mean Logged Intake, the Logging Gap value and its interval (design.md decisions
 * 3 and "Silence rule and hard floor") — called only once `checkHardFloor` has already returned
 * `false`, so a regression exists. `se: null` (fewer than 3 distinct EMA days) is treated as an
 * unbounded `trendErrorKcal`, which always suppresses output via the interval check below.
 */
export function computeLoggingGap(
  regression: { slope: number; intercept: number },
  se: number | null,
  nutritionTargetCalories: number,
  perDayWindowData: Record<number, DayWindowData>
): LoggingGapResult {
  const impliedIntake = nutritionTargetCalories + regression.slope * KCAL_PER_KG;
  const validDays = Object.values(perDayWindowData).filter(d => isValidDayState(d.state));
  const meanLoggedIntake = validDays.reduce((acc, d) => acc + d.calories, 0) / validDays.length;
  const value = impliedIntake - meanLoggedIntake;

  const formulaError = FORMULA_ERROR_RATE * nutritionTargetCalories;
  const trendErrorKcal = se === null ? Infinity : KCAL_PER_KG * se;
  const interval = Math.sqrt(formulaError * formulaError + trendErrorKcal * trendErrorKcal);

  if (Math.abs(value) <= interval) {
    return { kind: 'not_enough_data' };
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
