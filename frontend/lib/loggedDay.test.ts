import { describe, expect, it } from 'vitest';
import { loggedDayKey, loggedDayLabel } from './loggedDay';

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

describe('loggedDayLabel', () => {
  // Same instant as loggedDayKey's zone-shift case above: the label must
  // name the same calendar day the grouping key computes, not the day the
  // instant falls on in some other zone (e.g. the test runner's own TZ).
  it('names the same day as loggedDayKey for the same zone', () => {
    const d = new Date('2026-08-21T02:00:00Z');
    expect(loggedDayLabel(d, undefined, 'America/Los_Angeles')).toContain('20');
    expect(loggedDayLabel(d, undefined, 'America/Los_Angeles')).not.toContain('21');
  });

  it('falls back to UTC for an invalid/unsupported zone name', () => {
    const d = new Date('2026-08-21T02:00:00Z');
    expect(loggedDayLabel(d, undefined, 'Not/AZone')).toBe(loggedDayLabel(d, undefined, 'UTC'));
  });
});
