import type { DataType } from '@/lib/api';
import { formatMetricValue, toDisplayUnit } from '@/lib/dataTypeMeta';

/** The 8 metrics shown as full vitals-grid cards on the dashboard, in display order. */
export const PRIMARY_METRICS: { type: DataType; label: string }[] = [
  { type: 'steps', label: 'Steps' },
  { type: 'heart_rate', label: 'Heart Rate' },
  { type: 'sleep', label: 'Sleep' },
  { type: 'heart_rate_variability', label: 'HRV' },
  { type: 'distance', label: 'Distance' },
  { type: 'weight', label: 'Weight' },
  { type: 'blood_pressure', label: 'Blood Pressure' },
  { type: 'oxygen_saturation', label: 'Oxygen Sat.' },
];

/**
 * Reconciles a saved dashboard_order (from user settings) against
 * PRIMARY_METRICS: known types are reordered to match the saved order,
 * unknown/removed types are dropped, and any current metric missing from the
 * saved order is appended at the end. Returns PRIMARY_METRICS unchanged when
 * there's no saved order yet (default order for a user who hasn't customized).
 */
export function reconcileMetricOrder(saved: string[] | undefined): typeof PRIMARY_METRICS {
  if (!Array.isArray(saved) || saved.length === 0) return PRIMARY_METRICS;
  const byType = new Map(PRIMARY_METRICS.map(m => [m.type as string, m]));
  const seen = new Set<string>();
  const ordered: typeof PRIMARY_METRICS = [];
  for (const t of saved) {
    const m = typeof t === 'string' ? byType.get(t) : undefined;
    if (m && !seen.has(m.type)) {
      seen.add(m.type);
      ordered.push(m);
    }
  }
  const missing = PRIMARY_METRICS.filter(m => !seen.has(m.type));
  return [...ordered, ...missing];
}

export interface VitalResult {
  value: string;
  unit?: string;
  /** Chronological, oldest first — one point per day-bucket returned. */
  sparkline: number[];
  trend: 'up' | 'down' | 'flat';
}

function num(v: unknown): number {
  return typeof v === 'number' ? v : Number(v ?? 0);
}

function trendFrom(series: number[]): 'up' | 'down' | 'flat' {
  if (series.length < 2) return 'flat';
  const last = series[series.length - 1];
  const prev = series[series.length - 2];
  if (last > prev) return 'up';
  if (last < prev) return 'down';
  return 'flat';
}

/**
 * Extracts a vitals-grid card's display value, sparkline, and trend from a
 * ?bucket=day response, per the type's aggregation family (see
 * chart-zoom-aggregation spec). Returns null when there's no data — the
 * card renders a "no data" state rather than an empty/broken sparkline.
 */
export function extractVital(type: DataType, rows: Record<string, unknown>[]): VitalResult | null {
  if (rows.length === 0) return null;

  switch (type) {
    case 'steps': {
      const series = rows.map(r => num(r.sum));
      return { value: formatMetricValue('steps', series[series.length - 1]), sparkline: series, trend: trendFrom(series) };
    }
    case 'distance': {
      const series = rows.map(r => toDisplayUnit('distance', num(r.sum)));
      return { value: formatMetricValue('distance', series[series.length - 1]), unit: 'km', sparkline: series, trend: trendFrom(series) };
    }
    case 'sleep': {
      const series = rows.map(r => toDisplayUnit('sleep', num(r.sum)));
      return { value: formatMetricValue('sleep', series[series.length - 1]), unit: 'h', sparkline: series, trend: trendFrom(series) };
    }
    case 'heart_rate': {
      const series = rows.map(r => num(r.avg));
      return { value: formatMetricValue('heart_rate', series[series.length - 1]), unit: 'bpm', sparkline: series, trend: trendFrom(series) };
    }
    case 'heart_rate_variability': {
      const series = rows.map(r => num(r.avg));
      return { value: formatMetricValue('heart_rate_variability', series[series.length - 1]), unit: 'ms', sparkline: series, trend: trendFrom(series) };
    }
    case 'weight': {
      const series = rows.map(r => num(r.avg));
      return { value: formatMetricValue('weight', series[series.length - 1]), unit: 'kg', sparkline: series, trend: trendFrom(series) };
    }
    case 'oxygen_saturation': {
      const series = rows.map(r => num(r.avg));
      return { value: formatMetricValue('oxygen_saturation', series[series.length - 1]), unit: '%', sparkline: series, trend: trendFrom(series) };
    }
    case 'blood_pressure': {
      const sysSeries = rows.map(r => num(r.systolic_avg));
      const diaSeries = rows.map(r => num(r.diastolic_avg));
      const sysLatest = formatMetricValue('blood_pressure', sysSeries[sysSeries.length - 1]);
      const diaLatest = formatMetricValue('blood_pressure', diaSeries[diaSeries.length - 1]);
      return { value: `${sysLatest}/${diaLatest}`, sparkline: sysSeries, trend: trendFrom(sysSeries) };
    }
    default:
      return null;
  }
}
