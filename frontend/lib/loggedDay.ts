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
