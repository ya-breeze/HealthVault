import type { WeatherHistory, WeatherHour, WeatherGap } from './api';
import { loggedDayKey } from './loggedDay';

export const WEATHER_UNITS = {
  temperature_c: '°C', apparent_temperature_c: '°C', relative_humidity_percent: '%',
  surface_pressure_hpa: 'hPa', mean_sea_level_pressure_hpa: 'hPa', precipitation_mm: 'mm', wind_speed_kmh: 'km/h',
} as const;
export function weatherTimezone(value: unknown): string {
  if (typeof value !== 'string' || !value) return 'UTC';
  try { return new Intl.DateTimeFormat('en', { timeZone: value }).resolvedOptions().timeZone; } catch { return 'UTC'; }
}
export function validWeatherDate(value: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return false;
  const ms = Date.parse(`${value}T00:00:00Z`);
  return Number.isFinite(ms) && new Date(ms).toISOString().slice(0, 10) === value;
}
export function weatherDayBounds(date: string, timezone: string): { from: number; to: number } {
  if (!validWeatherDate(date)) throw new Error('Invalid weather date');
  const zone = weatherTimezone(timezone);
  const nominal = Date.parse(`${date}T00:00:00Z`);
  // Search actual calendar boundaries, rather than adding 24 hours: DST
  // changes and fractional UTC offsets alter the instants of local midnight.
  function boundary(after: boolean): number {
    let low = nominal - 36 * 3600000, high = nominal + 36 * 3600000;
    while (low < high) {
      const middle = Math.floor((low + high) / 2);
      const key = loggedDayKey(new Date(middle), zone);
      if (after ? key > date : key >= date) high = middle; else low = middle + 1;
    }
    return low;
  }
  const from = boundary(false), to = boundary(true);
  if (from === to || loggedDayKey(new Date(from), zone) !== date) throw new Error('Calendar day does not exist');
  return { from, to };
}
export function weatherQueryBounds(date: string, timezone: string, now: Date) {
  const day = weatherDayBounds(date, timezone);
  const from = Math.floor(day.from / 3600000) * 3600000;
  const to = Math.min(Math.ceil(day.to / 3600000) * 3600000, now.getTime());
  return { day, from: new Date(from).toISOString(), to: new Date(to).toISOString(), elapsed: to > from };
}
export function latestWeather(history: WeatherHistory, now?: Date): WeatherHour | null {
  const hours = now ? history.hours.filter(h => Date.parse(h.hour) >= now.getTime() - 7 * 86400000 && Date.parse(h.hour) + 3600000 <= now.getTime()) : history.hours;
  return hours.reduce<WeatherHour | null>((latest, hour) => !latest || Date.parse(hour.hour) > Date.parse(latest.hour) ? hour : latest, null);
}
export function validateWeatherHistory(history: WeatherHistory): WeatherHistory {
  if (!history || !Array.isArray(history.hours) || !Array.isArray(history.gaps)) throw new Error('Invalid weather history');
  for (const [key, unit] of Object.entries(WEATHER_UNITS)) {
    if (history.units?.[key] !== unit) throw new Error('Unexpected weather units');
  }
  const seen = new Set<number>();
  for (const hour of history.hours) {
    const time = Date.parse(hour.hour);
    if (!Number.isFinite(time) || time % 3600000 !== 0 || seen.has(time) ||
      Object.keys(WEATHER_UNITS).some(key => !Number.isFinite(hour[key as keyof typeof WEATHER_UNITS])) ||
      typeof hour.source !== 'string' || typeof hour.model !== 'string' ||
      !Number.isFinite(hour.latitude) || hour.latitude < -90 || hour.latitude > 90 ||
      !Number.isFinite(hour.longitude) || hour.longitude < -180 || hour.longitude > 180 ||
      typeof hour.fetched_at !== 'string' || !Number.isFinite(Date.parse(hour.fetched_at))) throw new Error('Invalid weather hour');
    seen.add(time);
  }
  for (const gap of history.gaps) {
    if (!Number.isFinite(Date.parse(gap.from)) || !Number.isFinite(Date.parse(gap.to)) || Date.parse(gap.from) >= Date.parse(gap.to) || typeof gap.reason !== 'string') throw new Error('Invalid weather gap');
  }
  return history;
}
export function dayWeather(history: WeatherHistory, from: number, to: number) {
  const hours = history.hours.filter(h => Date.parse(h.hour) < to && Date.parse(h.hour) + 3600000 > from).sort((a,b) => Date.parse(a.hour) - Date.parse(b.hour));
  const gaps: WeatherGap[] = history.gaps.filter(g => Date.parse(g.from) < to && Date.parse(g.to) > from).map(g => ({...g, from: new Date(Math.max(from, Date.parse(g.from))).toISOString(), to: new Date(Math.min(to, Date.parse(g.to))).toISOString()}));
  const byTime = new Map(hours.map(h => [Date.parse(h.hour), h]));
  const points = [];
  for (let time = Math.floor(from / 3600000) * 3600000; time < to; time += 3600000) {
    const hour = byTime.get(time);
    points.push({ time, temperature: hour?.temperature_c ?? null, apparent: hour?.apparent_temperature_c ?? null, pressure: hour?.surface_pressure_hpa ?? null, seaPressure: hour?.mean_sea_level_pressure_hpa ?? null });
  }
  return { hours, gaps, points };
}
