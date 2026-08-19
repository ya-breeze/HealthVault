import type { DataType } from '@/lib/api';

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
  if (!saved || saved.length === 0) return PRIMARY_METRICS;
  const byType = new Map(PRIMARY_METRICS.map(m => [m.type as string, m]));
  const ordered = saved.map(t => byType.get(t)).filter((m): m is (typeof PRIMARY_METRICS)[number] => !!m);
  const included = new Set(ordered.map(m => m.type));
  const missing = PRIMARY_METRICS.filter(m => !included.has(m.type));
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
      return { value: Math.round(series[series.length - 1]).toLocaleString(), sparkline: series, trend: trendFrom(series) };
    }
    case 'distance': {
      const series = rows.map(r => num(r.sum) / 1000); // meters -> km
      return { value: series[series.length - 1].toFixed(1), unit: 'km', sparkline: series, trend: trendFrom(series) };
    }
    case 'sleep': {
      const series = rows.map(r => num(r.sum) / 3600); // seconds -> hours
      return { value: series[series.length - 1].toFixed(1), unit: 'h', sparkline: series, trend: trendFrom(series) };
    }
    case 'heart_rate': {
      const series = rows.map(r => num(r.avg));
      return { value: Math.round(series[series.length - 1]).toString(), unit: 'bpm', sparkline: series, trend: trendFrom(series) };
    }
    case 'heart_rate_variability': {
      const series = rows.map(r => num(r.avg));
      return { value: Math.round(series[series.length - 1]).toString(), unit: 'ms', sparkline: series, trend: trendFrom(series) };
    }
    case 'weight': {
      const series = rows.map(r => num(r.avg));
      return { value: series[series.length - 1].toFixed(1), unit: 'kg', sparkline: series, trend: trendFrom(series) };
    }
    case 'oxygen_saturation': {
      const series = rows.map(r => num(r.avg));
      return { value: Math.round(series[series.length - 1]).toString(), unit: '%', sparkline: series, trend: trendFrom(series) };
    }
    case 'blood_pressure': {
      const sysSeries = rows.map(r => num(r.systolic_avg));
      const diaSeries = rows.map(r => num(r.diastolic_avg));
      const sysLatest = Math.round(sysSeries[sysSeries.length - 1]);
      const diaLatest = Math.round(diaSeries[diaSeries.length - 1]);
      return { value: `${sysLatest}/${diaLatest}`, sparkline: sysSeries, trend: trendFrom(sysSeries) };
    }
    default:
      return null;
  }
}
