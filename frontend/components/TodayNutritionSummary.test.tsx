import { describe, expect, it, vi } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import { DICTIONARIES } from '@/lib/i18n';
import type { TodaySummary } from '@/lib/api';
import TodayNutritionSummary from './TodayNutritionSummary';

vi.mock('./LanguageContext', () => ({useLanguage: () => ({language: 'en', t: (key: keyof typeof DICTIONARIES.en) => DICTIONARIES.en[key]})}));

const summary: TodaySummary = {
  date: '2026-10-14', calories_consumed: 1200, protein_grams_consumed: 80,
  carbs_grams_consumed: 130, fat_grams_consumed: 40, dietary_fiber_grams_consumed: 18.5,
  meal_count: 3, last_logged_at: null, display_language: 'en',
  target: {available: false, reason: 'missing_goal_weight', dietary_fiber_grams: 25},
};

describe('today nutrition presentation', () => {
  it('keeps consumed nutrition and independent fiber when calorie targets are unavailable', () => {
    const html = renderToStaticMarkup(<TodayNutritionSummary summary={summary}/>);
    expect(html).toContain('Fiber 18.5 / 25 g');
    expect(html).toContain('Protein 80 g');
    expect(html).toContain('1,200 kcal');
    expect(html).not.toContain('NaN');
  });
  it('distinguishes unknown fiber from a known zero and never invents an unavailable target', () => {
    const unknown = renderToStaticMarkup(<TodayNutritionSummary summary={{...summary, dietary_fiber_grams_consumed: null, target: {...summary.target, dietary_fiber_grams: null}}}/>);
    expect(unknown).toContain('Fiber — g');
    expect(unknown).not.toContain('25');
    const zero = renderToStaticMarkup(<TodayNutritionSummary summary={{...summary, dietary_fiber_grams_consumed: 0}}/>);
    expect(zero).toContain('Fiber 0 / 25 g');
  });
  it('keeps an above-target fiber value while limiting only its visual bar', () => {
    const html = renderToStaticMarkup(<TodayNutritionSummary summary={{...summary, dietary_fiber_grams_consumed: 35}}/>);
    expect(html).toContain('Fiber 35 / 25 g');
    expect(html).toContain('width:100%');
  });
});
