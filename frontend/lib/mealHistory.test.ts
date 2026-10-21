import { describe, expect, it } from 'vitest';
import type { MealSummary } from './api';
import { canRepeatMeal } from './mealHistory';

const meal = (logged_at: string, status: MealSummary['status'] = 'confirmed'): MealSummary => ({
  can_repeat: true, id: 'meal', name: 'Lunch', logged_at, status,
  calories: 200, protein_grams: 10, carbs_grams: 20, fat_grams: 8,
});

describe('repeat eligibility in the account timezone', () => {
  it('includes six previous local dates even across the autumn DST change', () => {
    const now = new Date('2026-10-26T12:00:00Z');
    expect(canRepeatMeal(meal('2026-10-19T22:00:00Z'), 'Europe/Prague', now)).toBe(true);
    expect(canRepeatMeal(meal('2026-10-19T21:59:59Z'), 'Europe/Prague', now)).toBe(false);
  });

  it('uses the account date rather than the UTC date near midnight', () => {
    const now = new Date('2026-10-21T22:30:00Z');
    expect(canRepeatMeal(meal('2026-10-15T22:00:00Z'), 'Europe/Prague', now)).toBe(true);
    expect(canRepeatMeal(meal('2026-10-15T21:59:59Z'), 'Europe/Prague', now)).toBe(false);
  });

  it('excludes future or unconfirmed meals and malformed timestamps', () => {
    const now = new Date('2026-10-21T12:00:00Z');
    expect(canRepeatMeal(meal('2026-10-21T12:00:01Z'), 'UTC', now)).toBe(false);
    expect(canRepeatMeal(meal('2026-10-21T11:00:00Z', 'pending_review'), 'UTC', now)).toBe(false);
    expect(canRepeatMeal(meal('invalid'), 'UTC', now)).toBe(false);
  });

  it('hides repeat when the server finds an unusable source or supplies no capability', () => {
    const now = new Date('2026-10-21T12:00:00Z');
    const current = meal('2026-10-21T11:00:00Z');
    expect(canRepeatMeal({ ...current, can_repeat: false }, 'UTC', now)).toBe(false);
    expect(canRepeatMeal({ ...current, can_repeat: undefined }, 'UTC', now)).toBe(false);
  });

  it('falls back to UTC for an invalid timezone like other food views', () => {
    const now = new Date('2026-10-21T12:00:00Z');
    expect(canRepeatMeal(meal('2026-10-15T00:00:00Z'), 'invalid', now)).toBe(true);
    expect(canRepeatMeal(meal('2026-10-14T23:59:59Z'), 'invalid', now)).toBe(false);
  });
});
