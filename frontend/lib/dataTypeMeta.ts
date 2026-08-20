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
