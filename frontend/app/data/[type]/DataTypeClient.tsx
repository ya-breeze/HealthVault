'use client';
import { useEffect, useMemo, useState } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import {
  LineChart, Line, BarChart, Bar, ComposedChart, Area, XAxis, YAxis, CartesianGrid, Tooltip,
  Legend, ResponsiveContainer, TooltipValueType,
} from 'recharts';
import { api, DataType } from '@/lib/api';
import { metricColorVar } from '@/lib/tokens';
import { TYPE_META, NUTRITION_MACROS, Zoom, rangeForZoom, computeYDomain, emaSeries, formatMetricValue, toDisplayUnit } from '@/lib/dataTypeMeta';
import Header from '@/components/Header';

interface Props {
  type: string;
}

const ZOOMS: { key: Zoom; label: string }[] = [
  { key: 'day', label: 'Day' },
  { key: 'week', label: 'Week' },
  { key: 'month', label: 'Month' },
  { key: 'year', label: 'Year' },
];

function num(v: unknown): number {
  return typeof v === 'number' ? v : Number(v ?? 0);
}

function bucketLabel(bucketStart: unknown, zoom: Zoom): string {
  const d = new Date(String(bucketStart));
  if (isNaN(d.getTime())) return String(bucketStart ?? '');
  return zoom === 'year'
    ? d.toLocaleDateString(undefined, { month: 'short' })
    : d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
}

function mean(values: number[]): number {
  return values.length ? values.reduce((a, b) => a + b, 0) / values.length : 0;
}

export default function DataTypeClient({ type }: Props) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const userParam = searchParams.get('user') ?? undefined;
  const dataType = type as DataType;
  const meta = TYPE_META[dataType];
  const isBloodPressure = type === 'blood_pressure';
  const isNutrition = type === 'nutrition';
  // food_meal never accepts ?bucket= (see data-api spec) and fits neither
  // aggregation family, so it gets the zoom control's time-range picking
  // but no chart — table only, always raw.
  const hasChart = type !== 'food_meal';
  const color = metricColorVar(dataType);

  // Converts a raw API value into this page's type's display unit (distance:
  // meters -> km, sleep: seconds -> hours; everything else passes through
  // unchanged) — see toDisplayUnit's doc comment. Every raw numeric value
  // that reaches the chart or stats row for THIS type must go through this,
  // not just num(), or the chart silently disagrees with the vitals-grid
  // card's unit for the same metric (not just its rounding).
  const numDisplay = (v: unknown) => toDisplayUnit(dataType, num(v));

  // Shared across every Tooltip below (raw line/bars, the min-max band, and
  // the weight trend line all use the same metric's precision) — see
  // chart-value-rounding's "Chart display precision" requirement. Handles
  // the min-max band's [number, number] tuple value as well as plain
  // numbers; only the rendered text is rounded, never the underlying data.
  const formatTooltipValue = (
    value: TooltipValueType | undefined, name: string | number | undefined
  ): [string, string | number] => {
    if (value === undefined) return ['', name ?? ''];
    if (Array.isArray(value)) {
      return [value.map(v => formatMetricValue(dataType, Number(v))).join(' – '), name ?? ''];
    }
    return [formatMetricValue(dataType, Number(value)), name ?? ''];
  };
  const yAxisTickFormatter = (v: number) => formatMetricValue(dataType, v);

  const [zoom, setZoom] = useState<Zoom>('week');
  const [macro, setMacro] = useState<string>('calories');
  const [records, setRecords] = useState<Record<string, unknown>[]>([]);
  const [chartRows, setChartRows] = useState<Record<string, unknown>[]>([]);
  const [loading, setLoading] = useState(true);
  const [pendingDeleteId, setPendingDeleteId] = useState<string | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  const { from, to, bucket } = useMemo(() => rangeForZoom(zoom), [zoom]);

  // weight's Week/Year-zoom trend line needs enough trailing bucketed history
  // to seed its EMA before the visible window starts — see
  // chart-zoom-aggregation's "Weight trend line" requirement. An alpha=0.25
  // EMA needs ~14-16 periods to converge; Week's 7 daily buckets and Year's
  // ~12-13 monthly buckets both fall short, so both get their bucketed fetch
  // doubled. Month's 30 daily buckets already exceed it and are left
  // untouched. Only the bucketed fetch is widened; the raw fetch (`records`)
  // and stats row stay on the normal range regardless.
  const needsWidenedLookback = dataType === 'weight' && (zoom === 'week' || zoom === 'year');
  const chartFrom = useMemo(() => {
    if (!needsWidenedLookback) return from;
    const widened = new Date(from);
    if (zoom === 'week') {
      widened.setDate(widened.getDate() - 7);
    } else {
      widened.setFullYear(widened.getFullYear() - 1);
    }
    return widened.toISOString();
  }, [needsWidenedLookback, zoom, from]);
  const fromMs = new Date(from).getTime();
  const toMs = new Date(to).getTime();

  useEffect(() => {
    setLoading(true);
    setPendingDeleteId(null);
    setDeleteError(null);

    // Raw and chart data are fetched independently: a raw-fetch failure
    // means the session is invalid (same signal the app has always used to
    // redirect to /login), but a chart/bucket-fetch failure is a narrower,
    // non-fatal problem — it should leave the chart empty, not log the user
    // out for an unrelated error.
    api.data(type, from, to, userParam)
      .then(raw => {
        setRecords(raw);
        setLoading(false);
      })
      .catch(() => router.push('/login'));

    const effectiveBucket = hasChart ? bucket : undefined;
    if (effectiveBucket) {
      api.data(type, chartFrom, to, userParam, effectiveBucket)
        .then(setChartRows)
        .catch(() => setChartRows([]));
    } else {
      setChartRows([]);
    }
  }, [type, from, to, chartFrom, bucket, hasChart, userParam, router]);

  const isDay = zoom === 'day';

  const numericKey = records.length > 0
    ? Object.entries(records[0]).find(([k, v]) =>
        typeof v === 'number' && !k.endsWith('_id') && k !== 'id'
      )?.[0]
    : undefined;

  const timeKey = records.length > 0
    ? (['time', 'start_time', 'timestamp', 'logged_at'].find(k => k in records[0]))
    : undefined;

  const displayColumns = records.length > 0
    ? Object.keys(records[0]).filter(
        k => !['id', 'family_id', 'user_id', 'source_payload_id', 'deleted_at'].includes(k)
      )
    : [];

  const dayLineData = records.map(r => ({
    ...r,
    ...(timeKey ? { [timeKey]: new Date(r[timeKey] as string).getTime() } : {}),
    // numericKey is the raw column driving the Line's dataKey below for
    // non-nutrition types (e.g. "meters", "duration_seconds") — convert it
    // in place to this type's display unit so the chart doesn't show
    // storage units that disagree with the vitals-grid card for the same
    // metric. No-op for types whose storage unit already is the display
    // unit (toDisplayUnit's default), so this is harmless to apply
    // regardless of numericKey's relevance for nutrition (rendered via
    // `macro` instead, untouched here).
    ...(numericKey ? { [numericKey]: numDisplay(r[numericKey]) } : {}),
  }));

  // For weight+week/year, `chartRows` holds the widened fetch used to seed
  // the trend's EMA (see above); every other chart/stat must only ever see
  // the zoom's own visible window. Filtered by each row's own bucket_start
  // against the visible range's real start date, not by array position: the
  // backend's bucket query is a plain GROUP BY with no zero-fill for missing
  // days (storage_impl.go's QueryAggregate), so sparse logging can return
  // fewer rows than the widened range spans — a positional slice(-N) would
  // then pull buckets from well outside the visible window into what's
  // labeled and rendered as the current view.
  const visibleMask = needsWidenedLookback
    ? chartRows.map(r => new Date(String(r.bucket_start)).getTime() >= fromMs)
    : chartRows.map(() => true);
  const visibleChartRows = chartRows.filter((_, i) => visibleMask[i]);

  // Trend is computed from the full (possibly widened) series so the EMA has
  // time to stabilize, then filtered down to the same visible window as
  // everything else (via the same mask, so it stays aligned with
  // visibleChartRows by index) before being merged into bucketBandData below.
  const trendFull = dataType === 'weight' ? emaSeries(chartRows.map(r => num(r.avg)), 0.25) : [];
  const visibleTrend = trendFull.filter((_, i) => visibleMask[i]);

  const bucketBarData = visibleChartRows.map(r => ({
    label: bucketLabel(r.bucket_start, zoom),
    value: isNutrition ? num(r[`sum_${macro}`]) : numDisplay(r.sum),
  }));

  // `range` is a [min, max] tuple rather than a stacked min+band pair: Recharts
  // renders a dataKey that resolves to a tuple as a "ranged Area" positioned
  // directly between those two values, with no stacking baseline involved. A
  // stacked (transparent-bottom + visible-band) Area would instead anchor its
  // baseline at the stack's absolute value origin (0), which silently pulls
  // the Y-axis back toward zero regardless of the `domain` prop below.
  const bucketBandData = visibleChartRows.map((r, i) => ({
    label: bucketLabel(r.bucket_start, zoom),
    avg: num(r.avg),
    range: [num(r.min), num(r.max)] as [number, number],
    ...(dataType === 'weight' ? { trend: visibleTrend[i] } : {}),
  }));

  const bucketBPData = visibleChartRows.map(r => ({
    label: bucketLabel(r.bucket_start, zoom),
    sysAvg: num(r.systolic_avg), sysRange: [num(r.systolic_min), num(r.systolic_max)] as [number, number],
    diaAvg: num(r.diastolic_avg), diaRange: [num(r.diastolic_min), num(r.diastolic_max)] as [number, number],
  }));

  // Two flattened series driving the stats row, uniform across Day (raw
  // records) and Week/Month/Year (bucketed) — see chart-zoom-aggregation's
  // "Chart summary stats follow the active zoom" requirement. Kept separate
  // because for point-family types Week/Month/Year only reads a bucket's
  // avg for the Avg/Total stats, but must read that bucket's own `max` for
  // the Max stat — the highest per-bucket average is not the highest
  // recorded value, which is exactly what the chart's shaded band already
  // shows and the stats row should agree with.
  const primaryAvgSeries = useMemo(() => {
    if (isBloodPressure) {
      return isDay ? records.map(r => num(r.systolic)) : visibleChartRows.map(r => num(r.systolic_avg));
    }
    if (isNutrition) {
      return isDay ? records.map(r => num(r[macro])) : visibleChartRows.map(r => num(r[`sum_${macro}`]));
    }
    if (isDay) {
      return numericKey ? records.map(r => numDisplay(r[numericKey])) : [];
    }
    return visibleChartRows.map(r => (meta?.family === 'cumulative' ? numDisplay(r.sum) : num(r.avg)));
  }, [isBloodPressure, isNutrition, isDay, records, visibleChartRows, numericKey, macro, meta, numDisplay]);

  const primaryMaxSeries = useMemo(() => {
    // Day (raw points) and cumulative types, including nutrition (whose
    // bucketed response has no separate max column — a bucket's sum *is*
    // the quantity of interest) use the same series as Avg/Total.
    if (isDay || meta?.family === 'cumulative') {
      return primaryAvgSeries;
    }
    if (isBloodPressure) {
      return visibleChartRows.map(r => num(r.systolic_max));
    }
    return visibleChartRows.map(r => num(r.max));
  }, [isDay, isBloodPressure, visibleChartRows, meta, primaryAvgSeries]);

  const stats = {
    avg: mean(primaryAvgSeries),
    max: primaryMaxSeries.length ? Math.max(...primaryMaxSeries) : 0,
    total: primaryAvgSeries.reduce((a, b) => a + b, 0),
  };
  const showTotal = !isBloodPressure && meta?.family === 'cumulative';

  // Point-in-time Y-axis domains — computed from the values actually driving
  // each chart, not defaulted toward zero. Cumulative types (bar charts) are
  // deliberately excluded and keep their zero-anchored default. See
  // chart-zoom-aggregation's "Point-in-time Y-axis domain" requirement.
  const dayDomain = useMemo(() => {
    if (meta?.family !== 'point') return undefined;
    const values = isBloodPressure
      ? records.flatMap(r => [num(r.systolic), num(r.diastolic)])
      : (numericKey ? records.map(r => num(r[numericKey])) : []);
    return computeYDomain(values);
  }, [meta, isBloodPressure, records, numericKey]);

  const bandDomain = useMemo(() => {
    if (meta?.family !== 'point') return undefined;
    const values = isBloodPressure
      ? visibleChartRows.flatMap(r => [
          num(r.systolic_min), num(r.systolic_max), num(r.diastolic_min), num(r.diastolic_max),
        ])
      : visibleChartRows.flatMap(r => [num(r.min), num(r.max)]);
    return computeYDomain(values);
  }, [meta, isBloodPressure, visibleChartRows]);

  const handleConfirmDelete = async (id: string) => {
    setDeleting(true);
    setDeleteError(null);
    try {
      await api.deleteRecord(type, id);
      setRecords(prev => prev.filter(r => r.id !== id));
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : 'Delete failed');
    } finally {
      setDeleting(false);
      setPendingDeleteId(prev => prev === id ? null : prev);
    }
  };

  return (
    <div className="min-h-screen bg-bg">
      <Header />

      <main className="max-w-4xl mx-auto px-6 py-8">
        <div className="flex items-center justify-between flex-wrap gap-3 mb-6">
          <h1 className="text-xl font-bold capitalize text-text flex items-center gap-2">
            <span className="w-2.5 h-2.5 rounded-full" style={{ background: color }} />
            {type.replace(/_/g, ' ')}
          </h1>
          <div className="flex gap-1 bg-bg-elevated border border-border rounded-lg p-1">
            {ZOOMS.map(z => (
              <button
                key={z.key}
                onClick={() => setZoom(z.key)}
                className={`font-[family-name:var(--font-data)] text-xs font-semibold px-3 py-1.5 rounded-md transition-colors ${
                  zoom === z.key ? 'bg-border text-accent' : 'text-text-muted hover:text-text'
                }`}
              >
                {z.label}
              </button>
            ))}
          </div>
        </div>

        {isNutrition && (
          <div className="flex gap-1.5 flex-wrap mb-4">
            {NUTRITION_MACROS.map(m => (
              <button
                key={m.key}
                onClick={() => setMacro(m.key)}
                className={`font-[family-name:var(--font-data)] text-[11px] font-bold uppercase tracking-wide px-2.5 py-1 rounded-md border transition-colors ${
                  macro === m.key ? 'border-accent text-accent' : 'border-border text-text-muted hover:text-text'
                }`}
              >
                {m.label}
              </button>
            ))}
          </div>
        )}

        {deleteError && (
          <div className="mb-4 px-4 py-3 rounded-lg bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 text-red-700 dark:text-red-300 text-sm">
            Delete failed: {deleteError}
          </div>
        )}

        {hasChart && (
        <div className="bg-bg-elevated rounded-[12px] border border-border p-4 mb-4">
          <ResponsiveContainer width="100%" height={280}>
            {isDay ? (
              isBloodPressure ? (
                <LineChart data={dayLineData}>
                  <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" opacity={0.5} />
                  <XAxis
                    dataKey={timeKey}
                    type="number"
                    scale="time"
                    domain={[fromMs, toMs]}
                    tickFormatter={(v: number) => new Date(v).toLocaleTimeString(undefined, { hour: 'numeric' })}
                    tick={{ fill: 'var(--text-muted)', fontSize: 11 }}
                  />
                  <YAxis domain={dayDomain} tick={{ fill: 'var(--text-muted)', fontSize: 11 }} tickFormatter={yAxisTickFormatter} />
                  <Tooltip labelFormatter={(v: unknown) => new Date(v as number).toLocaleString()} formatter={formatTooltipValue} />
                  <Legend wrapperStyle={{ fontSize: 12 }} />
                  <Line type="monotone" dataKey="systolic" stroke={color} dot strokeWidth={2} name="Systolic" />
                  <Line type="monotone" dataKey="diastolic" stroke={color} strokeDasharray="4 3" dot strokeWidth={2} name="Diastolic" />
                </LineChart>
              ) : (
                <LineChart data={dayLineData}>
                  <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" opacity={0.5} />
                  <XAxis
                    dataKey={timeKey}
                    type="number"
                    scale="time"
                    domain={[fromMs, toMs]}
                    tickFormatter={(v: number) => new Date(v).toLocaleTimeString(undefined, { hour: 'numeric' })}
                    tick={{ fill: 'var(--text-muted)', fontSize: 11 }}
                  />
                  <YAxis domain={dayDomain} tick={{ fill: 'var(--text-muted)', fontSize: 11 }} tickFormatter={yAxisTickFormatter} />
                  <Tooltip labelFormatter={(v: unknown) => new Date(v as number).toLocaleString()} formatter={formatTooltipValue} />
                  <Line
                    type="monotone"
                    dataKey={isNutrition ? macro : numericKey}
                    stroke={color}
                    dot
                    strokeWidth={2}
                  />
                </LineChart>
              )
            ) : isBloodPressure ? (
              <ComposedChart data={bucketBPData}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" opacity={0.5} />
                <XAxis dataKey="label" tick={{ fill: 'var(--text-muted)', fontSize: 11 }} />
                <YAxis domain={bandDomain} tick={{ fill: 'var(--text-muted)', fontSize: 11 }} tickFormatter={yAxisTickFormatter} />
                <Tooltip formatter={formatTooltipValue} />
                <Legend wrapperStyle={{ fontSize: 12 }} />
                <Area dataKey="sysRange" stroke="none" fill={color} fillOpacity={0.15} legendType="none" name="Systolic range" />
                <Area dataKey="diaRange" stroke="none" fill={color} fillOpacity={0.08} legendType="none" name="Diastolic range" />
                <Line type="monotone" dataKey="sysAvg" stroke={color} strokeWidth={2} dot={false} name="Systolic" />
                <Line type="monotone" dataKey="diaAvg" stroke={color} strokeDasharray="4 3" strokeWidth={2} dot={false} name="Diastolic" />
              </ComposedChart>
            ) : meta?.family === 'cumulative' ? (
              <BarChart data={bucketBarData}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" opacity={0.5} />
                <XAxis dataKey="label" tick={{ fill: 'var(--text-muted)', fontSize: 11 }} />
                <YAxis tick={{ fill: 'var(--text-muted)', fontSize: 11 }} tickFormatter={yAxisTickFormatter} />
                <Tooltip formatter={formatTooltipValue} />
                <Bar dataKey="value" fill={color} radius={[3, 3, 0, 0]} />
              </BarChart>
            ) : (
              <ComposedChart data={bucketBandData}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" opacity={0.5} />
                <XAxis dataKey="label" tick={{ fill: 'var(--text-muted)', fontSize: 11 }} />
                <YAxis domain={bandDomain} tick={{ fill: 'var(--text-muted)', fontSize: 11 }} tickFormatter={yAxisTickFormatter} />
                <Tooltip formatter={formatTooltipValue} />
                {dataType === 'weight' && <Legend wrapperStyle={{ fontSize: 12 }} />}
                <Area dataKey="range" stroke="none" fill={color} fillOpacity={0.18} legendType="none" name="Range" />
                <Line type="monotone" dataKey="avg" stroke={color} strokeWidth={2} dot={false} name="Avg" />
                {dataType === 'weight' && (
                  <Line
                    type="monotone"
                    dataKey="trend"
                    stroke="var(--accent)"
                    strokeWidth={2}
                    strokeDasharray="5 4"
                    dot={false}
                    name="Trend"
                  />
                )}
              </ComposedChart>
            )}
          </ResponsiveContainer>

          <div className="flex gap-6 mt-3 pt-3 border-t border-border">
            <div>
              <p className="font-[family-name:var(--font-data)] text-[11px] font-bold uppercase tracking-wide text-text-muted mb-1">Avg</p>
              <p className="font-[family-name:var(--font-data)] text-base font-semibold text-text tabular-nums">{formatMetricValue(dataType, stats.avg)}</p>
            </div>
            <div>
              <p className="font-[family-name:var(--font-data)] text-[11px] font-bold uppercase tracking-wide text-text-muted mb-1">Max</p>
              <p className="font-[family-name:var(--font-data)] text-base font-semibold text-text tabular-nums">{formatMetricValue(dataType, stats.max)}</p>
            </div>
            {showTotal && (
              <div>
                <p className="font-[family-name:var(--font-data)] text-[11px] font-bold uppercase tracking-wide text-text-muted mb-1">Total</p>
                <p className="font-[family-name:var(--font-data)] text-base font-semibold text-text tabular-nums">{formatMetricValue(dataType, stats.total)}</p>
              </div>
            )}
          </div>
        </div>
        )}

        <div className="bg-bg-elevated rounded-[12px] border border-border overflow-auto">
          {loading ? (
            <p className="p-6 text-text-muted text-center text-sm">Loading...</p>
          ) : (
            <table className="w-full text-sm">
              <thead className="bg-bg border-b border-border">
                <tr>
                  {displayColumns.map(k => (
                    <th key={k} className="px-4 py-3 text-left font-medium text-text-muted text-xs uppercase tracking-wider">
                      {k}
                    </th>
                  ))}
                  {!userParam && (
                    <th className="px-4 py-3 text-left font-medium text-text-muted text-xs uppercase tracking-wider">
                      Actions
                    </th>
                  )}
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {records.map(r => {
                  const id = r.id as string;
                  const isPending = id === pendingDeleteId;
                  return (
                    <tr
                      key={id}
                      className={isPending ? 'bg-red-50 dark:bg-red-900/20' : 'hover:bg-bg transition-colors'}
                    >
                      {displayColumns.map(k => (
                        <td key={k} className="px-4 py-3 text-text">
                          {typeof r[k] === 'string' && (r[k] as string).includes('T')
                            ? new Date(r[k] as string).toLocaleString()
                            : String(r[k] ?? '')}
                        </td>
                      ))}
                      {!userParam && (
                        <td className="px-4 py-3">
                          {isPending ? (
                            <span className="flex items-center gap-2">
                              <button
                                onClick={() => handleConfirmDelete(id)}
                                disabled={deleting}
                                className="text-xs px-2 py-1 rounded bg-red-600 text-white hover:bg-red-700 disabled:opacity-50 disabled:cursor-not-allowed"
                              >
                                {deleting ? '…' : 'Confirm'}
                              </button>
                              <button
                                onClick={() => setPendingDeleteId(null)}
                                disabled={deleting}
                                className="text-xs px-2 py-1 rounded bg-border text-text hover:opacity-80 disabled:opacity-50 disabled:cursor-not-allowed"
                              >
                                Cancel
                              </button>
                            </span>
                          ) : (
                            <button
                              onClick={() => { setDeleteError(null); setPendingDeleteId(id); }}
                              aria-label="Delete record"
                              className="text-text-muted hover:text-red-500 transition-colors"
                            >
                              🗑
                            </button>
                          )}
                        </td>
                      )}
                    </tr>
                  );
                })}
              </tbody>
            </table>
          )}
          {!loading && records.length === 0 && (
            <p className="p-6 text-text-muted text-center text-sm">No data in this range.</p>
          )}
        </div>
      </main>
    </div>
  );
}
