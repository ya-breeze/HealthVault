// Splits an inclusive [from, to] YYYY-MM-DD range into consecutive windows of
// at most `maxDays` days each, so a "load older" history page spanning more
// than the backend's 92-day GET /api/food/completeness cap (design.md §6
// "Frontend surface") can still be covered with one call per window instead
// of a single oversized call the backend would 400.
//
// Dates are parsed as UTC midnights (not through the browser's local
// timezone) since they're already resolved Logged-Day strings — re-parsing
// them through local time could shift the calendar date on a browser set to
// a zone behind UTC.
function parseISODate(s: string): Date {
  return new Date(`${s}T00:00:00Z`);
}

function formatISODate(d: Date): string {
  return d.toISOString().slice(0, 10);
}

function addDaysISO(s: string, days: number): string {
  const d = parseISODate(s);
  d.setUTCDate(d.getUTCDate() + days);
  return formatISODate(d);
}

export interface DateWindow {
  from: string;
  to: string;
}

export function splitRangeIntoWindows(from: string, to: string, maxDays = 92): DateWindow[] {
  const windows: DateWindow[] = [];
  let windowStart = from;
  while (windowStart <= to) {
    const maxEnd = addDaysISO(windowStart, maxDays - 1);
    const windowEnd = maxEnd < to ? maxEnd : to;
    windows.push({ from: windowStart, to: windowEnd });
    windowStart = addDaysISO(windowEnd, 1);
  }
  return windows;
}
