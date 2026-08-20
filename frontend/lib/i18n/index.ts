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

// Resolves an arbitrary stored `display_language` to the dictionary to render
// in, or null when this app ships none for it (the caller then falls back to
// English).
//
// Deliberately compares only the primary subtag, case-insensitively and after
// trimming, rather than requiring the whole string to equal 'en'/'ru'. That is
// exactly what the backend's vision.IsEnglishDisplayLanguage does, and the two
// must agree: `display_language` lives in an opaque, unvalidated settings blob
// that `PUT /users/me/settings` will store verbatim, so a non-frontend caller
// can put a regional tag like "ru-RU" in it. Under the previous whole-string
// check that produced a split-brain session — the frontend saw an unsupported
// value and rendered the English UI, while the backend read the same value as
// non-English and both asked the vision model for Russian Display Names and
// silently suppressed USDA/Open Food Facts matching for that user. Found in
// code review.
export function resolveLanguage(code: string): LanguageCode | null {
  const primary = code.trim().split(/[-_]/, 1)[0].toLowerCase();
  return primary === 'en' || primary === 'ru' ? primary : null;
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
