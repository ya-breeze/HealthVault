import type { Dictionary } from './i18n';

// Formatting and label lookup for the Nutrition card's top-row disclosure
// (docs/specs/idea.md). Pure and React-free deliberately: this repo has no
// component-test harness, so a pure module is the only way this formatting
// gets unit coverage — every test under frontend/ is a lib/*.test.ts.

// activity_tier arrives from the backend as one of these five English
// literals (backend/pkg/server/activity_level.go's tierSedentary..tierExtra
// Name fields), which are not translated at the source. This maps each to an
// i18n key.
const ACTIVITY_TIER_KEYS: Record<string, keyof Dictionary> = {
  Sedentary: 'loggingGap.tierSedentary',
  'Lightly active': 'loggingGap.tierLight',
  'Moderately active': 'loggingGap.tierModerate',
  'Very active': 'loggingGap.tierActive',
  'Extra active': 'loggingGap.tierExtra',
};

// Returns an i18n key for a known tier name, or `null` when the tier isn't
// recognised (e.g. one added to the backend later) — the caller falls back
// to rendering the raw backend string in that case, rather than passing an
// arbitrary string to `t()` and getting a missing-key placeholder back.
export function activityTierKey(tier: string): keyof Dictionary | null {
  return ACTIVITY_TIER_KEYS[tier] ?? null;
}

const SEX_KEYS: Record<'male' | 'female', keyof Dictionary> = {
  male: 'loggingGap.sexMale',
  female: 'loggingGap.sexFemale',
};

export function sexKey(sex: 'male' | 'female'): keyof Dictionary {
  return SEX_KEYS[sex];
}

// The subset of an available TodaySummaryTarget the panel's five sentences
// interpolate from.
export interface NutritionTargetDerivation {
  bmr: number;
  measured_weight_kg: number;
  goal_weight_kg: number;
  height_m: number;
  age_years: number;
  activity_multiplier: number;
  calories: number;
  protein_grams: number;
}

// The panel's interpolation values, each already formatted as the string the
// panel's copy shows — the caller only pairs these with `t()`-translated sex
// and tier labels.
export interface NutritionTargetPanelValues {
  bmr: string;
  weight: string;
  goal: string;
  height: string;
  age: string;
  multiplier: string;
  calories: string;
  protein: string;
}

// Weight and goal weight to one decimal, height as whole centimetres
// (converted from the metres the backend reports), kcal/grams/age rounded to
// the nearest whole number, and the multiplier stringified directly — its
// five possible values (1.2, 1.375, 1.55, 1.725, 1.9, from
// backend/pkg/server/activity_level.go's tier table) all stringify exactly,
// so there's no rounding to decide on.
export function formatNutritionTargetValues(target: NutritionTargetDerivation): NutritionTargetPanelValues {
  return {
    bmr: String(Math.round(target.bmr)),
    weight: target.measured_weight_kg.toFixed(1),
    goal: target.goal_weight_kg.toFixed(1),
    height: String(Math.round(target.height_m * 100)),
    age: String(Math.round(target.age_years)),
    multiplier: String(target.activity_multiplier),
    calories: String(Math.round(target.calories)),
    protein: String(Math.round(target.protein_grams)),
  };
}
