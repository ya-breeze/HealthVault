import { describe, expect, it } from 'vitest';
import { splitRangeIntoWindows } from './completeness';

describe('splitRangeIntoWindows', () => {
  it('returns a single window for a range within the cap', () => {
    expect(splitRangeIntoWindows('2026-08-01', '2026-08-10')).toEqual([
      { from: '2026-08-01', to: '2026-08-10' },
    ]);
  });

  it('returns a single window for a range exactly at the cap', () => {
    // 2026-01-01..2026-04-02 is exactly 92 inclusive days.
    const windows = splitRangeIntoWindows('2026-01-01', '2026-04-02');
    expect(windows).toEqual([{ from: '2026-01-01', to: '2026-04-02' }]);
  });

  it('splits a range one day over the cap into two consecutive windows', () => {
    // 2026-01-01..2026-04-03 is 93 inclusive days.
    const windows = splitRangeIntoWindows('2026-01-01', '2026-04-03');
    expect(windows).toEqual([
      { from: '2026-01-01', to: '2026-04-02' },
      { from: '2026-04-03', to: '2026-04-03' },
    ]);
  });

  it('splits a much longer range into several consecutive ≤92-day windows with no gaps or overlaps', () => {
    const windows = splitRangeIntoWindows('2025-01-01', '2026-08-21', 92);
    expect(windows.length).toBeGreaterThan(1);
    for (const w of windows) expect(w.from <= w.to).toBe(true);
    for (let i = 1; i < windows.length; i++) {
      // Consecutive: the next window starts exactly one day after the previous ends.
      const prevEnd = new Date(`${windows[i - 1].to}T00:00:00Z`);
      prevEnd.setUTCDate(prevEnd.getUTCDate() + 1);
      expect(windows[i].from).toBe(prevEnd.toISOString().slice(0, 10));
    }
    expect(windows[0].from).toBe('2025-01-01');
    expect(windows[windows.length - 1].to).toBe('2026-08-21');
  });

  it('returns a single-day window when from equals to', () => {
    expect(splitRangeIntoWindows('2026-08-21', '2026-08-21')).toEqual([
      { from: '2026-08-21', to: '2026-08-21' },
    ]);
  });

  it('returns no windows when from is after to', () => {
    expect(splitRangeIntoWindows('2026-08-22', '2026-08-21')).toEqual([]);
  });
});
