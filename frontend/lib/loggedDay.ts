// Mirrors the backend's LocalDate helper (backend/pkg/database/food_completeness.go):
// a meal's Logged Day is its timestamp converted into the caller's timezone and
// formatted YYYY-MM-DD. `en-CA` is the shortest built-in locale that formats dates
// that way, so it's used purely as a formatting trick, not for its regional meaning.
// An invalid/unsupported IANA zone name throws inside Intl.DateTimeFormat's
// constructor, so this falls back to UTC the same way the backend's
// `ResolveTimezone` does for a `time.LoadLocation` rejection — never throws itself.
export function loggedDayKey(d: Date, tz: string | undefined): string {
  try {
    return new Intl.DateTimeFormat('en-CA', { timeZone: tz || 'UTC' }).format(d);
  } catch {
    return new Intl.DateTimeFormat('en-CA', { timeZone: 'UTC' }).format(d);
  }
}

// Human-readable label for the same Logged Day `loggedDayKey` computes, so a
// day section's visible heading always names the calendar day its meals were
// actually grouped under (the stored `timezone`), rather than diverging from
// it by falling back to the browser's own zone. Same invalid-zone fallback as
// loggedDayKey.
export function loggedDayLabel(d: Date, locale: string | undefined, tz: string | undefined): string {
  const options: Intl.DateTimeFormatOptions = { weekday: 'long', month: 'short', day: 'numeric' };
  try {
    return d.toLocaleDateString(locale, { ...options, timeZone: tz || 'UTC' });
  } catch {
    return d.toLocaleDateString(locale, { ...options, timeZone: 'UTC' });
  }
}
