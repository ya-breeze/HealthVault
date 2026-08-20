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
