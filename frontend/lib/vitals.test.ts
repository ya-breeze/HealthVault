import { describe, expect, it } from 'vitest';
import { DATA_TYPES } from './api';
import { PRIMARY_METRICS, hasCardPresence, isDataTypeCard, reconcileMetricOrder, secondaryTypes, extractVital } from './vitals';

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

describe('reconcileMetricOrder with Food Cards', () => {
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
      'food_log_history',
    ]);
  });

  it('appends both Food Cards visible for a saved order that predates them', () => {
    // A saved order from before either Food Card existed: every current
    // PRIMARY_METRICS type except the two Food Cards.
    const saved = PRIMARY_METRICS.filter(m => isDataTypeCard(m.type)).map(m => ({ type: m.type, hidden: false }));
    const result = reconcileMetricOrder(saved);
    expect(result.slice(-2)).toEqual([
      { type: 'logging_gap', hidden: false },
      { type: 'food_log_history', hidden: false },
    ]);
  });

  it('persists explicit reorder and hide choices for both Food Cards', () => {
    const saved = [
      { type: 'food_log_history', hidden: true },
      { type: 'logging_gap', hidden: false },
      ...PRIMARY_METRICS.filter(m => isDataTypeCard(m.type)).map(m => ({ type: m.type, hidden: false })),
    ];
    const result = reconcileMetricOrder(saved);
    expect(result.slice(0, 2)).toEqual([
      { type: 'food_log_history', hidden: true },
      { type: 'logging_gap', hidden: false },
    ]);
  });
});

describe('secondaryTypes', () => {
  it('never includes Food Cards, since they are not members of the DataType list passed in', () => {
    expect(secondaryTypes(DATA_TYPES)).not.toContain('logging_gap');
    expect(secondaryTypes(DATA_TYPES)).not.toContain('food_log_history');
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
  it('is always true for both Food Cards when the presence map omits them', () => {
    expect(hasCardPresence({}, 'logging_gap')).toBe(true);
    expect(hasCardPresence({}, 'food_log_history')).toBe(true);
  });

  it('is always true for both Food Cards even when the presence map says false', () => {
    expect(hasCardPresence({ logging_gap: false }, 'logging_gap')).toBe(true);
    expect(hasCardPresence({ food_log_history: false }, 'food_log_history')).toBe(true);
  });

  it('is always true for both Food Cards when the presence map itself is null', () => {
    expect(hasCardPresence(null, 'logging_gap')).toBe(true);
    expect(hasCardPresence(null, 'food_log_history')).toBe(true);
  });

  it('delegates to the ordinary presence rules for a real DataType', () => {
    expect(hasCardPresence({ weight: false }, 'weight')).toBe(false);
    expect(hasCardPresence({ weight: true }, 'weight')).toBe(true);
    expect(hasCardPresence({}, 'weight')).toBe(true);
  });
});

describe('isDataTypeCard', () => {
  it('filters only the two non-DataType Food Card ids', () => {
    expect(isDataTypeCard('steps')).toBe(true);
    expect(isDataTypeCard('logging_gap')).toBe(false);
    expect(isDataTypeCard('food_log_history')).toBe(false);
  });
});
