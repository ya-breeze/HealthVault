import type { DailyTotal, DayCompleteness, DayCompletenessState } from './api';
import { isValidDay } from './loggingGap';
import { loggedDayKey } from './loggedDay';

export const FOOD_LOG_HISTORY_WINDOW_DAYS = 7;

export interface FoodLogHistoryWindow {
  /** Inclusive first completed Logged Day, YYYY-MM-DD. */
  from: string;
  /** Inclusive last completed Logged Day, YYYY-MM-DD (yesterday). */
  to: string;
}

export type FoodLogHistoryOutcome = 'counted' | 'no_food' | 'needs_attention';

/** Theme color (a CSS variable defined in globals.css) that marks each outcome. */
export const OUTCOME_COLOR: Record<FoodLogHistoryOutcome, string> = {
  counted: 'var(--outcome-counted)',
  no_food: 'var(--outcome-empty)',
  needs_attention: 'var(--outcome-attention)',
};

export interface FoodLogHistoryDay {
  date: string;
  state: DayCompletenessState;
  occasionCount: number;
  unconfirmedMeals: number;
  outcome: FoodLogHistoryOutcome;
}

export interface FoodLogHistorySummary {
  days: FoodLogHistoryDay[];
  countedDays: number;
  noFoodDays: number;
  needsAttentionDays: number;
}

function parseISODate(date: string): Date {
  return new Date(`${date}T00:00:00.000Z`);
}

function formatISODate(date: Date): string {
  return date.toISOString().slice(0, 10);
}

export function addDays(date: string, amount: number): string {
  const parsed = parseISODate(date);
  if (Number.isNaN(parsed.getTime())) throw new Error(`Invalid calendar date: ${date}`);
  parsed.setUTCDate(parsed.getUTCDate() + amount);
  return formatISODate(parsed);
}

function isCalendarDate(value: unknown): value is string {
  if (typeof value !== 'string' || !/^\d{4}-\d{2}-\d{2}$/.test(value)) return false;
  const parsed = parseISODate(value);
  return !Number.isNaN(parsed.getTime()) && formatISODate(parsed) === value;
}

/**
 * Resolves the seven completed Logged Days ending yesterday in the stored
 * timezone. Date arithmetic is done from UTC calendar midnights after the
 * boundary date has been obtained with loggedDayKey, so DST changes cannot
 * shorten or lengthen the requested seven-date range.
 */
export function resolveFoodLogHistoryWindow(now: Date, timezone: string | undefined): FoodLogHistoryWindow {
  const today = loggedDayKey(now, timezone);
  const to = addDays(today, -1);
  return {
    from: addDays(to, -(FOOD_LOG_HISTORY_WINDOW_DAYS - 1)),
    to,
  };
}

function validateExpectedDates(expectedDates: readonly string[]): string[] {
  if (expectedDates.length !== FOOD_LOG_HISTORY_WINDOW_DAYS) {
    throw new Error(`Expected exactly ${FOOD_LOG_HISTORY_WINDOW_DAYS} dates`);
  }

  const sorted = [...expectedDates].sort();
  if (sorted.some(date => !isCalendarDate(date)) || new Set(sorted).size !== sorted.length) {
    throw new Error('Expected dates must be unique YYYY-MM-DD calendar dates');
  }
  for (let i = 1; i < sorted.length; i++) {
    if (sorted[i] !== addDays(sorted[i - 1], 1)) {
      throw new Error('Expected dates must form a consecutive seven-day window');
    }
  }
  return sorted;
}

function validateCompletenessRow(row: DayCompleteness): void {
  if (!isCalendarDate(row.date) || !Number.isInteger(row.occasion_count) || row.occasion_count < 0) {
    throw new Error('Malformed completeness response row');
  }
  if (
    row.state !== 'complete' &&
    row.state !== 'confirmed_complete' &&
    row.state !== 'unconfirmed' &&
    row.state !== 'incomplete'
  ) {
    throw new Error('Malformed completeness response state');
  }
}

function validateDailyTotalRow(row: DailyTotal): void {
  if (!isCalendarDate(row.date) || !Number.isInteger(row.unconfirmed_meals) || row.unconfirmed_meals < 0) {
    throw new Error('Malformed daily-totals response row');
  }
}

function indexRows<T extends { date: string }>(
  rows: readonly T[],
  expectedDates: readonly string[],
  validate: (row: T) => void,
  endpoint: string,
): Map<string, T> {
  const expected = new Set(expectedDates);
  const indexed = new Map<string, T>();
  for (const row of rows) {
    validate(row);
    if (!expected.has(row.date)) throw new Error(`${endpoint} response contains an out-of-window date`);
    if (indexed.has(row.date)) throw new Error(`${endpoint} response contains a duplicate date`);
    indexed.set(row.date, row);
  }
  if (indexed.size !== expected.size || expectedDates.some(date => !indexed.has(date))) {
    throw new Error(`${endpoint} response is missing an expected date`);
  }
  return indexed;
}

/**
 * Strictly joins the two zero-filled range responses. Missing data is not
 * interpreted as a no-food day: an incomplete or duplicate response is a
 * retrieval failure and the card must render its unavailable state instead.
 */
export function summarizeFoodLogHistory(
  completeness: readonly DayCompleteness[],
  dailyTotals: readonly DailyTotal[],
  expectedDates: readonly string[],
): FoodLogHistorySummary {
  const dates = validateExpectedDates(expectedDates);
  const completenessByDate = indexRows(completeness, dates, validateCompletenessRow, 'completeness');
  const totalsByDate = indexRows(dailyTotals, dates, validateDailyTotalRow, 'daily-totals');

  const days: FoodLogHistoryDay[] = dates.map(date => {
    const completenessRow = completenessByDate.get(date)!;
    const totalRow = totalsByDate.get(date)!;
    const dayForEligibility = {
      state: completenessRow.state,
      calories: totalRow.calories,
      unconfirmedMeals: totalRow.unconfirmed_meals,
    };

    let outcome: FoodLogHistoryOutcome;
    if (isValidDay(dayForEligibility)) {
      outcome = 'counted';
    } else if (completenessRow.state === 'incomplete' && completenessRow.occasion_count === 0 && totalRow.unconfirmed_meals === 0) {
      outcome = 'no_food';
    } else {
      outcome = 'needs_attention';
    }

    return {
      date,
      state: completenessRow.state,
      occasionCount: completenessRow.occasion_count,
      unconfirmedMeals: totalRow.unconfirmed_meals,
      outcome,
    };
  });

  return {
    days,
    countedDays: days.filter(day => day.outcome === 'counted').length,
    noFoodDays: days.filter(day => day.outcome === 'no_food').length,
    needsAttentionDays: days.filter(day => day.outcome === 'needs_attention').length,
  };
}
