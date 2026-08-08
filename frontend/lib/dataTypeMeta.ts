import type { DataType } from './api';

export type AggFamily = 'cumulative' | 'point';

export interface TypeMeta {
  family: AggFamily;
  /** Raw-row column holding the value, for types with exactly one. Absent for
   * blood_pressure (two columns) and nutrition (seven) — those types render
   * their own special-cased chart paths in DataTypeClient. */
  valueCol?: string;
  unit?: string;
}

/**
 * Mirrors backend/pkg/server/api.go's typeRegistry family assignment, for
 * the frontend to know how to render each type's chart. food_meal is
 * intentionally absent — it never accepts ?bucket= (see data-api spec) and
 * has no data page of its own in this UI.
 */
export const TYPE_META: Partial<Record<DataType, TypeMeta>> = {
  steps: { family: 'cumulative', valueCol: 'count' },
  distance: { family: 'cumulative', valueCol: 'meters', unit: 'km' },
  active_calories: { family: 'cumulative', valueCol: 'calories', unit: 'kcal' },
  total_calories: { family: 'cumulative', valueCol: 'calories', unit: 'kcal' },
  hydration: { family: 'cumulative', valueCol: 'liters', unit: 'L' },
  exercise: { family: 'cumulative', valueCol: 'duration_seconds', unit: 'min' },
  sleep: { family: 'cumulative', valueCol: 'duration_seconds', unit: 'h' },
  nutrition: { family: 'cumulative' },
  heart_rate: { family: 'point', valueCol: 'bpm', unit: 'bpm' },
  heart_rate_variability: { family: 'point', valueCol: 'rmssd_millis', unit: 'ms' },
  weight: { family: 'point', valueCol: 'kilograms', unit: 'kg' },
  height: { family: 'point', valueCol: 'meters', unit: 'm' },
  blood_pressure: { family: 'point' },
  blood_glucose: { family: 'point', valueCol: 'mmol_per_liter', unit: 'mmol/L' },
  oxygen_saturation: { family: 'point', valueCol: 'percentage', unit: '%' },
  body_temperature: { family: 'point', valueCol: 'celsius', unit: '°C' },
  skin_temperature: { family: 'point', valueCol: 'delta_celsius', unit: '°C' },
  respiratory_rate: { family: 'point', valueCol: 'rate', unit: '/min' },
  resting_heart_rate: { family: 'point', valueCol: 'bpm', unit: 'bpm' },
  basal_metabolic_rate: { family: 'point', valueCol: 'watts', unit: 'W' },
  body_fat: { family: 'point', valueCol: 'percentage', unit: '%' },
  lean_body_mass: { family: 'point', valueCol: 'kilograms', unit: 'kg' },
  vo2_max: { family: 'point', valueCol: 'ml_per_kg_per_min', unit: 'mL/kg/min' },
  bone_mass: { family: 'point', valueCol: 'kilograms', unit: 'kg' },
  speed: { family: 'point', valueCol: 'meters_per_second', unit: 'm/s' },
};

export const NUTRITION_MACROS = [
  { key: 'calories', label: 'Calories' },
  { key: 'protein_grams', label: 'Protein' },
  { key: 'carbs_grams', label: 'Carbs' },
  { key: 'fat_grams', label: 'Fat' },
  { key: 'sugar_grams', label: 'Sugar' },
  { key: 'sodium_grams', label: 'Sodium' },
  { key: 'dietary_fiber_grams', label: 'Fiber' },
] as const;

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
