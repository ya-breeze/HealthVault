import { describe, expect, it } from 'vitest';
import { loggedDayKey } from './loggedDay';

describe('loggedDayKey', () => {
  it('defaults to UTC when no timezone is given', () => {
    expect(loggedDayKey(new Date('2026-08-21T02:00:00Z'), undefined)).toBe('2026-08-21');
  });

  it('shifts the day for a zone behind UTC', () => {
    // design.md's own example: 2026-08-21T02:00:00Z is still 2026-08-20
    // evening in America/Los_Angeles (UTC-7 in August, DST).
    expect(loggedDayKey(new Date('2026-08-21T02:00:00Z'), 'America/Los_Angeles')).toBe('2026-08-20');
  });

  it('falls back to UTC for an invalid/unsupported zone name', () => {
    expect(loggedDayKey(new Date('2026-08-21T02:00:00Z'), 'Not/AZone')).toBe('2026-08-21');
  });

  it('falls back to UTC for an empty string zone', () => {
    expect(loggedDayKey(new Date('2026-08-21T02:00:00Z'), '')).toBe('2026-08-21');
  });
});
