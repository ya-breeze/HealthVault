import { describe, expect, it } from 'vitest';
import { evaluateSustainability, type SustainabilityInputs } from './sustainability';
import type { LoggingGapResult } from './loggingGap';

// The production example from the spec's "Why" section: BMR 1799, TDEE 2159 (sedentary, ×1.2),
// mean logged intake 1617 over 28 days, weight trend -0.585 kg/week on 90.2 kg (0.65%/week),
// Logging Gap -102 ± 222 → on_track. The rate check must stay silent (0.65% is inside the 1%
// band); the intake check must fire (1617 is well under 1799 * 0.95 = 1709.05).
const workedExampleOnTrack: LoggingGapResult = { kind: 'on_track' };
const workedExampleTrend = { slope: -0.585 / 7, se: 0.02, weightAtWindowEnd: 90.2 };

function inputs(overrides: Partial<SustainabilityInputs>): SustainabilityInputs {
  return {
    gap: workedExampleOnTrack,
    meanLoggedIntake: 1617,
    bmr: 1799,
    trend: workedExampleTrend,
    ...overrides,
  };
}

describe('evaluateSustainability', () => {
  it('worked example: rate stays silent at 0.65%/week, intake fires at 1617 against a 1799 BMR under on_track', () => {
    const warnings = evaluateSustainability(inputs({}));
    expect(warnings).toEqual([{ kind: 'intake_below_bmr', meanLoggedIntake: 1617, bmr: 1799 }]);
  });

  it('does not report intake_below_bmr when the gap is a real gap, not on_track', () => {
    const gap: LoggingGapResult = { kind: 'gap', value: 300, interval: 100 };
    const warnings = evaluateSustainability(inputs({ gap, trend: null }));
    expect(warnings).toEqual([]);
  });

  it('does not report intake_below_bmr when there is not_enough_data', () => {
    const gap: LoggingGapResult = { kind: 'not_enough_data' };
    const warnings = evaluateSustainability(inputs({ gap, trend: null }));
    expect(warnings).toEqual([]);
  });

  it('suppresses the rate check entirely when se is null', () => {
    const trend = { slope: -1.5 / 7, se: null, weightAtWindowEnd: 100 };
    const gap: LoggingGapResult = { kind: 'gap', value: 300, interval: 100 };
    const warnings = evaluateSustainability(inputs({ trend, gap }));
    expect(warnings).toEqual([]);
  });

  it('suppresses the rate check on a positive slope (weight gain, not loss)', () => {
    const trend = { slope: 1.5 / 7, se: 0.05, weightAtWindowEnd: 100 };
    const gap: LoggingGapResult = { kind: 'gap', value: 300, interval: 100 };
    const warnings = evaluateSustainability(inputs({ trend, gap }));
    expect(warnings).toEqual([]);
  });

  it('suppresses the rate check when the slope is inside the sustainable band', () => {
    // 0.5%/week point estimate, conservative bound still under 1%.
    const trend = { slope: -0.5 / 7 * 0.7, se: 0.02, weightAtWindowEnd: 70 };
    const gap: LoggingGapResult = { kind: 'gap', value: 300, interval: 100 };
    const warnings = evaluateSustainability(inputs({ trend, gap }));
    expect(warnings).toEqual([]);
  });

  it('fires the rate check when the conservative slope+se bound clears the band', () => {
    // Point estimate 1.5%/week on 100kg; conservative bound (slope+se) still clears 1%.
    const trend = { slope: -1.5 / 7, se: 0.05, weightAtWindowEnd: 100 };
    const gap: LoggingGapResult = { kind: 'gap', value: 300, interval: 100 };
    const warnings = evaluateSustainability(inputs({ trend, gap }));
    expect(warnings).toHaveLength(1);
    expect(warnings[0].kind).toBe('loss_too_fast');
    if (warnings[0].kind === 'loss_too_fast') {
      expect(warnings[0].percentPerWeek).toBeCloseTo(1.5, 6);
    }
  });

  it('suppresses the intake check when the shortfall is inside the BMR margin', () => {
    const warnings = evaluateSustainability(inputs({ meanLoggedIntake: 1750, trend: null }));
    expect(warnings).toEqual([]);
  });

  it('reports both warnings together, loss_too_fast first', () => {
    const trend = { slope: -1.5 / 7, se: 0.05, weightAtWindowEnd: 100 };
    const warnings = evaluateSustainability(inputs({ trend }));
    expect(warnings.map(w => w.kind)).toEqual(['loss_too_fast', 'intake_below_bmr']);
  });
});
