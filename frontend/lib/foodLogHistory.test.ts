import { describe, expect, it } from 'vitest';
import type { DailyTotal, DayCompleteness } from './api';
import {
  OUTCOME_COLOR,
  resolveFoodLogHistoryWindow,
  summarizeFoodLogHistory,
  type FoodLogHistoryDay,
} from './foodLogHistory';

const DATES = ['2026-03-01', '2026-03-02', '2026-03-03', '2026-03-04', '2026-03-05', '2026-03-06', '2026-03-07'];

function completeness(state: DayCompleteness['state'] = 'complete', occasion_count = 3): DayCompleteness {
  return { date: '', state, occasion_count };
}

function dailyTotal(unconfirmed_meals = 0): DailyTotal {
  return {
    date: '',
    calories: 1800,
    protein_grams: 100,
    carbs_grams: 200,
    fat_grams: 60,
    sugar_grams: 20,
    sodium_grams: 2,
    dietary_fiber_grams: 25,
    unconfirmed_meals,
  };
}

function rows(
  states: DayCompleteness['state'][] = DATES.map(() => 'complete'),
  totals: number[] = DATES.map(() => 0),
): { completeness: DayCompleteness[]; dailyTotals: DailyTotal[] } {
  return {
    completeness: DATES.map((date, i) => ({ ...completeness(states[i], states[i] === 'incomplete' ? 0 : 3), date })),
    dailyTotals: DATES.map((date, i) => ({ ...dailyTotal(totals[i]), date })),
  };
}

function summarize(
  states?: DayCompleteness['state'][],
  totals?: number[],
) {
  const input = rows(states, totals);
  return summarizeFoodLogHistory(input.completeness, input.dailyTotals, DATES);
}

describe('resolveFoodLogHistoryWindow', () => {
  it('returns seven chronological closed days in UTC', () => {
    expect(resolveFoodLogHistoryWindow(new Date('2026-03-08T00:00:01Z'), 'UTC')).toEqual({
      from: '2026-03-01',
      to: '2026-03-07',
    });
    expect(resolveFoodLogHistoryWindow(new Date('2026-03-08T23:59:59Z'), 'UTC')).toEqual({
      from: '2026-03-01',
      to: '2026-03-07',
    });
  });

  it('uses the stored timezone at a UTC date boundary', () => {
    expect(resolveFoodLogHistoryWindow(new Date('2026-03-08T00:30:00Z'), 'Pacific/Honolulu')).toEqual({
      from: '2026-02-28',
      to: '2026-03-06',
    });
    expect(resolveFoodLogHistoryWindow(new Date('2026-03-07T23:30:00Z'), 'Pacific/Kiritimati')).toEqual({
      from: '2026-03-01',
      to: '2026-03-07',
    });
  });
});

describe('summarizeFoodLogHistory', () => {
  it('returns chronological days and all three outcomes', () => {
    const states: DayCompleteness['state'][] = [
      'complete',
      'confirmed_complete',
      'unconfirmed',
      'incomplete',
      'complete',
      'confirmed_complete',
      'incomplete',
    ];
    const result = summarize(states, [0, 0, 0, 0, 1, 2, 0]);
    expect(result.days.map(day => day.date)).toEqual(DATES);
    expect(result.days.map(day => day.outcome)).toEqual([
      'counted',
      'counted',
      'needs_attention',
      'no_food',
      'needs_attention',
      'needs_attention',
      'no_food',
    ]);
    expect(result).toMatchObject({ countedDays: 2, noFoodDays: 2, needsAttentionDays: 3 });
  });

  it('reuses isValidDay: unresolved meals disqualify complete-looking days', () => {
    const states: DayCompleteness['state'][] = ['complete', 'confirmed_complete', ...DATES.slice(2).map(() => 'complete' as const)];
    const result = summarize(states, [1, 1, 0, 0, 0, 0, 0]);
    expect(result.days.slice(0, 2).map(day => day.outcome)).toEqual(['needs_attention', 'needs_attention']);
    expect(result.countedDays).toBe(5);
  });

  it('keeps overlapping attention reasons on the day result', () => {
    const states: DayCompleteness['state'][] = ['unconfirmed', ...DATES.slice(1).map(() => 'complete' as const)];
    const input = rows(states, [2, 0, 0, 0, 0, 0, 0]);
    const first: FoodLogHistoryDay = summarizeFoodLogHistory(input.completeness, input.dailyTotals, DATES).days[0];
    expect(first).toMatchObject({ state: 'unconfirmed', occasionCount: 3, unconfirmedMeals: 2, outcome: 'needs_attention' });
  });

  it('rejects missing, duplicate, out-of-window, and mismatched response rows', () => {
    const input = rows();
    expect(() => summarizeFoodLogHistory(input.completeness.slice(1), input.dailyTotals, DATES)).toThrow();
    expect(() => summarizeFoodLogHistory([...input.completeness, input.completeness[0]], input.dailyTotals, DATES)).toThrow();
    expect(() => summarizeFoodLogHistory(input.completeness.map((row, i) => i === 0 ? { ...row, date: '2026-03-08' } : row), input.dailyTotals, DATES)).toThrow();
    expect(() => summarizeFoodLogHistory(input.completeness, input.dailyTotals.slice(1), DATES)).toThrow();
  });

  it('rejects malformed expected windows', () => {
    const input = rows();
    expect(() => summarizeFoodLogHistory(input.completeness, input.dailyTotals, [...DATES.slice(0, 6), '2026-03-09'])).toThrow();
    expect(() => summarizeFoodLogHistory(input.completeness, input.dailyTotals, [...DATES].reverse())).not.toThrow();
  });
});

describe('OUTCOME_COLOR', () => {
  it('gives every outcome its own color', () => {
    const colors = Object.values(OUTCOME_COLOR);
    expect(new Set(colors).size).toBe(3);
  });
});
