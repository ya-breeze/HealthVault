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

// Clock time within the Logged Day the two functions above compute. A meal row
// sits under a header that already names its day, so the row needs only the
// time — but it needs it in the *stored* zone. Formatted in the browser's zone
// instead, a meal can show a time belonging to a different calendar day than
// the header it is listed under: an account on UTC read from a +02:00 browser
// renders a 23:00 meal as 01:00 beneath a header naming the previous day. Same
// invalid-zone fallback as loggedDayKey.
export function loggedDayTime(d: Date, locale: string | undefined, tz: string | undefined): string {
  // Both fields 2-digit so the column aligns down a day's meal list, which is
  // the same reason the calories beside it carry `tabular-nums`. It matters
  // most in Russian and other 24-hour locales, where `hour: 'numeric'` yields
  // an unpadded "2:00" that no other time in the list lines up with.
  const options: Intl.DateTimeFormatOptions = { hour: '2-digit', minute: '2-digit' };
  try {
    return d.toLocaleTimeString(locale, { ...options, timeZone: tz || 'UTC' });
  } catch {
    return d.toLocaleTimeString(locale, { ...options, timeZone: 'UTC' });
  }
}
