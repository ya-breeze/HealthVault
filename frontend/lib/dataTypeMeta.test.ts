import { describe, expect, it } from 'vitest';
import {
  bmiBandEdgesKg,
  classifyBmi,
  computeProjection,
  computeYDomain,
  emaSeries,
  hasEnoughDataForProjection,
  hasHeightRecord,
  last30DayEmaWindow,
  linearRegression,
  rangeForZoom,
  toDayOffset,
} from './dataTypeMeta';

describe('emaSeries', () => {
  it('returns an empty series for empty input', () => {
    expect(emaSeries([])).toEqual([]);
  });

  it('seeds from the first value', () => {
    expect(emaSeries([80])).toEqual([80]);
  });

  it('smooths subsequent values toward the trend using the given alpha', () => {
    const result = emaSeries([80, 84], 0.25);
    expect(result[0]).toBe(80);
    expect(result[1]).toBeCloseTo(80 + 0.25 * (84 - 80), 10);
  });
});

describe('computeYDomain', () => {
  it('returns undefined for no data', () => {
    expect(computeYDomain([])).toBeUndefined();
  });

  it('pads a flat/single-point series by 2% of its own magnitude', () => {
    const [min, max] = computeYDomain([80])!;
    expect(min).toBeCloseTo(80 - 80 * 0.02, 10);
    expect(max).toBeCloseTo(80 + 80 * 0.02, 10);
  });

  it('pads a ranged series by 10% of the data range', () => {
    const [min, max] = computeYDomain([70, 80])!;
    expect(min).toBeCloseTo(70 - 1, 10);
    expect(max).toBeCloseTo(80 + 1, 10);
  });
});

describe('rangeForZoom', () => {
  it('day zoom has no bucket', () => {
    expect(rangeForZoom('day').bucket).toBeUndefined();
  });

  it('week and month zoom bucket daily', () => {
    expect(rangeForZoom('week').bucket).toBe('day');
    expect(rangeForZoom('month').bucket).toBe('day');
  });

  it('year zoom buckets monthly', () => {
    expect(rangeForZoom('year').bucket).toBe('month');
  });

  it('every zoom returns from before to', () => {
    for (const zoom of ['day', 'week', 'month', 'year'] as const) {
      const { from, to } = rangeForZoom(zoom);
      expect(new Date(from).getTime()).toBeLessThan(new Date(to).getTime());
    }
  });
});

describe('bmiBandEdgesKg', () => {
  it('converts BMI edges to kg for a given height', () => {
    // 1.8m tall: kg = bmi * 1.8^2 = bmi * 3.24
    const [underweightEdge, overweightEdge, obeseEdge] = bmiBandEdgesKg(1.8);
    expect(underweightEdge).toBeCloseTo(18.5 * 3.24, 10);
    expect(overweightEdge).toBeCloseTo(25 * 3.24, 10);
    expect(obeseEdge).toBeCloseTo(30 * 3.24, 10);
  });
});

describe('classifyBmi', () => {
  it('classifies clearly interior values', () => {
    expect(classifyBmi(17)).toBe('Underweight');
    expect(classifyBmi(22)).toBe('Normal');
    expect(classifyBmi(27)).toBe('Overweight');
    expect(classifyBmi(35)).toBe('Obese');
  });

  it('classifies exact boundary values into the higher category', () => {
    expect(classifyBmi(18.5)).toBe('Normal');
    expect(classifyBmi(25)).toBe('Overweight');
    expect(classifyBmi(30)).toBe('Obese');
  });
});

describe('hasHeightRecord', () => {
  it('is false with zero height records', () => {
    expect(hasHeightRecord([])).toBe(false);
  });

  it('is true with one height record', () => {
    expect(hasHeightRecord([{ time: '2026-01-01', value: 1.8 }])).toBe(true);
  });
});

describe('linearRegression', () => {
  it('returns zero slope/intercept for no points', () => {
    expect(linearRegression([])).toEqual({ slope: 0, intercept: 0 });
  });

  it('fits an exact line through collinear points', () => {
    const points = [0, 1, 2, 3].map(x => ({ x, y: 10 + 2 * x }));
    const { slope, intercept } = linearRegression(points);
    expect(slope).toBeCloseTo(2, 10);
    expect(intercept).toBeCloseTo(10, 10);
  });

  it('fits a flat line as zero slope', () => {
    const points = [0, 1, 2].map(x => ({ x, y: 80 }));
    const { slope, intercept } = linearRegression(points);
    expect(slope).toBeCloseTo(0, 10);
    expect(intercept).toBeCloseTo(80, 10);
  });
});

describe('last30DayEmaWindow', () => {
  it('returns empty for no dates', () => {
    expect(last30DayEmaWindow([], [])).toEqual([]);
  });

  it('keeps only the last 30 calendar days relative to the series end', () => {
    const dates = ['2026-01-01', '2026-01-15', '2026-02-01', '2026-02-10'];
    const emaValues = [90, 88, 85, 84];
    const window = last30DayEmaWindow(dates, emaValues);
    // Series ends 2026-02-10; cutoff is 30 days back = 2026-01-12, so 2026-01-01 drops out.
    expect(window.map(w => w.date)).toEqual(['2026-01-15', '2026-02-01', '2026-02-10']);
  });
});

describe('toDayOffset', () => {
  it('is monotonic with calendar date', () => {
    expect(toDayOffset('2026-01-02')).toBe(toDayOffset('2026-01-01') + 1);
  });
});

describe('hasEnoughDataForProjection', () => {
  it('rejects fewer than the minimum total records', () => {
    expect(hasEnoughDataForProjection(4, 30, 10)).toBe(false);
  });

  it('rejects fewer than the minimum lifetime span, even with enough records', () => {
    expect(hasEnoughDataForProjection(5, 13, 10)).toBe(false);
  });

  it('rejects fewer than 2 points inside the 30-day regression window, even with enough lifetime history', () => {
    expect(hasEnoughDataForProjection(20, 365, 1)).toBe(false);
  });

  it('accepts when all three thresholds are met', () => {
    expect(hasEnoughDataForProjection(5, 14, 2)).toBe(true);
  });
});

describe('computeProjection', () => {
  const base = { slope: 0, intercept: 0, goal: 75, windowStartEma: 90, latestEma: 90, lastDayOffset: 100 };

  it('reports not-on-track for a flat trend far from goal', () => {
    expect(computeProjection(base)).toEqual({ status: 'not-on-track' });
  });

  it('reports not-on-track for a trend moving the wrong direction', () => {
    // goal is below windowStartEma (losing weight), but slope is positive (gaining).
    const result = computeProjection({ ...base, slope: 0.1, intercept: 80 });
    expect(result.status).toBe('not-on-track');
  });

  it('reports on-track when the crossing falls just inside the 365-day horizon', () => {
    // Losing weight toward goal=75 from windowStartEma=90; crossing at day 460 -> 360 days out.
    const slope = -0.05;
    const intercept = 90 - slope * 100; // pins line through (100, 90)
    const result = computeProjection({ ...base, slope, intercept });
    expect(result.status).toBe('on-track');
    expect(result.crossingDayOffset).toBeCloseTo((base.goal - intercept) / slope, 6);
    expect(result.crossingDayOffset! - base.lastDayOffset).toBeLessThanOrEqual(365);
  });

  it('reports on-track when the crossing lands at exactly day 365', () => {
    const daysToGoal = 365;
    const crossingDayOffset = base.lastDayOffset + daysToGoal;
    const slope = -1;
    const intercept = base.goal - slope * crossingDayOffset;
    const result = computeProjection({ ...base, slope, intercept });
    expect(result.status).toBe('on-track');
  });

  it('reports not-on-track when the crossing lands at day 366 (beyond horizon)', () => {
    const daysToGoal = 366;
    const crossingDayOffset = base.lastDayOffset + daysToGoal;
    const slope = -1;
    const intercept = base.goal - slope * crossingDayOffset;
    const result = computeProjection({ ...base, slope, intercept });
    expect(result.status).toBe('not-on-track');
  });

  it('reports reached when the flat trend is already at goal', () => {
    const result = computeProjection({ ...base, windowStartEma: 75, latestEma: 75, goal: 75 });
    expect(result.status).toBe('reached');
  });

  it('reports reached when the flat trend has already passed the goal on the far side', () => {
    // windowStartEma above goal (direction = -1 toward goal), latestEma now below goal.
    const result = computeProjection({ ...base, windowStartEma: 90, latestEma: 70, goal: 75, slope: 0 });
    expect(result.status).toBe('reached');
  });

  it('reports not-on-track when an old lifetime weight sits on the opposite side of the goal from the current flat window', () => {
    // windowStartEma/latestEma (the 30-day window) are both 90, well above goal 75 — the
    // caller must never substitute the lifetime-earliest weight (60) for windowStartEma here.
    const result = computeProjection({ slope: 0, intercept: 90, goal: 75, windowStartEma: 90, latestEma: 90, lastDayOffset: 100 });
    expect(result.status).toBe('not-on-track');
  });

  it('reports not-on-track (not reached) when windowStartEma equals goal exactly but latestEma has since diverged', () => {
    const result = computeProjection({ slope: 0, intercept: 80, goal: 75, windowStartEma: 75, latestEma: 80, lastDayOffset: 100 });
    expect(result.status).toBe('not-on-track');
  });
});
