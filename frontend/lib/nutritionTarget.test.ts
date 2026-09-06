import { describe, expect, it } from 'vitest';
import { activityTierKey, sexKey, formatNutritionTargetValues } from './nutritionTarget';

describe('activityTierKey', () => {
  it.each([
    ['Sedentary', 'loggingGap.tierSedentary'],
    ['Lightly active', 'loggingGap.tierLight'],
    ['Moderately active', 'loggingGap.tierModerate'],
    ['Very active', 'loggingGap.tierActive'],
    ['Extra active', 'loggingGap.tierExtra'],
  ])('maps %s to %s', (tier, key) => {
    expect(activityTierKey(tier)).toBe(key);
  });

  it('returns null for an unrecognised tier, so the caller can fall back to the raw string', () => {
    expect(activityTierKey('Ultra active')).toBeNull();
  });
});

describe('sexKey', () => {
  it('maps male', () => {
    expect(sexKey('male')).toBe('loggingGap.sexMale');
  });

  it('maps female', () => {
    expect(sexKey('female')).toBe('loggingGap.sexFemale');
  });
});

describe('formatNutritionTargetValues', () => {
  const base = {
    bmr: 1750.4,
    measured_weight_kg: 80.456,
    goal_weight_kg: 75.049,
    height_m: 1.803,
    age_years: 35,
    activity_multiplier: 1.55,
    calories: 2712.9,
    protein_grams: 120.4,
  };

  it('rounds bmr, calories, protein and age to whole numbers', () => {
    const v = formatNutritionTargetValues(base);
    expect(v.bmr).toBe('1750');
    expect(v.calories).toBe('2713');
    expect(v.protein).toBe('120');
    expect(v.age).toBe('35');
  });

  it('formats measured and goal weight to one decimal', () => {
    const v = formatNutritionTargetValues(base);
    expect(v.weight).toBe('80.5');
    expect(v.goal).toBe('75.0');
  });

  it('converts height from metres to whole centimetres', () => {
    const v = formatNutritionTargetValues({ ...base, height_m: 1.803 });
    expect(formatNutritionTargetValues({ ...base, height_m: 1.8 }).height).toBe('180');
    expect(v.height).toBe('180');
  });

  it.each([
    [1.2, '1.2'],
    [1.375, '1.375'],
    [1.55, '1.55'],
    [1.725, '1.725'],
    [1.9, '1.9'],
  ])('stringifies the activity multiplier %s exactly as %s', (multiplier, want) => {
    expect(formatNutritionTargetValues({ ...base, activity_multiplier: multiplier }).multiplier).toBe(want);
  });
});
