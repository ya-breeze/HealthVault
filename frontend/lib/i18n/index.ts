import type { DataType } from '@/lib/api';
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

// The locale to format dates and times for, given a Display Language.
//
// Russian maps to a concrete locale; English deliberately maps to `undefined`,
// which tells Intl to use the browser's own locale. That asymmetry is the
// point rather than an oversight. "Russian" settles how a date should read, so
// deferring to a device that may be configured in another language would leave
// English weekday and month names on an otherwise Russian page — the whole
// premise of this setting is that the two can differ. "English" settles much
// less: it does not say whether a date is month-first or day-first, and the
// user's device is the better authority on that. Pinning English to a concrete
// locale here would silently switch existing en-GB and en-AU users to US date
// order, a regression this change has no reason to cause.
export function dateLocaleFor(language: LanguageCode): string | undefined {
  return language === 'ru' ? 'ru' : undefined;
}

// Selects among a counted noun's plural forms using the Display Language's own
// plural categories.
//
// Not an `n === 1` test: that happens to work for English, which has two
// categories, and is wrong for most values in Russian, which has four. Russian
// takes one form for 1, 21, 31…, another for 2–4, 22–24…, and another for 5–20
// and most other numbers — so a one-vs-many test picks the wrong wording for
// every count from 2 to 4, which for a "meals need attention" badge are the
// common ones. Intl.PluralRules ships with the platform and knows the rules;
// the caller supplies one string per category.
//
// `language` doubles as the locale here — 'en' and 'ru' are themselves valid
// language tags — deliberately *not* reusing dateLocaleFor above, whose
// `undefined` would resolve English plural categories against a browser that
// might be set to Russian.
export function pluralForm(
  language: LanguageCode,
  n: number,
  forms: { one: string; few: string; many: string; other: string },
): string {
  const category = new Intl.PluralRules(language).select(n);
  return forms[category as keyof typeof forms] ?? forms.other;
}

// Substitutes {name} placeholders in a dictionary string. Keeps interpolated
// values inside the translated sentence instead of concatenating fragments at
// the call site, which is what lets a translation put the number or the metric
// name where its own grammar needs it rather than where English puts it.
export function interpolate(template: string, vars: Record<string, string | number>): string {
  return template.replace(/\{(\w+)\}/g, (whole, key) =>
    key in vars ? String(vars[key]) : whole,
  );
}

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
// A metric's display name, keyed by its DataType. Takes `t` as a parameter
// rather than calling useLanguage itself, matching mealStatusLabel below, so
// it stays usable from non-component code.
//
// Deliberately cast-free. `metric.${DataType}` expands to a union of one
// literal type per member of DATA_TYPES, so this compiles only while the
// dictionary declares a `metric.` key for every data type — adding a data type
// without its two entries is a type error at this line rather than a metric
// silently rendering its raw key at runtime. There is no frontend unit-test
// runner in this project, so letting the compiler carry the invariant is worth
// more here than a comment promising a test does it.
export function metricLabel(t: (key: keyof Dictionary) => string, type: DataType): string {
  return t(`metric.${type}`);
}

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
