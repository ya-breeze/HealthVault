'use client';
import { useEffect, useMemo, useState } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import {
  LineChart, Line, BarChart, Bar, ComposedChart, Area, XAxis, YAxis, CartesianGrid, Tooltip,
  Legend, ResponsiveContainer,
} from 'recharts';
import { api, DataType } from '@/lib/api';
import { metricColorVar } from '@/lib/tokens';
import { TYPE_META, NUTRITION_MACROS, Zoom, rangeForZoom } from '@/lib/dataTypeMeta';
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

  useEffect(() => {
    setLoading(true);
    setPendingDeleteId(null);
    setDeleteError(null);

    const effectiveBucket = hasChart ? bucket : undefined;
    const rawPromise = api.data(type, from, to, userParam);
    const chartPromise = effectiveBucket ? api.data(type, from, to, userParam, effectiveBucket) : rawPromise;

    Promise.all([rawPromise, chartPromise])
      .then(([raw, chart]) => {
        setRecords(raw);
        setChartRows(chart);
        setLoading(false);
      })
      .catch(() => router.push('/login'));
  }, [type, from, to, bucket, hasChart, userParam, router]);

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

  const bucketBarData = chartRows.map(r => ({
    label: bucketLabel(r.bucket_start, zoom),
    value: isNutrition ? num(r[`sum_${macro}`]) : num(r.sum),
  }));

  const bucketBandData = chartRows.map(r => ({
    label: bucketLabel(r.bucket_start, zoom),
    avg: num(r.avg),
    min: num(r.min),
    band: num(r.max) - num(r.min),
  }));

  const bucketBPData = chartRows.map(r => ({
    label: bucketLabel(r.bucket_start, zoom),
    sysAvg: num(r.systolic_avg), sysMin: num(r.systolic_min), sysBand: num(r.systolic_max) - num(r.systolic_min),
    diaAvg: num(r.diastolic_avg), diaMin: num(r.diastolic_min), diaBand: num(r.diastolic_max) - num(r.diastolic_min),
  }));

  // A single flattened series driving the stats row, uniform across Day
  // (raw records) and Week/Month/Year (bucketed) — see chart-zoom-aggregation's
  // "Chart summary stats follow the active zoom" requirement.
  const primarySeries = useMemo(() => {
    if (isBloodPressure) {
      return isDay ? records.map(r => num(r.systolic)) : chartRows.map(r => num(r.systolic_avg));
    }
    if (isNutrition) {
      return isDay ? records.map(r => num(r[macro])) : chartRows.map(r => num(r[`sum_${macro}`]));
    }
    if (isDay) {
      return numericKey ? records.map(r => num(r[numericKey])) : [];
    }
    return chartRows.map(r => (meta?.family === 'cumulative' ? num(r.sum) : num(r.avg)));
  }, [isBloodPressure, isNutrition, isDay, records, chartRows, numericKey, macro, meta]);

  const stats = {
    avg: mean(primarySeries),
    max: primarySeries.length ? Math.max(...primarySeries) : 0,
    total: primarySeries.reduce((a, b) => a + b, 0),
  };
  const showTotal = !isBloodPressure && meta?.family === 'cumulative';

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
                  <YAxis tick={{ fill: 'var(--text-muted)', fontSize: 11 }} />
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
                  <YAxis tick={{ fill: 'var(--text-muted)', fontSize: 11 }} />
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
                <YAxis tick={{ fill: 'var(--text-muted)', fontSize: 11 }} />
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
                <YAxis tick={{ fill: 'var(--text-muted)', fontSize: 11 }} />
                <Tooltip />
                <Area dataKey="min" stackId="a" stroke="none" fill="transparent" legendType="none" />
                <Area dataKey="band" stackId="a" stroke="none" fill={color} fillOpacity={0.18} legendType="none" />
                <Line type="monotone" dataKey="avg" stroke={color} strokeWidth={2} dot={false} />
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
