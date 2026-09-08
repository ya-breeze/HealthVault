import { describe, expect, it } from 'vitest';
import {
  computeHealthinessLabel,
  HEALTHINESS_THRESHOLDS,
  type HealthinessDayData,
  type HealthinessWindow,
} from './healthiness';

const WINDOW: HealthinessWindow = { startDayOffset: 0, endDayOffset: 6 };

function day(overrides: Partial<HealthinessDayData> = {}): HealthinessDayData {
  return {
    state: 'complete',
    calories: 2000,
    unconfirmedMeals: 0,
    proteinGrams: 0,
    carbsGrams: 0,
    fatGrams: 0,
    sugarGrams: 0,
    sodiumGrams: 0,
    ...overrides,
  };
}

// This app's own Nutrition Target split (spec: "an ordinary target lands near 22% protein, 39%
// carbs, 39% fat"), reproduced in grams for a 2000 kcal day: 4*110 + 4*195 + 9*86.667 = 2000.
const GOOD_DAY = { proteinGrams: 110, carbsGrams: 195, fatGrams: 86.667, sugarGrams: 20, sodiumGrams: 1.5 };

function poolOf(count: number, values: Partial<HealthinessDayData> = {}): Record<number, HealthinessDayData> {
  const perDayData: Record<number, HealthinessDayData> = {};
  for (let i = 0; i < count; i++) perDayData[i] = day(values);
  return perDayData;
}

describe('computeHealthinessLabel — eligibility floor', () => {
  it('yields no label with fewer than three eligible days', () => {
    const perDayData: Record<number, HealthinessDayData> = {
      0: day(GOOD_DAY),
      1: day(GOOD_DAY),
      2: day({ ...GOOD_DAY, state: 'incomplete' }),
    };
    expect(computeHealthinessLabel(perDayData, WINDOW)).toBeNull();
  });

  it('is not null once a third day becomes eligible', () => {
    const perDayData = poolOf(3, GOOD_DAY);
    expect(computeHealthinessLabel(perDayData, WINDOW)).not.toBeNull();
  });

  it('excludes non-eligible days from the pool itself, not merely from the eligible-day count', () => {
    const perDayData: Record<number, HealthinessDayData> = {
      0: day(GOOD_DAY),
      1: day(GOOD_DAY),
      2: day(GOOD_DAY),
      // These days are Incomplete, so isValidDay excludes them — but their
      // extreme sugar would flip the label to needs_attention if it leaked
      // into the pooled sums instead of being dropped along with the day.
      3: day({ state: 'incomplete', proteinGrams: 0, carbsGrams: 0, fatGrams: 0, sugarGrams: 500, sodiumGrams: 10 }),
      4: day({ state: 'incomplete', proteinGrams: 0, carbsGrams: 0, fatGrams: 0, sugarGrams: 500, sodiumGrams: 10 }),
    };
    const result = computeHealthinessLabel(perDayData, WINDOW);
    expect(result).toEqual({ label: 'good', reasons: [] });
  });

  it('excludes a day whose meals never reached confirmed even when Day Completeness says complete', () => {
    const perDayData: Record<number, HealthinessDayData> = {
      0: day(GOOD_DAY),
      1: day(GOOD_DAY),
      2: day({ ...GOOD_DAY, unconfirmedMeals: 1 }),
    };
    expect(computeHealthinessLabel(perDayData, WINDOW)).toBeNull();
  });

  it('yields no label when pooled macro energy is zero, even with enough eligible days', () => {
    const perDayData = poolOf(3);
    expect(computeHealthinessLabel(perDayData, WINDOW)).toBeNull();
  });

  it('only reads days inside the given window', () => {
    const perDayData: Record<number, HealthinessDayData> = {
      // Outside [0, 6] — must not count toward eligibility or the pool.
      10: day(GOOD_DAY),
      11: day(GOOD_DAY),
      12: day(GOOD_DAY),
      0: day(GOOD_DAY),
      1: day(GOOD_DAY),
    };
    expect(computeHealthinessLabel(perDayData, WINDOW)).toBeNull();
  });
});

describe('computeHealthinessLabel — the Nutrition Target split is good', () => {
  it('labels a pooled window built from this app\'s own Nutrition Target split as good', () => {
    const perDayData = poolOf(7, GOOD_DAY);
    expect(computeHealthinessLabel(perDayData, WINDOW)).toEqual({ label: 'good', reasons: [] });
  });
});

// A pooled macro-energy total divisible by both 4 and 9, so every gram amount below is an exact
// integer and share↔gram round trips never pick up floating-point noise — critical right at a
// boundary, where even 1e-13 of noise can flip which side of an inclusive `<=` a value lands on.
const M = 3600;

function gramsForShare(share: number, energyPerGram: 4 | 9): number {
  return Math.round(share * (M / energyPerGram));
}

describe('computeHealthinessLabel — protein share boundaries (fat fixed ok at 0.28)', () => {
  // fatGrams is a fixed integer (fatShare = 9*112/3600 = 0.28) safely inside fat's own ok band
  // regardless of protein; carbGrams is the exact integer remainder, so it's whatever's left of M
  // — comfortably inside carbs' own ok band [0.25, 0.65] for every protein value tested below.
  // Both stay clear of their own boundaries, isolating the protein signal under test.
  function poolAtProteinShare(proteinShare: number): Record<number, HealthinessDayData> {
    const fatGrams = 112;
    const proteinGrams = gramsForShare(proteinShare, 4);
    const carbGrams = (M - 4 * proteinGrams - 9 * fatGrams) / 4;
    return poolOf(3, { proteinGrams, carbsGrams: carbGrams, fatGrams, sugarGrams: 0, sodiumGrams: 0 });
  }

  it('0.15 (offLow/ok boundary) lands on the ok side', () => {
    const result = computeHealthinessLabel(poolAtProteinShare(HEALTHINESS_THRESHOLDS.proteinShare.offLow), WINDOW);
    expect(result).toEqual({ label: 'good', reasons: [] });
  });

  it('0.40 (ok/offHigh boundary) lands on the ok side', () => {
    const result = computeHealthinessLabel(poolAtProteinShare(HEALTHINESS_THRESHOLDS.proteinShare.offHigh), WINDOW);
    expect(result).toEqual({ label: 'good', reasons: [] });
  });

  it('0.10 (farLow/offLow boundary) lands on the off side, not far', () => {
    const result = computeHealthinessLabel(poolAtProteinShare(HEALTHINESS_THRESHOLDS.proteinShare.farLow), WINDOW);
    expect(result).toEqual({ label: 'fair', reasons: ['protein_low'] });
  });

  it('0.45 (offHigh/farHigh boundary) lands on the off side, not far', () => {
    const result = computeHealthinessLabel(poolAtProteinShare(HEALTHINESS_THRESHOLDS.proteinShare.farHigh), WINDOW);
    expect(result).toEqual({ label: 'fair', reasons: ['protein_high'] });
  });

  it('just under the farLow boundary is far, not off', () => {
    const result = computeHealthinessLabel(poolAtProteinShare(0.09), WINDOW);
    expect(result?.label).toBe('needs_attention');
    expect(result?.reasons).toContain('protein_low');
  });

  it('just over the farHigh boundary is far, not off', () => {
    const result = computeHealthinessLabel(poolAtProteinShare(0.46), WINDOW);
    expect(result?.label).toBe('needs_attention');
    expect(result?.reasons).toContain('protein_high');
  });
});

describe('computeHealthinessLabel — fat share boundaries (protein fixed ok at 0.25)', () => {
  // proteinGrams is a fixed integer (proteinShare = 4*225/3600 = 0.25) safely inside protein's own
  // ok band; carbGrams is the exact integer remainder, staying inside carbs' own ok band for every
  // fat value tested below (see the M=3600 comment above).
  function poolAtFatShare(fatShare: number): Record<number, HealthinessDayData> {
    const proteinGrams = 225;
    const fatGrams = gramsForShare(fatShare, 9);
    const carbGrams = (M - 4 * proteinGrams - 9 * fatGrams) / 4;
    return poolOf(3, { proteinGrams, carbsGrams: carbGrams, fatGrams, sugarGrams: 0, sodiumGrams: 0 });
  }

  it('0.20 (offLow/ok boundary) lands on the ok side', () => {
    const result = computeHealthinessLabel(poolAtFatShare(HEALTHINESS_THRESHOLDS.fatShare.offLow), WINDOW);
    expect(result).toEqual({ label: 'good', reasons: [] });
  });

  it('0.40 (ok/offHigh boundary) lands on the ok side', () => {
    const result = computeHealthinessLabel(poolAtFatShare(HEALTHINESS_THRESHOLDS.fatShare.offHigh), WINDOW);
    expect(result).toEqual({ label: 'good', reasons: [] });
  });

  it('0.15 (farLow/offLow boundary) lands on the off side, not far', () => {
    const result = computeHealthinessLabel(poolAtFatShare(HEALTHINESS_THRESHOLDS.fatShare.farLow), WINDOW);
    expect(result).toEqual({ label: 'fair', reasons: ['fat_low'] });
  });

  it('0.48 (offHigh/farHigh boundary) lands on the off side, not far', () => {
    const result = computeHealthinessLabel(poolAtFatShare(HEALTHINESS_THRESHOLDS.fatShare.farHigh), WINDOW);
    expect(result).toEqual({ label: 'fair', reasons: ['fat_high'] });
  });
});

describe('computeHealthinessLabel — carb share boundaries', () => {
  it('0.25 (offLow/ok boundary) lands on the ok side (protein=0.36, fat=0.39, both comfortably ok)', () => {
    const carbGrams = gramsForShare(HEALTHINESS_THRESHOLDS.carbShare.offLow, 4);
    const proteinGrams = gramsForShare(0.36, 4);
    const fatGrams = (M - 4 * proteinGrams - 4 * carbGrams) / 9;
    const perDayData = poolOf(3, { proteinGrams, carbsGrams: carbGrams, fatGrams });
    expect(computeHealthinessLabel(perDayData, WINDOW)).toEqual({ label: 'good', reasons: [] });
  });

  it('0.65 (ok/offHigh boundary) lands on the ok side — the only fit forces protein and fat to their own offLow boundaries too', () => {
    // Combined ok maximum for protein+fat is 0.40+0.40 = 0.80; a carb share of 0.65 leaves only
    // 0.35 for the other two, which exactly matches their combined ok *minimum* (0.15+0.20). So
    // protein=0.15 and fat=0.20 aren't an arbitrary choice here — they're the only values that
    // keep every signal on the ok side while carbs sits at 0.65.
    const carbGrams = gramsForShare(HEALTHINESS_THRESHOLDS.carbShare.offHigh, 4);
    const proteinGrams = 135; // proteinShare = 4*135/3600 = 0.15
    const fatGrams = 80; // fatShare = 9*80/3600 = 0.20
    const perDayData = poolOf(3, { proteinGrams, carbsGrams: carbGrams, fatGrams });
    expect(computeHealthinessLabel(perDayData, WINDOW)).toEqual({ label: 'good', reasons: [] });
  });

  it('a carb share below farLow is far — its arithmetic partners cannot be the explanation', () => {
    const energy = 2000;
    const perDayData = poolOf(3, {
      proteinGrams: (0.35 * energy) / 4,
      carbsGrams: (0.1 * energy) / 4,
      fatGrams: (0.55 * energy) / 9,
    });
    const result = computeHealthinessLabel(perDayData, WINDOW);
    expect(result?.label).toBe('needs_attention');
  });

  it('a carb share above farHigh is far — its arithmetic partners cannot be the explanation', () => {
    const energy = 2000;
    const perDayData = poolOf(3, {
      proteinGrams: (0.14 * energy) / 4,
      carbsGrams: (0.78 * energy) / 4,
      fatGrams: (0.08 * energy) / 9,
    });
    const result = computeHealthinessLabel(perDayData, WINDOW);
    expect(result?.label).toBe('needs_attention');
  });
});

describe('computeHealthinessLabel — sugar share boundary (macros held at the good baseline)', () => {
  function poolAtSugarShare(sugarShare: number): Record<number, HealthinessDayData> {
    const energy = 2000;
    return poolOf(3, { ...GOOD_DAY, sugarGrams: (sugarShare * energy) / 4, sodiumGrams: 0 });
  }

  it('0.15 (offLow/ok boundary) lands on the ok side', () => {
    const result = computeHealthinessLabel(poolAtSugarShare(HEALTHINESS_THRESHOLDS.sugarShare.offLow), WINDOW);
    expect(result).toEqual({ label: 'good', reasons: [] });
  });

  it('0.22 (offHigh/farLow boundary) lands on the off side, not far', () => {
    const result = computeHealthinessLabel(poolAtSugarShare(HEALTHINESS_THRESHOLDS.sugarShare.farLow), WINDOW);
    expect(result).toEqual({ label: 'fair', reasons: ['sugar_high'] });
  });

  it('just over 0.22 is far', () => {
    const result = computeHealthinessLabel(poolAtSugarShare(0.23), WINDOW);
    expect(result).toEqual({ label: 'needs_attention', reasons: ['sugar_high'] });
  });
});

describe('computeHealthinessLabel — sodium boundary (macros held at the good baseline)', () => {
  function poolAtSodiumGramsPerDay(sodiumGramsPerDay: number): Record<number, HealthinessDayData> {
    return poolOf(3, { ...GOOD_DAY, sugarGrams: 0, sodiumGrams: sodiumGramsPerDay });
  }

  it('2.3 g/day (offLow/ok boundary) lands on the ok side', () => {
    const result = computeHealthinessLabel(
      poolAtSodiumGramsPerDay(HEALTHINESS_THRESHOLDS.sodiumGramsPerDay.offLow),
      WINDOW
    );
    expect(result).toEqual({ label: 'good', reasons: [] });
  });

  it('3.5 g/day (offHigh/farLow boundary) lands on the off side, not far', () => {
    const result = computeHealthinessLabel(
      poolAtSodiumGramsPerDay(HEALTHINESS_THRESHOLDS.sodiumGramsPerDay.farLow),
      WINDOW
    );
    expect(result).toEqual({ label: 'fair', reasons: ['sodium_high'] });
  });

  it('just over 3.5 g/day is far', () => {
    const result = computeHealthinessLabel(poolAtSodiumGramsPerDay(3.6), WINDOW);
    expect(result).toEqual({ label: 'needs_attention', reasons: ['sodium_high'] });
  });
});

describe('computeHealthinessLabel — combination rule', () => {
  it('one off signal is fair', () => {
    const perDayData = poolOf(7, { ...GOOD_DAY, sodiumGrams: 2.8 });
    expect(computeHealthinessLabel(perDayData, WINDOW)).toEqual({ label: 'fair', reasons: ['sodium_high'] });
  });

  it('three off signals is needs_attention', () => {
    const energy = 2000;
    // protein off-low (0.12), carbs/fat ok, sugar off, sodium off.
    const perDayData = poolOf(7, {
      proteinGrams: (0.12 * energy) / 4,
      carbsGrams: (0.55 * energy) / 4,
      fatGrams: (0.33 * energy) / 9,
      sugarGrams: (0.18 * energy) / 4,
      sodiumGrams: 2.8,
    });
    const result = computeHealthinessLabel(perDayData, WINDOW);
    expect(result?.label).toBe('needs_attention');
    // Fixed tie-break order (protein, sugar, sodium, fat, carbs): the first
    // two offs in that order are protein and sugar.
    expect(result?.reasons).toEqual(['protein_low', 'sugar_high']);
  });

  it('any far signal is needs_attention even with every other signal ok', () => {
    const perDayData = poolOf(7, { ...GOOD_DAY, sodiumGrams: 4.0 });
    expect(computeHealthinessLabel(perDayData, WINDOW)).toEqual({
      label: 'needs_attention',
      reasons: ['sodium_high'],
    });
  });

  it('all five signals ok is good, with no reasons', () => {
    const perDayData = poolOf(7, GOOD_DAY);
    expect(computeHealthinessLabel(perDayData, WINDOW)).toEqual({ label: 'good', reasons: [] });
  });
});

describe('computeHealthinessLabel — reason ordering and cap', () => {
  it('a far signal is listed before an off signal regardless of their tie-break order', () => {
    // Sugar (earlier in tie-break order) is merely off; sodium (later) is far.
    // far must still be listed first.
    const perDayData = poolOf(7, {
      ...GOOD_DAY,
      sugarGrams: (0.18 * 2000) / 4, // off
      sodiumGrams: 4.0, // far
    });
    const result = computeHealthinessLabel(perDayData, WINDOW);
    expect(result?.label).toBe('needs_attention');
    expect(result?.reasons).toEqual(['sodium_high', 'sugar_high']);
  });

  it('caps the reason list at two, keeping the first two in fixed signal order among the worst verdict', () => {
    const energy = 2000;
    // protein far-high, fat far-high, carbs also far (absorbing the remainder) — three far
    // signals. Fixed order is protein, sugar, sodium, fat, carbs, so only protein and fat survive
    // the cap even though carbs is far too.
    const perDayData = poolOf(7, {
      proteinGrams: (0.5 * energy) / 4,
      fatGrams: (0.49 * energy) / 9,
      carbsGrams: (0.01 * energy) / 4,
      sugarGrams: 0,
      sodiumGrams: 0,
    });
    const result = computeHealthinessLabel(perDayData, WINDOW);
    expect(result?.label).toBe('needs_attention');
    expect(result?.reasons).toHaveLength(2);
    expect(result?.reasons).toEqual(['protein_high', 'fat_high']);
  });
});
