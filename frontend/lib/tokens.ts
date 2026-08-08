import type { DataType } from './api';

/**
 * One CSS custom property per registered type (defined in globals.css),
 * shared by the dashboard vitals grid and every data page's own chart —
 * see dashboard-ui's "Consistent per-metric color" requirement.
 */
export function metricColorVar(type: DataType): string {
  return `var(--c-${type})`;
}
