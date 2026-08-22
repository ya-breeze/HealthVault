import type { DataType } from './api';

export type AggFamily = 'cumulative' | 'point';

export interface TypeMeta {
  family: AggFamily;
  /** Display decimal precision — see formatMetricValue. */
  decimals: number;
}

/**
 * Mirrors backend/pkg/server/api.go's typeRegistry family assignment, for
 * the frontend to know how to render each type's chart. food_meal is
 * intentionally absent — it never accepts ?bucket= (see data-api spec) and
 * has no data page of its own in this UI.
 *
 * `decimals` is the single source of truth for display precision, consumed
 * by formatMetricValue below (chart axis/tooltip, vitals-grid cards, and the
 * stats row all read it instead of each hardcoding their own toFixed/round).
 * Carried over unchanged from the precision vitals.ts already used for its 8
 * metrics; every other type is a fresh judgment call sanity-checked against
 * realistic values for that unit (see chart-value-rounding/design.md), not a
 * blanket default:
 *   - kilograms/percent/ml-per-kg-per-min/m-per-s quantities (weight-like,
 *     body_fat, vo2_max, speed, lean_body_mass, bone_mass) and liters
 *     (hydration) get 1 decimal — a whole number would hide meaningful
 *     variation (e.g. 72.4kg, 18.5%, 3.2 m/s).
 *   - temperatures and blood_glucose (mmol/L) get 1 decimal, matching how
 *     these are conventionally displayed (36.6°C, 5.6 mmol/L).
 *   - height (meters) gets 2 decimals — 0 or 1 decimal is too coarse for a
 *     quantity normally written as e.g. 1.75.
 *   - counts/rates/kcal/mmHg (steps, heart_rate, hrv, oxygen_saturation,
 *     blood_pressure, calories, nutrition grams/kcal, respiratory_rate,
 *     resting_heart_rate, basal_metabolic_rate) get 0 decimals — already
 *     whole numbers in practice, matches the existing Math.round
 *     conventions elsewhere (vitals.ts, MacroSummary.tsx).
 *   - exercise (duration_seconds, no existing display-unit convention
 *     anywhere in this app) is shown in its raw storage unit — seconds —
 *     same as before this change, at 0 decimals; unlike distance/sleep
 *     below, there is no established "minutes" display elsewhere to match,
 *     so no unit conversion is applied (see toDisplayUnit).
 */
export const TYPE_META: Partial<Record<DataType, TypeMeta>> = {
  steps: { family: 'cumulative', decimals: 0 },
  distance: { family: 'cumulative', decimals: 1 },
  active_calories: { family: 'cumulative', decimals: 0 },
  total_calories: { family: 'cumulative', decimals: 0 },
  hydration: { family: 'cumulative', decimals: 1 },
  exercise: { family: 'cumulative', decimals: 0 },
  sleep: { family: 'cumulative', decimals: 1 },
  nutrition: { family: 'cumulative', decimals: 0 },
  heart_rate: { family: 'point', decimals: 0 },
  heart_rate_variability: { family: 'point', decimals: 0 },
  weight: { family: 'point', decimals: 1 },
  weight_goal: { family: 'point', decimals: 1 },
  height: { family: 'point', decimals: 2 },
  blood_pressure: { family: 'point', decimals: 0 },
  blood_glucose: { family: 'point', decimals: 1 },
  oxygen_saturation: { family: 'point', decimals: 0 },
  body_temperature: { family: 'point', decimals: 1 },
  skin_temperature: { family: 'point', decimals: 1 },
  respiratory_rate: { family: 'point', decimals: 0 },
  resting_heart_rate: { family: 'point', decimals: 0 },
  basal_metabolic_rate: { family: 'point', decimals: 0 },
  body_fat: { family: 'point', decimals: 1 },
  lean_body_mass: { family: 'point', decimals: 1 },
  vo2_max: { family: 'point', decimals: 1 },
  bone_mass: { family: 'point', decimals: 1 },
  speed: { family: 'point', decimals: 1 },
};

/**
 * Converts a raw API value (in typeRegistry's storage unit — see
 * backend/pkg/server/api.go's typeRegistry valueCol comments, e.g. distance
 * is stored/returned in meters, sleep in seconds) to the unit this type is
 * actually displayed in elsewhere in the app. Most types display in their
 * storage unit already (kg, %, bpm, ...) and pass through unchanged; only
 * distance and sleep have an established display unit that differs from
 * storage. Every caller that renders a distance/sleep value — the
 * vitals-grid card (vitals.ts) and every chart element (DataTypeClient.tsx)
 * — MUST route raw API values through this before formatMetricValue, or the
 * two ends of the app silently disagree on units, not just decimal places.
 */
export function toDisplayUnit(type: DataType, raw: number): number {
  switch (type) {
    case 'distance': return raw / 1000; // meters -> km
    case 'sleep': return raw / 3600; // seconds -> hours
    default: return raw;
  }
}

/**
 * Formats a raw metric value to its type's established display precision.
 * Uses toLocaleString (not toFixed) with a fixed fraction-digit count so
 * large values still get thousands grouping (e.g. steps "12,345") while
 * matching toFixed's rounding for everything else — this is what lets the
 * vitals-grid refactor (see extractVital in vitals.ts) reuse this helper
 * without changing steps' previously-hardcoded `.toLocaleString()` output.
 * Pinned to the 'en-US' locale rather than the browser's — design.md's
 * "Locale-aware number formatting ... out of scope, matches current
 * behavior" non-goal means this must not introduce a locale-dependent
 * decimal separator (e.g. ',' instead of '.') for the many types that
 * previously used a fixed-locale toFixed(1); a code-review pass caught this
 * regressing on the initial `toLocaleString(undefined, ...)` draft.
 * Callers are responsible for passing an already-display-unit value (see
 * toDisplayUnit above) — this function only rounds, it never converts units.
 */
export function formatMetricValue(type: DataType, value: number): string {
  const decimals = TYPE_META[type]?.decimals ?? 0;
  return value.toLocaleString('en-US', { minimumFractionDigits: decimals, maximumFractionDigits: decimals });
}

export const NUTRITION_MACROS = [
  { key: 'calories', label: 'Calories' },
  { key: 'protein_grams', label: 'Protein' },
  { key: 'carbs_grams', label: 'Carbs' },
  { key: 'fat_grams', label: 'Fat' },
  { key: 'sugar_grams', label: 'Sugar' },
  { key: 'sodium_grams', label: 'Sodium' },
  { key: 'dietary_fiber_grams', label: 'Fiber' },
] as const;

/**
 * Y-axis domain for a point-in-time chart, computed from the values actually
 * driving it rather than defaulting toward zero — see chart-zoom-aggregation's
 * "Point-in-time Y-axis domain" requirement. Pads by 10% of the data range, or
 * by 2% of the data's own magnitude when the range is zero (flat/single-point
 * data), so a flat series still renders a visible band. Returns undefined when
 * there's no data to derive a range from, leaving the axis at its default.
 */
export function computeYDomain(values: number[]): [number, number] | undefined {
  if (values.length === 0) return undefined;
  const dataMin = Math.min(...values);
  const dataMax = Math.max(...values);
  const range = dataMax - dataMin;
  const pad = range > 0 ? range * 0.1 : Math.max(Math.abs(dataMax), 1) * 0.02;
  return [dataMin - pad, dataMax + pad];
}

/**
 * Exponential moving average, alpha = 0.25 (~7-period EMA) — see
 * chart-zoom-aggregation's "Weight trend line" requirement. Seeded from the
 * first value so the trend starts exactly where the data starts.
 */
export function emaSeries(values: number[], alpha = 0.25): number[] {
  if (values.length === 0) return [];
  const result: number[] = [values[0]];
  for (let i = 1; i < values.length; i++) {
    result.push(result[i - 1] + alpha * (values[i] - result[i - 1]));
  }
  return result;
}

/**
 * WHO BMI category band edges, in BMI units (kg/m²) — see
 * goal-weight-bmi-bands-trend-projection's design.md "BMI bands and
 * readout". Shared by bmiBandEdgesKg (chart bands) and classifyBmi (readout
 * category), so the two can never disagree at a boundary value.
 */
export const BMI_BAND_EDGES = [18.5, 25, 30] as const;

/**
 * Converts the WHO BMI band edges to kg for a given height, so the weight
 * chart (a kg Y-axis) can render them as ReferenceAreas without converting
 * plotted weight data to BMI. `bmi = kg / heightMeters^2`, so
 * `kg = bmi * heightMeters^2`.
 */
export function bmiBandEdgesKg(heightMeters: number): number[] {
  return BMI_BAND_EDGES.map(bmi => bmi * heightMeters * heightMeters);
}

export type BmiCategory = 'Underweight' | 'Normal' | 'Overweight' | 'Obese';

/**
 * WHO BMI category lookup with lower-inclusive boundaries — a BMI of
 * exactly 18.5, 25, or 30 belongs to the higher category, matching
 * bmiBandEdgesKg's band edges so the chart bands and this readout never
 * disagree at a boundary value.
 */
export function classifyBmi(bmi: number): BmiCategory {
  if (bmi < BMI_BAND_EDGES[0]) return 'Underweight';
  if (bmi < BMI_BAND_EDGES[1]) return 'Normal';
  if (bmi < BMI_BAND_EDGES[2]) return 'Overweight';
  return 'Obese';
}

/**
 * Shared gate for BMI bands + readout — both are suppressed together when
 * the user has no `height` record on file, rather than each independently
 * checking record presence and risking disagreement. See design.md "Both
 * the bands and the readout are gated on a single condition".
 */
export function hasHeightRecord(heightRecords: unknown[]): boolean {
  return heightRecords.length > 0;
}

const MS_PER_DAY = 86_400_000;

/** See goal-weight-bmi-bands-trend-projection's design.md "Trend projection". */
export const PROJECTION_WINDOW_DAYS = 30;
export const PROJECTION_HORIZON_DAYS = 365;
export const PROJECTION_MIN_RECORDS = 5;
export const PROJECTION_MIN_LIFETIME_SPAN_DAYS = 14;
export const PROJECTION_MIN_WINDOW_POINTS = 2;

/**
 * Days since the Unix epoch (UTC, floored) for a date string — the x-axis
 * unit for the trend regression (task 7.1) and for the projection's
 * crossing/horizon math (task 7.4), so a day-offset can be converted back to
 * a Date with simple multiplication rather than calendar-month arithmetic.
 */
export function toDayOffset(dateStr: string): number {
  return Math.floor(new Date(dateStr).getTime() / MS_PER_DAY);
}

/**
 * Ordinary least-squares fit over arbitrary (x, y) pairs — used for the
 * trend projection's (day_offset, ema_value) regression (task 7.1), kept
 * generic/pure so it needs no chart or date context to unit test.
 */
export function linearRegression(points: { x: number; y: number }[]): { slope: number; intercept: number } {
  const n = points.length;
  if (n === 0) return { slope: 0, intercept: 0 };
  const sumX = points.reduce((a, p) => a + p.x, 0);
  const sumY = points.reduce((a, p) => a + p.y, 0);
  const sumXY = points.reduce((a, p) => a + p.x * p.y, 0);
  const sumXX = points.reduce((a, p) => a + p.x * p.x, 0);
  const denom = n * sumXX - sumX * sumX;
  const slope = denom === 0 ? 0 : (n * sumXY - sumX * sumY) / denom;
  const intercept = (sumY - slope * sumX) / n;
  return { slope, intercept };
}

/**
 * Selects the last 30 calendar days (inclusive of the series' own last date,
 * regardless of the currently active zoom) of an EMA series paired with its
 * source dates — task 7.2. `dates` and `emaValues` must be the same length
 * and in ascending date order (i.e. dates[i] produced emaValues[i], as
 * emaSeries already preserves index alignment with its input).
 */
export function last30DayEmaWindow(dates: string[], emaValues: number[]): { date: string; ema: number }[] {
  if (dates.length === 0) return [];
  const lastMs = new Date(dates[dates.length - 1]).getTime();
  const cutoffMs = lastMs - (PROJECTION_WINDOW_DAYS - 1) * MS_PER_DAY;
  const result: { date: string; ema: number }[] = [];
  for (let i = 0; i < dates.length; i++) {
    if (new Date(dates[i]).getTime() >= cutoffMs) {
      result.push({ date: dates[i], ema: emaValues[i] });
    }
  }
  return result;
}

/**
 * "Not enough data to project yet" gate (task 7.3) — a lifetime-history
 * check (total records, earliest-to-latest span) independent of the 30-day
 * regression window itself, which can separately fail to have 2+ points
 * (e.g. an inactive user with old data but nothing recent). Either
 * insufficiency alone is enough to gate the projection off.
 */
export function hasEnoughDataForProjection(
  totalRecords: number,
  lifetimeSpanDays: number,
  windowPointCount: number
): boolean {
  return totalRecords >= PROJECTION_MIN_RECORDS
    && lifetimeSpanDays >= PROJECTION_MIN_LIFETIME_SPAN_DAYS
    && windowPointCount >= PROJECTION_MIN_WINDOW_POINTS;
}

export type ProjectionStatus = 'reached' | 'not-on-track' | 'on-track';

export interface ProjectionResult {
  status: ProjectionStatus;
  /** Set only when status is 'on-track' — see toDayOffset for the unit. */
  crossingDayOffset?: number;
}

/**
 * Given a regression fit over the 30-day EMA window plus the goal, decides
 * between "already reached" (task 7.4a), "not on track" and "on track"
 * (task 7.4) — in that priority order, per design.md's "Trend projection".
 *
 * `windowStartEma`/`latestEma` are the oldest/newest EMA values inside the
 * 30-day regression window (task 7.2's output), NOT the user's
 * lifetime-earliest weight — using lifetime history here would let an old,
 * no-longer-relevant weight flip the reached/direction determination for a
 * user whose weight has since crossed to the other side of the goal.
 *
 * When `windowStartEma === goal` exactly, direction is undefined (sign of
 * zero): this is only "reached" if `latestEma` is still exactly at goal too;
 * otherwise it falls through to the slope-based on-track/not-on-track check
 * below rather than being misreported as reached.
 */
export function computeProjection(params: {
  slope: number;
  intercept: number;
  goal: number;
  windowStartEma: number;
  latestEma: number;
  lastDayOffset: number;
}): ProjectionResult {
  const { slope, intercept, goal, windowStartEma, latestEma, lastDayOffset } = params;

  const direction = Math.sign(goal - windowStartEma);
  if (direction !== 0) {
    const reached = direction > 0 ? latestEma >= goal : latestEma <= goal;
    if (reached) return { status: 'reached' };
  } else if (latestEma === goal) {
    return { status: 'reached' };
  }

  if (slope === 0) return { status: 'not-on-track' };
  const crossingDayOffset = (goal - intercept) / slope;
  const daysToGoal = crossingDayOffset - lastDayOffset;
  if (daysToGoal <= 0 || daysToGoal > PROJECTION_HORIZON_DAYS) return { status: 'not-on-track' };
  return { status: 'on-track', crossingDayOffset };
}

/**
 * Synthetic future (day_offset, projected kg) points along the regression
 * line — from just after `lastDayOffset` through `endDayOffset` inclusive —
 * evaluated directly from the fit (`intercept + slope * dayOffset`) rather
 * than re-fit, so the dashed continuation is exactly the same line the solid
 * trend already shows. `endDayOffset` is always the projection's own
 * crossing day-offset (computeProjection already rejects anything beyond the
 * 365-day horizon), at either daily or monthly spacing matching the active
 * zoom's own bucket granularity — task 7.4b.
 */
export function projectionPoints(
  intercept: number,
  slope: number,
  lastDayOffset: number,
  endDayOffset: number,
  granularity: 'day' | 'month'
): { dayOffset: number; value: number }[] {
  const stepDays = granularity === 'day' ? 1 : 30;
  const points: { dayOffset: number; value: number }[] = [];
  for (let d = lastDayOffset + stepDays; d < endDayOffset; d += stepDays) {
    points.push({ dayOffset: d, value: intercept + slope * d });
  }
  points.push({ dayOffset: endDayOffset, value: intercept + slope * endDayOffset });
  return points;
}

export type Zoom = 'day' | 'week' | 'month' | 'year';

/** Zoom → time range + bucket, per chart-zoom-aggregation's zoom table. */
export function rangeForZoom(zoom: Zoom): { from: string; to: string; bucket?: 'day' | 'month' } {
  const now = new Date();
  const to = now.toISOString();
  const from = new Date(now);
  switch (zoom) {
    case 'day':
      from.setDate(from.getDate() - 1);
      return { from: from.toISOString(), to };
    case 'week':
      from.setDate(from.getDate() - 7);
      return { from: from.toISOString(), to, bucket: 'day' };
    case 'month':
      from.setDate(from.getDate() - 30);
      return { from: from.toISOString(), to, bucket: 'day' };
    case 'year':
      from.setFullYear(from.getFullYear() - 1);
      return { from: from.toISOString(), to, bucket: 'month' };
  }
}
