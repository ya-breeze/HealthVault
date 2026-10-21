import type { MealSummary } from './api';
import { loggedDayKey } from './loggedDay';

// Seven local calendar dates, including today. Subtract calendar dates in
// UTC only after conversion so daylight-saving days can have 23 or 25 hours.
export function canRepeatMeal(meal: MealSummary, timezone: string, now = new Date()): boolean {
  const logged = new Date(meal.logged_at);
  if (meal.can_repeat !== true || meal.status !== 'confirmed' || !Number.isFinite(logged.getTime()) || logged > now) return false;
  const today = new Date(`${loggedDayKey(now, timezone)}T00:00:00Z`);
  today.setUTCDate(today.getUTCDate() - 6);
  return loggedDayKey(logged, timezone) >= today.toISOString().slice(0, 10);
}
