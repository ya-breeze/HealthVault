import type { DataType } from './api';
import type { Dictionary } from './i18n';

type Translate = (key: keyof Dictionary) => string;
type ColumnLabelKey = Extract<keyof Dictionary, `dataTable.column.${string}`>;

// Columns whose meaning is independent of the record type. The values are
// dictionary keys rather than display strings so this transport-to-presentation
// registry cannot accidentally become an English-only source of UI copy.
const SHARED_COLUMN_LABEL_KEYS: Readonly<Record<string, ColumnLabelKey>> = {
  created_at: 'dataTable.column.createdAt',
  updated_at: 'dataTable.column.updatedAt',
  time: 'dataTable.column.time',
  start_time: 'dataTable.column.startTime',
  end_time: 'dataTable.column.endTime',
  session_end_time: 'dataTable.column.sessionEndTime',
  logged_at: 'dataTable.column.loggedAt',
  name: 'dataTable.column.name',
  status: 'dataTable.column.status',
  count: 'dataTable.column.stepCount',
  liters: 'dataTable.column.hydrationLiters',
  rmssd_millis: 'dataTable.column.rmssdMillis',
  mmol_per_liter: 'dataTable.column.bloodGlucoseMmolPerLiter',
  celsius: 'dataTable.column.bodyTemperatureCelsius',
  rate: 'dataTable.column.respiratoryRate',
  watts: 'dataTable.column.basalMetabolicRateWatts',
  systolic: 'dataTable.column.systolic',
  diastolic: 'dataTable.column.diastolic',
  delta_celsius: 'dataTable.column.skinTemperatureDeltaCelsius',
  baseline_celsius: 'dataTable.column.skinTemperatureBaselineCelsius',
  measurement_location: 'dataTable.column.measurementLocation',
  exercise_type: 'dataTable.column.exerciseType',
  distance_meters: 'dataTable.column.exerciseDistanceMeters',
  steps: 'dataTable.column.exerciseSteps',
  avg_cadence_spm: 'dataTable.column.averageCadenceSpm',
  max_cadence_spm: 'dataTable.column.maximumCadenceSpm',
  stride_length_m: 'dataTable.column.strideLengthMeters',
  meters_per_second: 'dataTable.column.speedMetersPerSecond',
  protein_grams: 'dataTable.column.proteinGrams',
  carbs_grams: 'dataTable.column.carbsGrams',
  fat_grams: 'dataTable.column.fatGrams',
  sugar_grams: 'dataTable.column.sugarGrams',
  sodium_grams: 'dataTable.column.sodiumGrams',
  dietary_fiber_grams: 'dataTable.column.dietaryFiberGrams',
};

// These raw names occur in more than one schema and do not describe the
// measurement on their own. Keep the stored unit in each label because the
// record table deliberately displays raw values (unlike some charts).
const TYPE_COLUMN_LABEL_KEYS: Partial<
  Record<DataType, Readonly<Record<string, ColumnLabelKey>>>
> = {
  distance: { meters: 'dataTable.column.distanceMeters' },
  height: { meters: 'dataTable.column.heightMeters' },
  weight: { kilograms: 'dataTable.column.weightKilograms' },
  weight_goal: { kilograms: 'dataTable.column.goalWeightKilograms' },
  lean_body_mass: { kilograms: 'dataTable.column.leanBodyMassKilograms' },
  bone_mass: { kilograms: 'dataTable.column.boneMassKilograms' },
  heart_rate: { bpm: 'dataTable.column.heartRateBpm' },
  resting_heart_rate: { bpm: 'dataTable.column.restingHeartRateBpm' },
  oxygen_saturation: { percentage: 'dataTable.column.oxygenSaturationPercentage' },
  body_fat: { percentage: 'dataTable.column.bodyFatPercentage' },
  active_calories: { calories: 'dataTable.column.activeCaloriesKcal' },
  total_calories: { calories: 'dataTable.column.totalCaloriesKcal' },
  nutrition: { calories: 'dataTable.column.nutritionCaloriesKcal' },
  food_meal: { calories: 'dataTable.column.mealCaloriesKcal' },
  sleep: { duration_seconds: 'dataTable.column.sleepDurationSeconds' },
  exercise: { duration_seconds: 'dataTable.column.exerciseDurationSeconds' },
};

export function dataColumnLabel(t: Translate, type: DataType, column: string): string {
  const key = TYPE_COLUMN_LABEL_KEYS[type]?.[column] ?? SHARED_COLUMN_LABEL_KEYS[column];
  if (key) return t(key);

  // Do not expose a future backend schema key to the UI. It remains available
  // in development diagnostics so the missing presentation metadata is easy
  // to identify and add.
  if (process.env.NODE_ENV !== 'production') {
    console.warn(`Unknown data-table column "${column}" for data type "${type}"`);
  }
  return t('dataTable.column.unknown');
}
