import { describe, expect, it } from 'vitest';
import {
  checkHardFloor,
  computeLoggingGap,
  excludedOutlierCount,
  LOGGING_GAP_WINDOW_DAYS,
  rejectOutliers,
  resolveLoggingGapWindow,
  slopeStandardError,
  type DayValueRecord,
  type DayWindowData,
} from './loggingGap';
import { toDayOffset } from './dataTypeMeta';

describe('rejectOutliers', () => {
  it('passes through an empty input unchanged', () => {
    expect(rejectOutliers([])).toEqual({ kept: [], rejected: [], bootstrapSiblingAmbiguous: false });
  });

  it('passes through a single-record input unchanged', () => {
    const result = rejectOutliers([{ day: 10, value: 80 }]);
    expect(result.kept).toEqual([{ day: 10, value: 80 }]);
    expect(result.rejected).toEqual([]);
    expect(result.bootstrapSiblingAmbiguous).toBe(false);
  });

  it('excludes a single implausible mid-series reading without poisoning the next comparison', () => {
    const records: DayValueRecord[] = [
      { day: 0, value: 91.0 },
      { day: 1, value: 91.2 },
      { day: 2, value: 75.0 },
      { day: 3, value: 91.4 },
    ];
    const result = rejectOutliers(records);
    expect(result.rejected).toEqual([{ day: 2, value: 75.0 }]);
    expect(result.kept).toEqual([
      { day: 0, value: 91.0 },
      { day: 1, value: 91.2 },
      { day: 3, value: 91.4 },
    ]);
    expect(result.bootstrapSiblingAmbiguous).toBe(false);
  });

  it('never rejects a gradual multi-day change at or under 2 kg/day', () => {
    const records: DayValueRecord[] = [
      { day: 0, value: 90.0 },
      { day: 1, value: 89.7 },
      { day: 2, value: 89.4 },
      { day: 3, value: 89.1 },
    ];
    const result = rejectOutliers(records);
    expect(result.rejected).toEqual([]);
    expect(result.kept).toHaveLength(4);
    expect(result.bootstrapSiblingAmbiguous).toBe(false);
  });

  it('rejects a bad leading reading during bootstrap without poisoning subsequent good readings', () => {
    const records: DayValueRecord[] = [
      { day: 0, value: 75.0 },
      { day: 1, value: 91.0 },
      { day: 6, value: 92.0 },
      { day: 9, value: 91.5 },
    ];
    const result = rejectOutliers(records);
    expect(result.rejected).toEqual([{ day: 0, value: 75.0 }]);
    expect(result.kept).toEqual([
      { day: 1, value: 91.0 },
      { day: 6, value: 92.0 },
      { day: 9, value: 91.5 },
    ]);
    expect(result.bootstrapSiblingAmbiguous).toBe(false);
  });

  it('never rejects two same-day records against each other', () => {
    const records: DayValueRecord[] = [
      { day: 0, value: 91.0 },
      { day: 0, value: 91.4 },
    ];
    const result = rejectOutliers(records);
    expect(result.rejected).toEqual([]);
    expect(result.kept).toHaveLength(2);
    expect(result.bootstrapSiblingAmbiguous).toBe(false);
  });

  it('skips a same-day pair among the first two records and continues bootstrap against the original candidate', () => {
    const records: DayValueRecord[] = [
      { day: 0, value: 91.0 },
      { day: 0, value: 95.0 },
      { day: 1, value: 91.2 },
    ];
    // If the bootstrap wrongly advanced its candidate to the same-day sibling (95.0), 91.2 would
    // be rejected (rate 3.8 > 2). It should instead still be rate-checked against 91.0 and kept.
    const result = rejectOutliers(records);
    expect(result.rejected).toEqual([]);
    expect(result.kept).toEqual(records);
    expect(result.bootstrapSiblingAmbiguous).toBe(false);
  });

  it('never lets a same-day-exempted record become the reference for a later comparison', () => {
    const records: DayValueRecord[] = [
      { day: 10, value: 91.0 },
      { day: 11, value: 91.2 },
      { day: 11, value: 95.0 },
      { day: 12, value: 91.6 },
    ];
    // If day 12's record were wrongly rate-checked against the same-day sibling (95.0) instead of
    // the record that actually passed a rate check (91.2), it would be rejected (rate 3.4 > 2).
    const result = rejectOutliers(records);
    expect(result.rejected).toEqual([]);
    expect(result.kept).toEqual(records);
    expect(result.bootstrapSiblingAmbiguous).toBe(false);
  });

  it('anchors a symmetric two-cluster leading sequence on the first agreeing pair and rejects both outliers', () => {
    const records: DayValueRecord[] = [
      { day: 0, value: 91.0 },
      { day: 1, value: 75.0 },
      { day: 2, value: 75.2 },
      { day: 3, value: 91.2 },
    ];
    const result = rejectOutliers(records);
    expect(result.kept).toEqual([
      { day: 1, value: 75.0 },
      { day: 2, value: 75.2 },
    ]);
    expect(result.rejected).toEqual([
      { day: 0, value: 91.0 },
      { day: 3, value: 91.2 },
    ]);
    expect(result.bootstrapSiblingAmbiguous).toBe(false);
  });

  it('flags bootstrapSiblingAmbiguous when a bootstrap rejection has a same-day sibling, rejecting only the candidate', () => {
    const records: DayValueRecord[] = [
      { day: 1, value: 75.0 },
      { day: 1, value: 75.2 },
      { day: 2, value: 91.0 },
      { day: 3, value: 91.2 },
    ];
    const result = rejectOutliers(records);
    expect(result.rejected).toEqual([{ day: 1, value: 75.0 }]);
    expect(result.kept).toEqual([
      { day: 1, value: 75.2 },
      { day: 2, value: 91.0 },
      { day: 3, value: 91.2 },
    ]);
    expect(result.bootstrapSiblingAmbiguous).toBe(true);
  });

  it('does not flag bootstrapSiblingAmbiguous for an ordinary same-day kept/rejected overlap unrelated to the bootstrap', () => {
    const records: DayValueRecord[] = [
      { day: 0, value: 90.9 },
      { day: 1, value: 91.0 },
      { day: 2, value: 94.0 },
      { day: 2, value: 91.2 },
    ];
    const result = rejectOutliers(records);
    expect(result.rejected).toEqual([{ day: 2, value: 94.0 }]);
    expect(result.kept).toEqual([
      { day: 0, value: 90.9 },
      { day: 1, value: 91.0 },
      { day: 2, value: 91.2 },
    ]);
    expect(result.bootstrapSiblingAmbiguous).toBe(false);
  });

  it('keeps a candidate at exactly the 2.0 kg/day rate cap (boundary, not >)', () => {
    const records: DayValueRecord[] = [
      { day: 0, value: 90.0 },
      { day: 1, value: 92.0 },
    ];
    const result = rejectOutliers(records);
    expect(result.rejected).toEqual([]);
    expect(result.kept).toEqual(records);
  });
});

describe('slopeStandardError', () => {
  it('returns a hand-computed standard error for a known small dataset', () => {
    // x=[0,1,2], y=[0,1,3]: slope=1.5, intercept=-1/6 (see loggingGap.test.ts derivation);
    // SE(slope) reduces to the exact closed form 1/sqrt(12).
    const result = slopeStandardError(
      [
        { x: 0, y: 0 },
        { x: 1, y: 1 },
        { x: 2, y: 3 },
      ],
      1.5,
      -1 / 6
    );
    expect(result).not.toBeNull();
    expect(result!).toBeCloseTo(1 / Math.sqrt(12), 8);
  });

  it('returns null for exactly 2 points', () => {
    const result = slopeStandardError(
      [
        { x: 0, y: 0 },
        { x: 1, y: 2 },
      ],
      2,
      0
    );
    expect(result).toBeNull();
  });

  it('returns a defined value for exactly 3 points', () => {
    const result = slopeStandardError(
      [
        { x: 0, y: 0 },
        { x: 1, y: 1 },
        { x: 2, y: 3 },
      ],
      1.5,
      -1 / 6
    );
    expect(result).not.toBeNull();
  });

  it('does not throw or divide by zero unexpectedly for a perfectly linear n>=3 input', () => {
    const result = slopeStandardError(
      [
        { x: 0, y: 0 },
        { x: 1, y: 2 },
        { x: 2, y: 4 },
      ],
      2,
      0
    );
    expect(result).toBeCloseTo(0, 10);
  });
});

describe('resolveLoggingGapWindow', () => {
  it('ends the window at yesterday regardless of time-of-day', () => {
    const morning = resolveLoggingGapWindow(new Date('2026-08-26T00:00:01Z'), 'UTC');
    const night = resolveLoggingGapWindow(new Date('2026-08-26T23:59:59Z'), 'UTC');
    expect(morning.windowEnd).toBe('2026-08-25');
    expect(night.windowEnd).toBe('2026-08-25');
  });

  it('spans exactly 28 calendar days ending yesterday, matching the spec example', () => {
    const window = resolveLoggingGapWindow(new Date('2026-08-26T12:00:00Z'), 'UTC');
    expect(window.windowStart).toBe('2026-07-29');
    expect(window.windowEnd).toBe('2026-08-25');
    expect(window.windowLastDayOffset - window.windowStartDayOffset).toBe(LOGGING_GAP_WINDOW_DAYS - 1);
  });

  it('derives a lead-in range wider than the visible 28-day window', () => {
    const window = resolveLoggingGapWindow(new Date('2026-08-26T12:00:00Z'), 'UTC');
    expect(window.leadInStart < window.windowStart).toBe(true);
  });

  it('leadInFetchFromUTC covers local midnight of leadInStart for a positive UTC offset', () => {
    // UTC+14 local midnight of leadInStart is the previous UTC calendar day at 10:00 —
    // literal `${leadInStart}T00:00:00.000Z` would start 14 hours too late and miss
    // early lead-in-day weigh-ins.
    const window = resolveLoggingGapWindow(new Date('2026-08-26T12:00:00Z'), 'Pacific/Kiritimati');
    const leadInLocalMidnightUTC = new Date(`${window.leadInStart}T00:00:00.000Z`);
    leadInLocalMidnightUTC.setUTCHours(leadInLocalMidnightUTC.getUTCHours() - 14);
    expect(new Date(window.leadInFetchFromUTC).getTime()).toBeLessThanOrEqual(leadInLocalMidnightUTC.getTime());
  });

  it('leadInFetchFromUTC covers local midnight of leadInStart for a negative UTC offset', () => {
    const window = resolveLoggingGapWindow(new Date('2026-08-26T12:00:00Z'), 'Pacific/Honolulu');
    const leadInLocalMidnightUTC = new Date(`${window.leadInStart}T00:00:00.000Z`);
    leadInLocalMidnightUTC.setUTCHours(leadInLocalMidnightUTC.getUTCHours() + 10);
    expect(new Date(window.leadInFetchFromUTC).getTime()).toBeLessThanOrEqual(leadInLocalMidnightUTC.getTime());
  });

  it('leadInStartDayOffset matches the Logged Day offset of leadInStart', () => {
    const window = resolveLoggingGapWindow(new Date('2026-08-26T12:00:00Z'), 'UTC');
    expect(window.leadInStartDayOffset).toBe(toDayOffset(window.leadInStart));
  });
});

describe('checkHardFloor', () => {
  const windowStartDayOffset = 100;
  const windowLastDayOffset = 127;

  function validPerDayWindowData(validDayCount: number): Record<number, DayWindowData> {
    const data: Record<number, DayWindowData> = {};
    for (let i = 0; i < validDayCount; i++) {
      data[windowStartDayOffset + i] = { state: 'complete', calories: 2000 };
    }
    return data;
  }

  const safeKept: DayValueRecord[] = [
    { day: 120, value: 80 },
    { day: 123, value: 80.2 },
  ];
  const safeRejected: DayValueRecord[] = [];
  const safeValidDays = validPerDayWindowData(5);
  const safeMostRecentKeptDayOffset = 125;

  it('fires when fewer than 2 raw weigh-ins survive', () => {
    const result = checkHardFloor(
      [{ day: 120, value: 80 }],
      safeRejected,
      false,
      windowStartDayOffset,
      safeValidDays,
      120,
      windowLastDayOffset
    );
    expect(result).toBe(true);
  });

  it('does not fire on exactly 2 raw survivors sharing a calendar day', () => {
    const result = checkHardFloor(
      [
        { day: 120, value: 80 },
        { day: 120, value: 80.2 },
      ],
      safeRejected,
      false,
      windowStartDayOffset,
      safeValidDays,
      120,
      windowLastDayOffset
    );
    expect(result).toBe(false);
  });

  it.each([0, 1, 2])('fires when only %i valid days are in the window', validDayCount => {
    const result = checkHardFloor(
      safeKept,
      safeRejected,
      false,
      windowStartDayOffset,
      validPerDayWindowData(validDayCount),
      safeMostRecentKeptDayOffset,
      windowLastDayOffset
    );
    expect(result).toBe(true);
  });

  it('does not fire with exactly 3 valid days', () => {
    const result = checkHardFloor(
      safeKept,
      safeRejected,
      false,
      windowStartDayOffset,
      validPerDayWindowData(3),
      safeMostRecentKeptDayOffset,
      windowLastDayOffset
    );
    expect(result).toBe(false);
  });

  it('does not fire when the most recent kept weigh-in is exactly 7 days stale', () => {
    const result = checkHardFloor(
      safeKept,
      safeRejected,
      false,
      windowStartDayOffset,
      safeValidDays,
      windowLastDayOffset - 7,
      windowLastDayOffset
    );
    expect(result).toBe(false);
  });

  it('fires when the most recent kept weigh-in is 8 days stale', () => {
    const result = checkHardFloor(
      safeKept,
      safeRejected,
      false,
      windowStartDayOffset,
      safeValidDays,
      windowLastDayOffset - 8,
      windowLastDayOffset
    );
    expect(result).toBe(true);
  });

  it('does not fire with exactly 3 rejected weigh-ins in the window', () => {
    const rejected: DayValueRecord[] = [
      { day: 105, value: 75 },
      { day: 108, value: 75.1 },
      { day: 111, value: 75.2 },
    ];
    const result = checkHardFloor(
      safeKept,
      rejected,
      false,
      windowStartDayOffset,
      safeValidDays,
      safeMostRecentKeptDayOffset,
      windowLastDayOffset
    );
    expect(result).toBe(false);
  });

  it('fires via the rejection cap alone with exactly 4 rejected weigh-ins, even with an otherwise-passing surviving cluster', () => {
    const rejected: DayValueRecord[] = [
      { day: 105, value: 75 },
      { day: 108, value: 75.1 },
      { day: 111, value: 75.2 },
      { day: 114, value: 75.3 },
    ];
    const result = checkHardFloor(
      safeKept,
      rejected,
      false,
      windowStartDayOffset,
      safeValidDays,
      safeMostRecentKeptDayOffset,
      windowLastDayOffset
    );
    expect(result).toBe(true);
  });

  it('fires via the same-day-sibling suppression flag alone, even when kept/rejected counts are both safely under their own caps', () => {
    const result = checkHardFloor(
      [
        { day: 120, value: 80 },
        { day: 122, value: 80.1 },
        { day: 124, value: 80.2 },
      ],
      [{ day: 121, value: 95 }],
      true,
      windowStartDayOffset,
      safeValidDays,
      safeMostRecentKeptDayOffset,
      windowLastDayOffset
    );
    expect(result).toBe(true);
  });

  it('fires via the same-day-sibling suppression flag even when the rejected candidate and its sibling are outside the window-filtered kept/rejected arrays', () => {
    // rejectOutliers' own bootstrapSiblingAmbiguous is sourced from the whole lead-in-extended
    // walk (see the 'flags bootstrapSiblingAmbiguous' rejectOutliers test above, days 1-3),
    // entirely outside this window ([100, 127]) — the caller passes it through unfiltered while
    // kept/rejected are independently window-filtered and would otherwise pass every other gate.
    const { bootstrapSiblingAmbiguous } = rejectOutliers([
      { day: 1, value: 75.0 },
      { day: 1, value: 75.2 },
      { day: 2, value: 91.0 },
      { day: 3, value: 91.2 },
    ]);
    expect(bootstrapSiblingAmbiguous).toBe(true);
    const result = checkHardFloor(
      safeKept,
      safeRejected,
      bootstrapSiblingAmbiguous,
      windowStartDayOffset,
      safeValidDays,
      safeMostRecentKeptDayOffset,
      windowLastDayOffset
    );
    expect(result).toBe(true);
  });

  it('does not fire for an ordinary same-day kept/rejected overlap unrelated to the bootstrap, proving array overlap alone is not the trigger', () => {
    const smallWindowStart = 0;
    const smallWindowLast = 9;
    const { kept, rejected, bootstrapSiblingAmbiguous } = rejectOutliers([
      { day: 0, value: 90.9 },
      { day: 1, value: 91.0 },
      { day: 2, value: 94.0 },
      { day: 2, value: 91.2 },
    ]);
    expect(bootstrapSiblingAmbiguous).toBe(false);
    expect(kept.some(r => r.day === 2)).toBe(true);
    expect(rejected.some(r => r.day === 2)).toBe(true);

    const perDayWindowData: Record<number, DayWindowData> = {
      0: { state: 'complete', calories: 2000 },
      1: { state: 'complete', calories: 2000 },
      2: { state: 'complete', calories: 2000 },
    };
    const mostRecentKeptDayOffset = Math.max(...kept.map(r => r.day));
    const result = checkHardFloor(
      kept,
      rejected,
      bootstrapSiblingAmbiguous,
      smallWindowStart,
      perDayWindowData,
      mostRecentKeptDayOffset,
      smallWindowLast
    );
    expect(result).toBe(false);
  });
});

describe('computeLoggingGap', () => {
  function perDayWindowData(entries: DayWindowData[]): Record<number, DayWindowData> {
    const data: Record<number, DayWindowData> = {};
    entries.forEach((entry, i) => {
      data[i] = entry;
    });
    return data;
  }

  it('reports not_enough_data when the interval covers zero', () => {
    const result = computeLoggingGap(
      { slope: 0, intercept: 0 },
      0,
      3100,
      perDayWindowData([
        { state: 'complete', calories: 2950 },
        { state: 'complete', calories: 2950 },
        { state: 'complete', calories: 2950 },
      ])
    );
    expect(result).toEqual({ kind: 'not_enough_data' });
  });

  it('reports a gap clearly outside the interval', () => {
    const result = computeLoggingGap(
      { slope: 0, intercept: 0 },
      0,
      3100,
      perDayWindowData([
        { state: 'complete', calories: 2200 },
        { state: 'complete', calories: 2200 },
        { state: 'complete', calories: 2200 },
      ])
    );
    expect(result.kind).toBe('gap');
    if (result.kind === 'gap') {
      expect(result.value).toBeCloseTo(900, 6);
      expect(result.interval).toBeCloseTo(310, 6);
    }
  });

  it('suppresses a gap exactly equal to its interval (boundary, <=)', () => {
    const result = computeLoggingGap(
      { slope: 0, intercept: 0 },
      0,
      3100,
      perDayWindowData([
        { state: 'complete', calories: 2790 },
        { state: 'complete', calories: 2790 },
        { state: 'complete', calories: 2790 },
      ])
    );
    expect(result).toEqual({ kind: 'not_enough_data' });
  });

  it('reports a negative gap when logged intake exceeds implied intake', () => {
    const result = computeLoggingGap(
      { slope: 0, intercept: 0 },
      0,
      3100,
      perDayWindowData([
        { state: 'complete', calories: 4000 },
        { state: 'complete', calories: 4000 },
        { state: 'complete', calories: 4000 },
      ])
    );
    expect(result.kind).toBe('gap');
    if (result.kind === 'gap') {
      expect(result.value).toBeCloseTo(-900, 6);
      expect(result.interval).toBeCloseTo(310, 6);
    }
  });

  it('folds a non-zero weight-trend slope into Implied Intake via KCAL_PER_KG', () => {
    // slope -0.2 kg/day of weight loss implies eating 0.2 * 7700 = 1540 kcal/day less than the
    // nutrition target: impliedIntake = 3100 - 1540 = 1560. Mean Logged Intake of 1200 gives a
    // hand-computed value of 360, clearly outside the 310 formula-error-only interval.
    const result = computeLoggingGap(
      { slope: -0.2, intercept: 80 },
      0,
      3100,
      perDayWindowData([
        { state: 'complete', calories: 1200 },
        { state: 'complete', calories: 1200 },
        { state: 'complete', calories: 1200 },
      ])
    );
    expect(result.kind).toBe('gap');
    if (result.kind === 'gap') {
      expect(result.value).toBeCloseTo(360, 6);
      expect(result.interval).toBeCloseTo(310, 6);
    }
  });

  it('excludes unconfirmed/incomplete days from Mean Logged Intake rather than counting them as zero', () => {
    const result = computeLoggingGap(
      { slope: 0, intercept: 0 },
      0,
      3100,
      perDayWindowData([
        { state: 'complete', calories: 2200 },
        { state: 'complete', calories: 2200 },
        { state: 'complete', calories: 2200 },
        { state: 'unconfirmed', calories: 0 },
        { state: 'incomplete', calories: 0 },
      ])
    );
    expect(result.kind).toBe('gap');
    if (result.kind === 'gap') {
      // Mean Logged Intake is still 2200 (average of the 3 valid days only) — if the two
      // unconfirmed/incomplete days had been averaged in as zero-intake days, the value would be
      // higher than 900.
      expect(result.value).toBeCloseTo(900, 6);
    }
  });

  it('reports not_enough_data when se is null, regardless of value', () => {
    const result = computeLoggingGap(
      { slope: 0, intercept: 0 },
      null,
      3100,
      perDayWindowData([
        { state: 'complete', calories: 2200 },
        { state: 'complete', calories: 2200 },
        { state: 'complete', calories: 2200 },
      ])
    );
    expect(result).toEqual({ kind: 'not_enough_data' });
  });
});

describe('excludedOutlierCount', () => {
  it('counts only rejections within the window, excluding the lead-in extension', () => {
    const rejected: DayValueRecord[] = [
      { day: 50, value: 75 }, // lead-in, before the window
      { day: 105, value: 75 },
      { day: 110, value: 75 },
      { day: 200, value: 75 }, // after the window
    ];
    expect(excludedOutlierCount(rejected, 100, 127)).toBe(2);
  });

  it('returns 0 for no rejections', () => {
    expect(excludedOutlierCount([], 100, 127)).toBe(0);
  });
});
