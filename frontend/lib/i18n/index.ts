import en from './en';
import ru from './ru';

// ru.ts is typed against this (import type — erased at compile time, so it
// doesn't create a runtime circular import with the DICTIONARIES map below).
export type Dictionary = typeof en;

export type LanguageCode = 'en' | 'ru';

export const DICTIONARIES: Record<LanguageCode, Dictionary> = { en, ru };

// Both dictionaries ship in every build — no per-locale chunk loading at
// this scale (two languages). See design.md decision 6.
export const SUPPORTED_LANGUAGES: { code: LanguageCode; label: string }[] = [
  { code: 'en', label: 'English' },
  { code: 'ru', label: 'Русский' },
];

export function isSupportedLanguage(code: string): code is LanguageCode {
  return code === 'en' || code === 'ru';
}

// Shared by the review and history screens, which both render a
// FoodMeal.status badge — kept here instead of duplicated in each so the
// status set can't drift between the two.
export function mealStatusLabel(t: (key: keyof Dictionary) => string, status: string): string {
  switch (status) {
    case 'processing':
      return t('status.processing');
    case 'pending_clarification':
      return t('status.pending_clarification');
    case 'pending_review':
      return t('status.pending_review');
    case 'confirmed':
      return t('status.confirmed');
    case 'failed':
      return t('status.failed');
    default:
      return status;
  }
}

// A lookup function rather than an object literal built from `t` at each
// call site (MealItemRow's prior shape) — matches mealStatusLabel above, and
// avoids allocating a fresh 4-key object on every render for what's always
// just one lookup.
export function macroSourceLabel(t: (key: keyof Dictionary) => string, source: string): string {
  switch (source) {
    case 'reference':
      return t('item.sourceReference');
    case 'manual':
      return t('item.sourceManual');
    case 'estimated':
      return t('item.sourceEstimated');
    case 'none':
      return t('item.sourceNone');
    default:
      return source;
  }
}
