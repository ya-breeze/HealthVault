import type { DataType } from './api';

export type AggFamily = 'cumulative' | 'point';

export interface TypeMeta {
  family: AggFamily;
}

/**
 * Mirrors backend/pkg/server/api.go's typeRegistry family assignment, for
 * the frontend to know how to render each type's chart. food_meal is
 * intentionally absent — it never accepts ?bucket= (see data-api spec) and
 * has no data page of its own in this UI.
 */
export const TYPE_META: Partial<Record<DataType, TypeMeta>> = {
  steps: { family: 'cumulative' },
  distance: { family: 'cumulative' },
  active_calories: { family: 'cumulative' },
  total_calories: { family: 'cumulative' },
  hydration: { family: 'cumulative' },
  exercise: { family: 'cumulative' },
  sleep: { family: 'cumulative' },
  nutrition: { family: 'cumulative' },
  heart_rate: { family: 'point' },
  heart_rate_variability: { family: 'point' },
  weight: { family: 'point' },
  height: { family: 'point' },
  blood_pressure: { family: 'point' },
  blood_glucose: { family: 'point' },
  oxygen_saturation: { family: 'point' },
  body_temperature: { family: 'point' },
  skin_temperature: { family: 'point' },
  respiratory_rate: { family: 'point' },
  resting_heart_rate: { family: 'point' },
  basal_metabolic_rate: { family: 'point' },
  body_fat: { family: 'point' },
  lean_body_mass: { family: 'point' },
  vo2_max: { family: 'point' },
  bone_mass: { family: 'point' },
  speed: { family: 'point' },
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
