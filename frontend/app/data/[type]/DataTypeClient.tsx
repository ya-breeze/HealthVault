'use client';
import { useEffect, useMemo, useState } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import {
  LineChart, Line, BarChart, Bar, ComposedChart, Area, XAxis, YAxis, CartesianGrid, Tooltip,
  Legend, ResponsiveContainer,
} from 'recharts';
import { api, DataType } from '@/lib/api';
import { metricColorVar } from '@/lib/tokens';
import { TYPE_META, NUTRITION_MACROS, Zoom, rangeForZoom, computeYDomain, emaSeries } from '@/lib/dataTypeMeta';
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

  const [zoom, setZoom] = useState<Zoom>('week');
  const [macro, setMacro] = useState<string>('calories');
  const [records, setRecords] = useState<Record<string, unknown>[]>([]);
  const [chartRows, setChartRows] = useState<Record<string, unknown>[]>([]);
  const [loading, setLoading] = useState(true);
  const [pendingDeleteId, setPendingDeleteId] = useState<string | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  const { from, to, bucket } = useMemo(() => rangeForZoom(zoom), [zoom]);

  // weight's Week-zoom trend line needs 14 trailing days of bucketed data to
  // seed its EMA before the visible 7-day window starts — see
  // chart-zoom-aggregation's "Weight trend line" requirement. Only the
  // bucketed fetch is widened; the raw fetch (`records`) and stats row stay
  // on the normal 7-day range regardless.
  const isWeightWeek = dataType === 'weight' && zoom === 'week';
  const chartFrom = useMemo(() => {
    if (!isWeightWeek) return from;
    const widened = new Date(to);
    widened.setDate(widened.getDate() - 14);
    return widened.toISOString();
  }, [isWeightWeek, from, to]);

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

  const fromMs = new Date(from).getTime();
  const toMs = new Date(to).getTime();

  const dayLineData = timeKey
    ? records.map(r => ({ ...r, [timeKey]: new Date(r[timeKey] as string).getTime() }))
    : records;

  // For weight+week, `chartRows` holds the widened 14-day fetch used to seed
  // the trend's EMA (see the effect above); every other chart/stat must only
  // ever see the zoom's own visible window, so they read this slice instead.
  const visibleChartRows = isWeightWeek ? chartRows.slice(-7) : chartRows;

  // Trend is computed from the full (possibly widened) series so the EMA has
  // time to stabilize, then sliced down to the same visible window as
  // everything else before being merged into bucketBandData below.
  const trendFull = dataType === 'weight' ? emaSeries(chartRows.map(r => num(r.avg)), 0.25) : [];
  const visibleTrend = isWeightWeek ? trendFull.slice(-7) : trendFull;

  const bucketBarData = visibleChartRows.map(r => ({
    label: bucketLabel(r.bucket_start, zoom),
    value: isNutrition ? num(r[`sum_${macro}`]) : num(r.sum),
  }));

  const bucketBandData = visibleChartRows.map((r, i) => ({
    label: bucketLabel(r.bucket_start, zoom),
    avg: num(r.avg),
    min: num(r.min),
    band: num(r.max) - num(r.min),
    ...(dataType === 'weight' ? { trend: visibleTrend[i] } : {}),
  }));

  const bucketBPData = visibleChartRows.map(r => ({
    label: bucketLabel(r.bucket_start, zoom),
    sysAvg: num(r.systolic_avg), sysMin: num(r.systolic_min), sysBand: num(r.systolic_max) - num(r.systolic_min),
    diaAvg: num(r.diastolic_avg), diaMin: num(r.diastolic_min), diaBand: num(r.diastolic_max) - num(r.diastolic_min),
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
      return numericKey ? records.map(r => num(r[numericKey])) : [];
    }
    return visibleChartRows.map(r => (meta?.family === 'cumulative' ? num(r.sum) : num(r.avg)));
  }, [isBloodPressure, isNutrition, isDay, records, visibleChartRows, numericKey, macro, meta]);

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
                  <YAxis domain={dayDomain} tick={{ fill: 'var(--text-muted)', fontSize: 11 }} />
                  <Tooltip labelFormatter={(v: unknown) => new Date(v as number).toLocaleString()} />
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
                  <YAxis domain={dayDomain} tick={{ fill: 'var(--text-muted)', fontSize: 11 }} />
                  <Tooltip labelFormatter={(v: unknown) => new Date(v as number).toLocaleString()} />
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
                <YAxis domain={bandDomain} tick={{ fill: 'var(--text-muted)', fontSize: 11 }} />
                <Tooltip />
                <Legend wrapperStyle={{ fontSize: 12 }} />
                <Area dataKey="sysMin" stackId="sys" stroke="none" fill="transparent" legendType="none" />
                <Area dataKey="sysBand" stackId="sys" stroke="none" fill={color} fillOpacity={0.15} legendType="none" />
                <Area dataKey="diaMin" stackId="dia" stroke="none" fill="transparent" legendType="none" />
                <Area dataKey="diaBand" stackId="dia" stroke="none" fill={color} fillOpacity={0.08} legendType="none" />
                <Line type="monotone" dataKey="sysAvg" stroke={color} strokeWidth={2} dot={false} name="Systolic" />
                <Line type="monotone" dataKey="diaAvg" stroke={color} strokeDasharray="4 3" strokeWidth={2} dot={false} name="Diastolic" />
              </ComposedChart>
            ) : meta?.family === 'cumulative' ? (
              <BarChart data={bucketBarData}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" opacity={0.5} />
                <XAxis dataKey="label" tick={{ fill: 'var(--text-muted)', fontSize: 11 }} />
                <YAxis tick={{ fill: 'var(--text-muted)', fontSize: 11 }} />
                <Tooltip />
                <Bar dataKey="value" fill={color} radius={[3, 3, 0, 0]} />
              </BarChart>
            ) : (
              <ComposedChart data={bucketBandData}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" opacity={0.5} />
                <XAxis dataKey="label" tick={{ fill: 'var(--text-muted)', fontSize: 11 }} />
                <YAxis domain={bandDomain} tick={{ fill: 'var(--text-muted)', fontSize: 11 }} />
                <Tooltip />
                {dataType === 'weight' && <Legend wrapperStyle={{ fontSize: 12 }} />}
                <Area dataKey="min" stackId="a" stroke="none" fill="transparent" legendType="none" />
                <Area dataKey="band" stackId="a" stroke="none" fill={color} fillOpacity={0.18} legendType="none" />
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
              <p className="font-[family-name:var(--font-data)] text-base font-semibold text-text tabular-nums">{stats.avg.toFixed(1)}</p>
            </div>
            <div>
              <p className="font-[family-name:var(--font-data)] text-[11px] font-bold uppercase tracking-wide text-text-muted mb-1">Max</p>
              <p className="font-[family-name:var(--font-data)] text-base font-semibold text-text tabular-nums">{stats.max.toFixed(1)}</p>
            </div>
            {showTotal && (
              <div>
                <p className="font-[family-name:var(--font-data)] text-[11px] font-bold uppercase tracking-wide text-text-muted mb-1">Total</p>
                <p className="font-[family-name:var(--font-data)] text-base font-semibold text-text tabular-nums">{stats.total.toLocaleString()}</p>
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
