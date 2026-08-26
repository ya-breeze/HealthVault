import type { DataType } from './api';
import type { Dictionary } from './i18n';
import { formatMetricValue, toDisplayUnit } from './dataTypeMeta';

/**
 * A dashboard card's identifier: either a `DataType`-backed Vital Card, or a
 * card with no presence signal of its own — currently just the Logging Gap
 * Card (design.md decision 8). This union is a small, additive widening over
 * the old `DataType`-only registry, kept deliberately narrow rather than a
 * general "card kind" system: if a future card needs a third shape, the union
 * grows again then.
 */
export type CardId = DataType | 'logging_gap';

/**
 * The metrics shown as full vitals-grid cards on the dashboard, in display
 * order. Includes the Logging Gap Card, the registry's first non-`DataType`
 * entry (design.md decision 8) — it has no `/api/data/{type}` presence to
 * gate on, so it's always eligible to render, subject only to the user's own
 * hidden/visible choice (see `hasCardPresence`).
 *
 * Type only, no label: display names now come from the `metric.<type>` keys in
 * lib/i18n so they can be translated. Keeping an English label here as well
 * would give each metric two names with nothing keeping them in step, and the
 * one the dashboard actually renders would be the other one.
 */
export const PRIMARY_METRICS: { type: CardId }[] = [
  { type: 'steps' },
  { type: 'heart_rate' },
  { type: 'sleep' },
  { type: 'heart_rate_variability' },
  { type: 'distance' },
  { type: 'weight' },
  { type: 'blood_pressure' },
  { type: 'oxygen_saturation' },
  { type: 'logging_gap' },
];

/**
 * One vitals-grid card as the user has arranged it: which card, and whether
 * they've hidden it. `hidden` cards keep their position in this list so that
 * re-showing one restores it where it was rather than appending it at the end.
 */
export interface DashboardCardPref {
  type: CardId;
  hidden: boolean;
}

/**
 * A single entry as it may appear in the stored `dashboard_order` setting.
 * The plain string is the pre-visibility shape: before per-card show/hide
 * existed the setting was just a list of metric types. Both shapes are read
 * (see reconcileMetricOrder); only the object shape is written.
 */
export type StoredCardPref = string | { type?: unknown; hidden?: unknown };

/**
 * Reconciles a saved dashboard_order (from user settings) against
 * PRIMARY_METRICS: known types are reordered to match the saved order,
 * unknown/removed types are dropped, and any current metric missing from the
 * saved order is appended at the end, visible. Returns every PRIMARY_METRIC
 * visible in default order when there's no saved order yet (a user who hasn't
 * customized).
 *
 * Accepts both stored shapes. A plain-string entry is the pre-visibility
 * shape and normalizes to `{ type, hidden: false }`, so a user who saved an
 * order before show/hide existed keeps that order with nothing hidden. Note
 * this tolerance is one-way: an older build reading the object shape resolves
 * every entry to undefined and silently falls back to the default order (see
 * the change's design.md — rollback is lossy).
 */
export function reconcileMetricOrder(saved: StoredCardPref[] | undefined): DashboardCardPref[] {
  const defaults = (): DashboardCardPref[] => PRIMARY_METRICS.map(m => ({ type: m.type, hidden: false }));
  if (!Array.isArray(saved) || saved.length === 0) return defaults();

  const known = new Set<string>(PRIMARY_METRICS.map(m => m.type as string));
  const seen = new Set<string>();
  const ordered: DashboardCardPref[] = [];
  for (const entry of saved) {
    // Unwrap either shape into a type + hidden pair, ignoring anything that
    // isn't one of them — a malformed settings blob must not break the grid.
    let type: unknown;
    let hidden = false;
    if (typeof entry === 'string') {
      type = entry;
    } else if (entry && typeof entry === 'object') {
      type = entry.type;
      hidden = entry.hidden === true;
    }
    if (typeof type !== 'string' || !known.has(type) || seen.has(type)) continue;
    seen.add(type);
    ordered.push({ type: type as CardId, hidden });
  }

  const missing = PRIMARY_METRICS.filter(m => !seen.has(m.type)).map(m => ({ type: m.type, hidden: false }));
  return [...ordered, ...missing];
}

/**
 * Whether `type` should be treated as having data, per the presence map
 * returned by `api.dataTypesPresence()`. Fails open in both documented ways:
 * a `null` map (the fetch failed) and a map missing an entry for `type` (an
 * incomplete-but-successful response) both resolve to "present" rather than
 * "absent" — only an explicit `false` hides a type. See the
 * hide-unrecorded-data-types change's design.md, "Fail-open contract".
 */
export function hasPresence(presence: Record<string, boolean> | null, type: string): boolean {
  if (!presence) return true;
  const value = presence[type];
  return value === undefined ? true : value;
}

/**
 * Presence gate for a dashboard card (`CardId`, not just `DataType`).
 * `'logging_gap'` has no presence signal of its own (design.md decision 8) —
 * this SHALL NOT fall through to `hasPresence` for it, so a presence
 * response that omits it, or one that (incorrectly) returns `false` for it,
 * has no effect on whether the card is eligible to render. Every other
 * `CardId` delegates to `hasPresence` unchanged.
 */
export function hasCardPresence(presence: Record<string, boolean> | null, type: CardId): boolean {
  if (type === 'logging_gap') return true;
  return hasPresence(presence, type);
}

/**
 * The non-primary data types shown in the dashboard's "More Data" section:
 * every `DataType` not already registered as a primary vitals-grid card.
 * Takes the full `DataType` list as a parameter (rather than importing
 * `DATA_TYPES` itself) purely so callers can pass a fixture in tests;
 * `'logging_gap'` is never a candidate since it isn't a member of `allTypes`
 * to begin with — this filter can't accidentally surface it as a secondary
 * type.
 */
export function secondaryTypes(allTypes: readonly DataType[]): DataType[] {
  return allTypes.filter(t => !PRIMARY_METRICS.some(m => m.type === t));
}

export interface VitalResult {
  value: string;
  // A dictionary key, not the unit text itself: this module is a pure data
  // transform with no access to `t`, so it names the unit and VitalCard
  // renders it. Russian writes these units in Cyrillic ("уд/мин", "км", "ч"),
  // so passing the literal string through would leave an English fragment
  // beside every value on an otherwise translated card — the same reason the
  // meal macro line routes "kcal" and "g" through the dictionary.
  unit?: keyof Dictionary;
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
      return { value: formatMetricValue('distance', series[series.length - 1]), unit: 'unit.km', sparkline: series, trend: trendFrom(series) };
    }
    case 'sleep': {
      const series = rows.map(r => toDisplayUnit('sleep', num(r.sum)));
      return { value: formatMetricValue('sleep', series[series.length - 1]), unit: 'unit.h', sparkline: series, trend: trendFrom(series) };
    }
    case 'heart_rate': {
      const series = rows.map(r => num(r.avg));
      return { value: formatMetricValue('heart_rate', series[series.length - 1]), unit: 'unit.bpm', sparkline: series, trend: trendFrom(series) };
    }
    case 'heart_rate_variability': {
      const series = rows.map(r => num(r.avg));
      return { value: formatMetricValue('heart_rate_variability', series[series.length - 1]), unit: 'unit.ms', sparkline: series, trend: trendFrom(series) };
    }
    case 'weight': {
      const series = rows.map(r => num(r.avg));
      return { value: formatMetricValue('weight', series[series.length - 1]), unit: 'unit.kg', sparkline: series, trend: trendFrom(series) };
    }
    case 'oxygen_saturation': {
      const series = rows.map(r => num(r.avg));
      return { value: formatMetricValue('oxygen_saturation', series[series.length - 1]), unit: 'unit.percent', sparkline: series, trend: trendFrom(series) };
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
