'use client';
import { useEffect, useMemo, useState } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import {
  LineChart, Line, BarChart, Bar, ComposedChart, Area, ReferenceArea, ReferenceLine, XAxis, YAxis,
  CartesianGrid, Tooltip, Legend, ResponsiveContainer, TooltipValueType,
} from 'recharts';
import { api, DataType, WRITABLE_TYPES } from '@/lib/api';
import { metricColorVar } from '@/lib/tokens';
import {
  TYPE_META, NUTRITION_MACROS, Zoom, rangeForZoom, computeYDomain, emaSeries, formatMetricValue,
  toDisplayUnit, bmiBandEdgesKg, classifyBmi, hasHeightRecord, toDayOffset, linearRegression,
  last30DayEmaWindow, hasEnoughDataForProjection, computeProjection, projectionPoints,
  ProjectionResult,
} from '@/lib/dataTypeMeta';
import Header from '@/components/Header';
import AddRecordForm from '@/components/AddRecordForm';
import TapTarget from '@/components/ui/TapTarget';

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

// `projection` is only ever set on the last real bucket (the dashed line's
// join point) and on synthetic future rows appended by the trend-projection
// feature (task 7.4b) — `avg`/`range`/`trend` are deliberately left
// undefined on synthetic rows so the existing series don't extend into them.
interface BandRow {
  label: string;
  avg?: number;
  range?: [number, number];
  trend?: number;
  projection?: number;
}

// weight/height/weight_goal all use timeCol "time" in the backend's
// typeRegistry (see api.go) — safe to hardcode rather than re-deriving the
// timeKey-detection dance `records`/`displayColumns` do for arbitrary types.
function latestByTime(records: Record<string, unknown>[]): Record<string, unknown> | undefined {
  if (records.length === 0) return undefined;
  return records.reduce((latest, r) =>
    new Date(String(r.time)).getTime() > new Date(String(latest.time)).getTime() ? r : latest
  );
}

// Height changes rarely, so the BMI gate (task 5.5) fetches all-time rather
// than the page's own zoom-windowed range — a height set years ago must
// still surface the readout today.
const ALL_TIME_FROM = new Date(0).toISOString();

// WHO-convention band colors (blue/green/yellow/red), low opacity so the
// plotted weight line/area stays the focal element.
const BMI_BAND_COLORS = ['#3b82f6', '#22c55e', '#eab308', '#ef4444'];

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
  // Bumped by AddRecordForm's onSuccess to force the fetch effect below to
  // re-run — the effect otherwise only depends on the range/zoom, so a
  // successful write would never appear in `records`/`chartRows` without
  // navigating away and back. See task 4.1.
  const [refreshKey, setRefreshKey] = useState(0);
  // Latest height record, fetched all-time (task 5.5) — feeds the BMI bands
  // (5.2) and readout (5.3) on the weight page, both gated on its presence
  // (5.4). Empty on every other type's page.
  const [heightRecords, setHeightRecords] = useState<Record<string, unknown>[]>([]);
  // Latest weight_goal record, fetched all-time (task 6.1) — a goal is set
  // once and rarely changed, so (like height) it must not fall out of a
  // zoom-windowed fetch. Feeds the goal ReferenceLine (6.2) on the weight
  // page. Empty on every other type's page.
  const [goalRecords, setGoalRecords] = useState<Record<string, unknown>[]>([]);
  // All-time raw weight records (task 7.3's lifetime-history gate: total
  // record count + earliest-to-latest span) — fetched all-time like
  // height/goal above, since the dedicated regression fetch below is capped
  // to a fixed lookback and can't answer "how much history exists in total".
  const [allTimeWeightRecords, setAllTimeWeightRecords] = useState<Record<string, unknown>[]>([]);
  // Dedicated 30+-day daily-bucketed weight fetch for the trend regression
  // (task 7.0) — independent of the active zoom's own bucket fetch: Day zoom
  // fetches no bucketed data, Week's widened lookback only reaches 14 days,
  // and Year's bucket is monthly, so none of the three can feed a 30-daily-
  // point EMA window on their own. 60 days back gives the alpha=0.25 EMA
  // room to converge before the last 30 calendar days it's actually judged on.
  const [projectionBucketRows, setProjectionBucketRows] = useState<Record<string, unknown>[]>([]);
  const isWritable = WRITABLE_TYPES.includes(dataType);
  // "Set goal" shortcut (task 4.2): only meaningful on the weight page,
  // where it opens the same AddRecordForm pre-targeted at `weight_goal`
  // rather than `weight` — a distinct type from the page it's mounted on.
  const [showGoalForm, setShowGoalForm] = useState(false);

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

    if (dataType === 'weight') {
      api.data('height', ALL_TIME_FROM, to, userParam)
        .then(setHeightRecords)
        .catch(() => setHeightRecords([]));
      api.data('weight_goal', ALL_TIME_FROM, to, userParam)
        .then(setGoalRecords)
        .catch(() => setGoalRecords([]));
      api.data('weight', ALL_TIME_FROM, to, userParam)
        .then(setAllTimeWeightRecords)
        .catch(() => setAllTimeWeightRecords([]));
      const projectionFrom = new Date(to);
      projectionFrom.setDate(projectionFrom.getDate() - 60);
      api.data('weight', projectionFrom.toISOString(), to, userParam, 'day')
        .then(setProjectionBucketRows)
        .catch(() => setProjectionBucketRows([]));
    } else {
      setHeightRecords([]);
      setGoalRecords([]);
      setAllTimeWeightRecords([]);
      setProjectionBucketRows([]);
    }
  }, [type, dataType, from, to, chartFrom, bucket, hasChart, userParam, router, refreshKey]);

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
  const bucketBandData: BandRow[] = visibleChartRows.map((r, i) => ({
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
  // Routed through numDisplay (not num()) for the same reason the plotted
  // series is: currently a no-op for every 'point'-family type (toDisplayUnit
  // only converts distance/sleep, both 'cumulative'), but a code-review pass
  // flagged that leaving these on raw num() would silently desync the axis
  // range from the data the moment any point-family type gains a unit
  // conversion — keeping both on the same helper closes that off structurally.
  // Latest weight_goal record (task 6.1) — unlike BMI bands, the goal line's
  // value IS folded into both Y-domain computations below, so it always
  // expands the axis rather than being clipped to it (design.md's "Goal line
  // Y-domain: always expands" — an accepted trade-off).
  const latestGoalKg = useMemo(() => {
    if (dataType !== 'weight') return undefined;
    const latest = latestByTime(goalRecords);
    return latest ? num(latest.kilograms) : undefined;
  }, [dataType, goalRecords]);

  const dayDomain = useMemo(() => {
    if (meta?.family !== 'point') return undefined;
    const values = isBloodPressure
      ? records.flatMap(r => [numDisplay(r.systolic), numDisplay(r.diastolic)])
      : (numericKey ? records.map(r => numDisplay(r[numericKey])) : []);
    if (latestGoalKg !== undefined) values.push(latestGoalKg);
    return computeYDomain(values);
  }, [meta, isBloodPressure, records, numericKey, latestGoalKg]);

  const bandDomain = useMemo(() => {
    if (meta?.family !== 'point') return undefined;
    const values = isBloodPressure
      ? visibleChartRows.flatMap(r => [
          numDisplay(r.systolic_min), numDisplay(r.systolic_max), numDisplay(r.diastolic_min), numDisplay(r.diastolic_max),
        ])
      : visibleChartRows.flatMap(r => [numDisplay(r.min), numDisplay(r.max)]);
    if (latestGoalKg !== undefined) values.push(latestGoalKg);
    return computeYDomain(values);
  }, [meta, isBloodPressure, visibleChartRows, latestGoalKg]);

  // BMI bands + readout (task 5): both gated on the same "does a height
  // record exist" condition (hasHeightRecord, task 5.4) so neither can
  // render while the other silently doesn't.
  const heightExists = dataType === 'weight' && hasHeightRecord(heightRecords);
  const latestHeightMeters = useMemo(() => {
    const latest = latestByTime(heightRecords);
    return latest ? num(latest.meters) : undefined;
  }, [heightRecords]);
  // The same raw weight record already shown in the table below — see
  // design.md's "matches the same raw weight already shown elsewhere on the
  // page" — not a separate all-time fetch like height's.
  const latestWeightKg = useMemo(() => {
    if (dataType !== 'weight') return undefined;
    const latest = latestByTime(records);
    return latest ? num(latest.kilograms) : undefined;
  }, [dataType, records]);
  const bmi = heightExists && latestHeightMeters !== undefined && latestWeightKg !== undefined
    ? latestWeightKg / (latestHeightMeters * latestHeightMeters)
    : undefined;
  const bmiBandEdges = heightExists && latestHeightMeters !== undefined
    ? bmiBandEdgesKg(latestHeightMeters)
    : undefined;
  // Rendered as ReferenceAreas whose open ends (an omitted y1/y2) fall back
  // to the Y-axis's own current domain — see recharts' ReferenceArea
  // getRect — so the bands are clipped to whatever the chart's Y-domain
  // already is and can never be the thing that expands it (design.md).
  const bmiBandAreas = bmiBandEdges
    ? [
        <ReferenceArea key="bmi-under" y2={bmiBandEdges[0]} fill={BMI_BAND_COLORS[0]} fillOpacity={0.08} stroke="none" ifOverflow="hidden" />,
        <ReferenceArea key="bmi-normal" y1={bmiBandEdges[0]} y2={bmiBandEdges[1]} fill={BMI_BAND_COLORS[1]} fillOpacity={0.08} stroke="none" ifOverflow="hidden" />,
        <ReferenceArea key="bmi-over" y1={bmiBandEdges[1]} y2={bmiBandEdges[2]} fill={BMI_BAND_COLORS[2]} fillOpacity={0.08} stroke="none" ifOverflow="hidden" />,
        <ReferenceArea key="bmi-obese" y1={bmiBandEdges[2]} fill={BMI_BAND_COLORS[3]} fillOpacity={0.08} stroke="none" ifOverflow="hidden" />,
      ]
    : null;

  // Goal line (task 6.2) — rendered at every zoom level, in both chart
  // branches below, same as the BMI bands. Unlike the bands, its value is
  // already folded into dayDomain/bandDomain above, so it's never clipped.
  const goalLine = latestGoalKg !== undefined
    ? <ReferenceLine y={latestGoalKg} stroke="var(--accent)" strokeDasharray="3 3" strokeWidth={1.5} label={{ value: 'Goal', position: 'insideTopRight', fill: 'var(--accent)', fontSize: 11 }} />
    : null;

  // Trend projection (task 7) — `undefined` (no line, no text) whenever no
  // goal is set (task 7.6) or there isn't enough data (task 7.3); otherwise
  // one of computeProjection's three outcomes, plus (only for 'on-track')
  // the extra fields extendedBucketBandData/projectionMessage need to render
  // the dashed line and ETA date. Computed independent of the active zoom
  // (design.md's "Zoom-independence is load-bearing") from a dedicated
  // fetch (projectionBucketRows, task 7.0), not whatever bucketed data the
  // current zoom happens to already have loaded.
  type Projection = ProjectionResult & { crossingDate?: Date; lastDayOffset?: number; slope?: number; intercept?: number };
  const projection: Projection | undefined = useMemo(() => {
    if (dataType !== 'weight' || latestGoalKg === undefined) return undefined;

    const times = allTimeWeightRecords.map(r => new Date(String(r.time)).getTime());
    const totalRecords = allTimeWeightRecords.length;
    const lifetimeSpanDays = totalRecords > 1
      ? (Math.max(...times) - Math.min(...times)) / (24 * 60 * 60 * 1000)
      : 0;

    const dates = projectionBucketRows.map(r => String(r.bucket_start));
    const fullEma = emaSeries(projectionBucketRows.map(r => num(r.avg)), 0.25);
    const window = last30DayEmaWindow(dates, fullEma);

    if (!hasEnoughDataForProjection(totalRecords, lifetimeSpanDays, window.length)) {
      return undefined;
    }

    const points = window.map(w => ({ x: toDayOffset(w.date), y: w.ema }));
    const { slope, intercept } = linearRegression(points);
    const windowStartEma = window[0].ema;
    const latestEma = window[window.length - 1].ema;
    const lastDayOffset = points[points.length - 1].x;

    const result = computeProjection({ slope, intercept, goal: latestGoalKg, windowStartEma, latestEma, lastDayOffset });
    if (result.status !== 'on-track' || result.crossingDayOffset === undefined) return result;

    return {
      ...result,
      crossingDate: new Date(result.crossingDayOffset * 24 * 60 * 60 * 1000),
      lastDayOffset,
      slope,
      intercept,
    };
  }, [dataType, latestGoalKg, allTimeWeightRecords, projectionBucketRows]);

  const projectionMessage = !projection ? null : (
    projection.status === 'reached' ? "You've reached your goal weight" :
    projection.status === 'not-on-track' ? 'Not on track at your current trend' :
    `On track to reach your goal around ${projection.crossingDate!.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' })}`
  );
  const noDataMessage = dataType === 'weight' && latestGoalKg !== undefined && projection === undefined
    ? 'Not enough data to project yet'
    : null;

  // Dashed projection line (task 7.5) renders only at Month/Year zoom; the
  // ETA/status text above renders at every zoom via projectionMessage/
  // noDataMessage regardless of zoom, so switching zoom never hides the
  // answer to the question the feature exists to answer (design.md).
  const showProjectionLine = dataType === 'weight' && (zoom === 'month' || zoom === 'year') && projection?.status === 'on-track';
  const projectionGranularity: 'day' | 'month' = zoom === 'year' ? 'month' : 'day';

  const extendedBucketBandData: BandRow[] = useMemo(() => {
    if (!showProjectionLine || !projection || projection.status !== 'on-track'
      || projection.crossingDayOffset === undefined || projection.lastDayOffset === undefined
      || projection.slope === undefined || projection.intercept === undefined) {
      return bucketBandData;
    }
    const pts = projectionPoints(
      projection.intercept, projection.slope, projection.lastDayOffset, projection.crossingDayOffset, projectionGranularity
    );
    const syntheticRows: BandRow[] = pts.map(p => ({
      label: bucketLabel(new Date(p.dayOffset * 24 * 60 * 60 * 1000).toISOString(), zoom),
      projection: p.value,
    }));
    if (bucketBandData.length === 0) return syntheticRows;
    // Seed the join point on the last real bucket so the dashed line
    // connects continuously from the solid trend line rather than jumping.
    const joined = bucketBandData.slice(0, -1).concat({
      ...bucketBandData[bucketBandData.length - 1],
      projection: bucketBandData[bucketBandData.length - 1].avg,
    });
    return [...joined, ...syntheticRows];
  }, [showProjectionLine, projection, bucketBandData, projectionGranularity, zoom]);

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
          {dataType === 'weight' && !showGoalForm && (
            <TapTarget
              type="button"
              onClick={() => setShowGoalForm(true)}
              className="rounded-md text-xs font-semibold uppercase tracking-wide bg-border text-text px-3 py-1.5"
            >
              Set goal
            </TapTarget>
          )}
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

        {dataType === 'weight' && showGoalForm && (
          <AddRecordForm
            type="weight_goal"
            onSuccess={() => { setShowGoalForm(false); setRefreshKey(k => k + 1); }}
            onCancel={() => setShowGoalForm(false)}
          />
        )}

        {isWritable && <AddRecordForm type={dataType} onSuccess={() => setRefreshKey(k => k + 1)} />}

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
                  {bmiBandAreas}
                  {goalLine}
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
              <ComposedChart data={dataType === 'weight' ? extendedBucketBandData : bucketBandData}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" opacity={0.5} />
                <XAxis dataKey="label" tick={{ fill: 'var(--text-muted)', fontSize: 11 }} />
                <YAxis domain={bandDomain} tick={{ fill: 'var(--text-muted)', fontSize: 11 }} tickFormatter={yAxisTickFormatter} />
                <Tooltip formatter={formatTooltipValue} />
                {dataType === 'weight' && <Legend wrapperStyle={{ fontSize: 12 }} />}
                {bmiBandAreas}
                {goalLine}
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
                {showProjectionLine && (
                  <Line
                    type="monotone"
                    dataKey="projection"
                    stroke="var(--accent)"
                    strokeWidth={1.5}
                    strokeDasharray="2 4"
                    dot={false}
                    name="Projection"
                  />
                )}
              </ComposedChart>
            )}
          </ResponsiveContainer>

          {dataType === 'weight' && (projectionMessage || noDataMessage) && (
            <p className="mt-3 text-xs text-text-muted">{projectionMessage ?? noDataMessage}</p>
          )}

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
            {bmi !== undefined && (
              <div>
                <p className="font-[family-name:var(--font-data)] text-[11px] font-bold uppercase tracking-wide text-text-muted mb-1">BMI</p>
                <p className="font-[family-name:var(--font-data)] text-base font-semibold text-text tabular-nums">{bmi.toFixed(1)} · {classifyBmi(bmi)}</p>
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
