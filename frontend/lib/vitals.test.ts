import { describe, expect, it } from 'vitest';
import { DATA_TYPES } from './api';
import { PRIMARY_METRICS, hasCardPresence, reconcileMetricOrder, secondaryTypes, extractVital } from './vitals';

describe('extractVital asOf', () => {
  it('is set from the last bucket for a cumulative type (steps)', () => {
    const rows = [
      { bucket_start: '2026-03-14T00:00:00Z', sum: 5000 },
      { bucket_start: '2026-03-15T00:00:00Z', sum: 6000 },
    ];
    const result = extractVital('steps', rows);
    expect(result?.asOf).toBe('2026-03-15T00:00:00Z');
  });

  it('is set from the last bucket for a point type (heart_rate)', () => {
    const rows = [
      { bucket_start: '2026-03-14T00:00:00Z', avg: 60 },
      { bucket_start: '2026-03-15T00:00:00Z', avg: 65 },
    ];
    const result = extractVital('heart_rate', rows);
    expect(result?.asOf).toBe('2026-03-15T00:00:00Z');
  });

  it('is absent when the last row carries no bucket_start', () => {
    const rows = [{ sum: 5000 }];
    const result = extractVital('steps', rows);
    expect(result?.asOf).toBeUndefined();
  });
});

describe('reconcileMetricOrder with logging_gap', () => {
  it('reorders a saved order that includes logging_gap', () => {
    const saved = [
      { type: 'logging_gap', hidden: false },
      { type: 'weight', hidden: false },
      { type: 'steps', hidden: false },
      { type: 'heart_rate', hidden: false },
      { type: 'sleep', hidden: false },
      { type: 'heart_rate_variability', hidden: false },
      { type: 'distance', hidden: false },
      { type: 'blood_pressure', hidden: false },
      { type: 'oxygen_saturation', hidden: false },
    ];
    const result = reconcileMetricOrder(saved);
    expect(result.map(m => m.type)).toEqual([
      'logging_gap', 'weight', 'steps', 'heart_rate', 'sleep',
      'heart_rate_variability', 'distance', 'blood_pressure', 'oxygen_saturation',
    ]);
  });

  it('appends logging_gap visible at the default position for a saved order that predates it', () => {
    // A saved order from before this metric existed: every current
    // PRIMARY_METRICS type except logging_gap.
    const saved = PRIMARY_METRICS.filter(m => m.type !== 'logging_gap').map(m => ({ type: m.type, hidden: false }));
    const result = reconcileMetricOrder(saved);
    const loggingGapEntry = result.find(m => m.type === 'logging_gap');
    expect(loggingGapEntry).toEqual({ type: 'logging_gap', hidden: false });
    // Appended at the end, same as any other newly-added metric would be.
    expect(result[result.length - 1].type).toBe('logging_gap');
  });

  it('persists a hidden logging_gap entry across reconciliation', () => {
    const saved = PRIMARY_METRICS.map(m => ({ type: m.type, hidden: m.type === 'logging_gap' }));
    const result = reconcileMetricOrder(saved);
    const loggingGapEntry = result.find(m => m.type === 'logging_gap');
    expect(loggingGapEntry?.hidden).toBe(true);
  });
});

describe('secondaryTypes', () => {
  it('never includes logging_gap, since it is not a member of the DataType list passed in', () => {
    expect(secondaryTypes(DATA_TYPES)).not.toContain('logging_gap');
  });

  it('excludes every PRIMARY_METRICS DataType and keeps every other DataType', () => {
    const result = secondaryTypes(DATA_TYPES);
    const primaryDataTypes = new Set(PRIMARY_METRICS.map(m => m.type));
    for (const type of DATA_TYPES) {
      if (primaryDataTypes.has(type)) {
        expect(result).not.toContain(type);
      } else {
        expect(result).toContain(type);
      }
    }
  });
});

describe('hasCardPresence', () => {
  it('is always true for logging_gap when the presence map omits it', () => {
    expect(hasCardPresence({}, 'logging_gap')).toBe(true);
  });

  it('is always true for logging_gap even when the presence map explicitly returns false for it', () => {
    expect(hasCardPresence({ logging_gap: false }, 'logging_gap')).toBe(true);
  });

  it('is always true for logging_gap when the presence map itself is null (fetch failed)', () => {
    expect(hasCardPresence(null, 'logging_gap')).toBe(true);
  });

  it('delegates to the ordinary presence rules for a real DataType', () => {
    expect(hasCardPresence({ weight: false }, 'weight')).toBe(false);
    expect(hasCardPresence({ weight: true }, 'weight')).toBe(true);
    expect(hasCardPresence({}, 'weight')).toBe(true);
  });
});
