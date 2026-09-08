import { describe, expect, it, vi } from 'vitest';
import { DATA_TYPES, type DataType } from './api';
import { dataColumnLabel } from './dataColumnMeta';
import en from './i18n/en';
import ru from './i18n/ru';
import type { Dictionary } from './i18n';

const translate = (dictionary: Dictionary) => (key: keyof Dictionary) => dictionary[key];
const tEn = translate(en);
const tRu = translate(ru);

const AUDIT_COLUMNS = ['created_at', 'updated_at'] as const;

// Mirrors the user-visible columns returned by QueryRecords: TenantModel and
// model fields from backend/pkg/database/models.go, minus DataTypeClient's
// existing identifier filter. food_meal instead mirrors columnAllowlist in
// backend/pkg/database/storage_impl.go, again minus its filtered id.
const CURRENT_USER_VISIBLE_COLUMNS = {
  steps: [...AUDIT_COLUMNS, 'start_time', 'end_time', 'count'],
  heart_rate: [...AUDIT_COLUMNS, 'time', 'bpm'],
  heart_rate_variability: [...AUDIT_COLUMNS, 'time', 'rmssd_millis'],
  sleep: [...AUDIT_COLUMNS, 'start_time', 'session_end_time', 'duration_seconds'],
  distance: [...AUDIT_COLUMNS, 'start_time', 'end_time', 'meters'],
  active_calories: [...AUDIT_COLUMNS, 'start_time', 'end_time', 'calories'],
  total_calories: [...AUDIT_COLUMNS, 'start_time', 'end_time', 'calories'],
  weight: [...AUDIT_COLUMNS, 'time', 'kilograms'],
  height: [...AUDIT_COLUMNS, 'time', 'meters'],
  blood_pressure: [...AUDIT_COLUMNS, 'time', 'systolic', 'diastolic'],
  blood_glucose: [...AUDIT_COLUMNS, 'time', 'mmol_per_liter'],
  oxygen_saturation: [...AUDIT_COLUMNS, 'time', 'percentage'],
  body_temperature: [...AUDIT_COLUMNS, 'time', 'celsius'],
  skin_temperature: [
    ...AUDIT_COLUMNS, 'time', 'delta_celsius', 'baseline_celsius', 'measurement_location',
  ],
  respiratory_rate: [...AUDIT_COLUMNS, 'time', 'rate'],
  resting_heart_rate: [...AUDIT_COLUMNS, 'time', 'bpm'],
  exercise: [
    ...AUDIT_COLUMNS, 'start_time', 'end_time', 'duration_seconds', 'exercise_type',
    'distance_meters', 'steps', 'avg_cadence_spm', 'max_cadence_spm', 'stride_length_m',
  ],
  hydration: [...AUDIT_COLUMNS, 'start_time', 'end_time', 'liters'],
  nutrition: [
    ...AUDIT_COLUMNS, 'start_time', 'end_time', 'calories', 'protein_grams', 'carbs_grams',
    'fat_grams', 'sugar_grams', 'sodium_grams', 'dietary_fiber_grams', 'name',
  ],
  basal_metabolic_rate: [...AUDIT_COLUMNS, 'time', 'watts'],
  body_fat: [...AUDIT_COLUMNS, 'time', 'percentage'],
  lean_body_mass: [...AUDIT_COLUMNS, 'time', 'kilograms'],
  vo2_max: [...AUDIT_COLUMNS, 'time', 'ml_per_kg_per_min'],
  bone_mass: [...AUDIT_COLUMNS, 'time', 'kilograms'],
  speed: [...AUDIT_COLUMNS, 'time', 'meters_per_second'],
  food_meal: [
    'logged_at', 'name', 'status', 'calories', 'protein_grams', 'carbs_grams', 'fat_grams',
    'sugar_grams', 'sodium_grams', 'dietary_fiber_grams',
  ],
  weight_goal: [...AUDIT_COLUMNS, 'time', 'kilograms'],
} as const satisfies Record<DataType, readonly string[]>;

describe('dataColumnLabel', () => {
  it('resolves shared labels in English and Russian', () => {
    expect(dataColumnLabel(tEn, 'steps', 'created_at')).toBe('Created at');
    expect(dataColumnLabel(tRu, 'steps', 'created_at')).toBe('Создано');
  });

  it('uses metric-specific labels for ambiguous raw value columns and stored units', () => {
    expect(dataColumnLabel(tEn, 'distance', 'meters')).toBe('Distance (m)');
    expect(dataColumnLabel(tEn, 'height', 'meters')).toBe('Height (m)');
    expect(dataColumnLabel(tEn, 'sleep', 'duration_seconds')).toBe('Sleep duration (seconds)');
    expect(dataColumnLabel(tEn, 'exercise', 'duration_seconds')).toBe('Exercise duration (seconds)');
    expect(dataColumnLabel(tRu, 'weight', 'kilograms')).toBe('Вес (кг)');
    expect(dataColumnLabel(tRu, 'bone_mass', 'kilograms')).toBe('Костная масса (кг)');
  });

  it('resolves unique metric and food-meal allowlist fields', () => {
    expect(dataColumnLabel(tEn, 'skin_temperature', 'baseline_celsius'))
      .toBe('Baseline temperature (°C)');
    expect(dataColumnLabel(tRu, 'exercise', 'avg_cadence_spm'))
      .toBe('Средний темп (шагов/мин)');
    for (const column of CURRENT_USER_VISIBLE_COLUMNS.food_meal) {
      expect(dataColumnLabel(tEn, 'food_meal', column)).not.toBe(en['dataTable.column.unknown']);
      expect(dataColumnLabel(tRu, 'food_meal', column)).not.toBe(ru['dataTable.column.unknown']);
    }
  });

  it('uses a localized generic label for an unexpected future key', () => {
    const warning = vi.spyOn(console, 'warn').mockImplementation(() => {});
    try {
      expect(dataColumnLabel(tEn, 'steps', 'future_schema_key')).toBe('Field');
      expect(dataColumnLabel(tRu, 'steps', 'future_schema_key')).toBe('Поле');
      expect(warning).toHaveBeenCalledWith(
        'Unknown data-table column "future_schema_key" for data type "steps"',
      );
    } finally {
      warning.mockRestore();
    }
  });

  it('has non-fallback English and Russian labels for every current visible raw column', () => {
    expect(Object.keys(CURRENT_USER_VISIBLE_COLUMNS)).toEqual([...DATA_TYPES]);

    for (const type of DATA_TYPES) {
      for (const column of CURRENT_USER_VISIBLE_COLUMNS[type]) {
        const english = dataColumnLabel(tEn, type, column);
        const russian = dataColumnLabel(tRu, type, column);
        expect(english, `${type}.${column} English`).not.toBe(column);
        expect(russian, `${type}.${column} Russian`).not.toBe(column);
        expect(english, `${type}.${column} English`).not.toBe(en['dataTable.column.unknown']);
        expect(russian, `${type}.${column} Russian`).not.toBe(ru['dataTable.column.unknown']);
      }
    }
  });
});
