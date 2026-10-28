import { describe, expect, it, vi } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import type { FoodMeal } from '@/lib/api';
import { DICTIONARIES } from '@/lib/i18n';
import RecentMealSuggestion from './RecentMealSuggestion';

vi.mock('@/components/LanguageContext', () => ({
  useLanguage: () => ({ language: 'en', t: (key: keyof typeof DICTIONARIES.en) => DICTIONARIES.en[key] }),
}));

const meal: FoodMeal = {
  id: 'target', status: 'pending_review', logged_at: '2026-10-21T12:00:00Z', name: 'Lunch',
  history_version: 'current', clarify_round: 0, clarify_log: '',
  calories: 0, protein_grams: 0, carbs_grams: 0, fat_grams: 0, sugar_grams: 0,
  sodium_grams: 0, dietary_fiber_grams: 0, saturated_fat_grams: 0,
  items: [{
    id: 'target-item', meal_id: 'target', name: 'New cutlet', preparation: '', state: '',
    macro_source: 'estimated', weight_grams: 180, confidence: 0.8,
    calories: 0, protein_grams: 0, carbs_grams: 0, fat_grams: 0, sugar_grams: 0,
    sodium_grams: 0, dietary_fiber_grams: 0, saturated_fat_grams: 0,
  }],
  history_suggestion: {
    source_meal_id: 'source', source_name: 'Previous lunch', source_logged_at: '2026-10-20T12:00:00Z',
    source_items: [
      { name: 'Meat cutlet', weight_grams: 100, macro_source: 'manual' },
      { name: 'Steamed vegetables', weight_grams: 200, macro_source: 'estimated' },
    ],
    mappings: [{ item_id: 'target-item', source_item_id: 'source-item', source_name: 'Meat cutlet' }],
  },
};

describe('recent meal source presentation', () => {
  it('does not present an unnamed target as the source food', () => {
    const unnamed: FoodMeal = { ...meal, items: [{ ...meal.items![0], name: '', canonical_name: '' }] };
    const html = renderToStaticMarkup(<RecentMealSuggestion meal={unnamed} timezone="UTC" busy={false}
      onAction={async () => unnamed} onRefresh={async () => unnamed} />);
    expect(html).toContain('Meat cutlet → Unnamed food · 180 g');
    expect(html).not.toContain('Meat cutlet → Meat cutlet');
  });
  it('uses the canonical name when the target display name is empty', () => {
    const withEmptyName: FoodMeal = { ...meal, items: [{ ...meal.items![0], name: '', canonical_name: 'meat cutlet' }] };
    const html = renderToStaticMarkup(<RecentMealSuggestion meal={withEmptyName} timezone="UTC" busy={false}
      onAction={async () => withEmptyName} onRefresh={async () => withEmptyName} />);
    expect(html).toContain('Meat cutlet → meat cutlet · 180 g');
  });
  it('shows the entire source composition and its uncertainty separately from current matched portions', () => {
    const html = renderToStaticMarkup(<RecentMealSuggestion meal={meal} timezone="UTC" busy={false}
      onAction={async () => meal} onRefresh={async () => meal} />);
    const composition = html.split('data-testid="history-suggestion-composition"')[1].split('</ul>')[0];
    const mappings = html.split('data-testid="history-suggestion-mappings"')[1].split('</ul>')[0];
    expect(composition).toContain('Meat cutlet · 100 g');
    expect(composition).toContain('Steamed vegetables · 200 g');
    expect(composition).toContain('AI estimate');
    expect(mappings).toContain('Meat cutlet → New cutlet · 180 g');
    expect(mappings).not.toContain('Steamed vegetables');
    expect(html).toContain('Estimates stay estimates when reused.');
  });
});
