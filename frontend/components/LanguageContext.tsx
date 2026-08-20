'use client';
import { createContext, useCallback, useContext, useEffect, useState } from 'react';
import { usePathname } from 'next/navigation';
import { api, UserSettings } from '@/lib/api';
import { DICTIONARIES, LanguageCode, isSupportedLanguage } from '@/lib/i18n';

interface LanguageContextValue {
  language: LanguageCode;
  // Async and can reject (a failed PUT) — callers that show their own error
  // feedback should await and catch, matching every other settings-mutating
  // control in this app (see app/page.tsx's handleDone).
  setLanguage: (code: LanguageCode) => Promise<void>;
  t: (key: keyof typeof DICTIONARIES.en) => string;
}

const LanguageContext = createContext<LanguageContextValue | null>(null);

// App-wide Display Language provider. Loads display_language from the
// user's settings blob on mount, defaulting to English when unset, absent,
// or the load itself fails — a failed background load isn't something the
// user needs to act on (unlike a failed save, which does show a toast at
// its call site), and English is always a safe, working fallback. See
// openspec/specs/display-language "Per-User Display Language Setting" and
// design.md decision 6.
export function LanguageProvider({ children }: { children: React.ReactNode }) {
  const [language, setLanguageState] = useState<LanguageCode>('en');
  const [settings, setSettings] = useState<UserSettings>({});
  const pathname = usePathname();

  // Renders in English immediately rather than blocking the whole app
  // (including the unauthenticated /login page, where this fetch always
  // 401s) behind this request — the language updates in place once it
  // resolves, the same "render now, reconcile shortly after" pattern
  // app/page.tsx already uses for dashboard_order.
  //
  // Refetches on every pathname change, not just once on mount: this
  // provider lives in the root layout, which the App Router keeps mounted
  // across client-side navigations — including the redirect from /login to
  // an authenticated page — so a mount-only fetch would run once, while
  // still unauthenticated on /login, 401 silently, and never retry, leaving
  // the language stuck on English for the rest of the session even after a
  // successful login. Re-running per navigation is cheap (one GET, already
  // in flight for other reasons on most pages) and matches how Header's own
  // api.me() check already re-verifies auth on every page mount.
  useEffect(() => {
    api.getSettings()
      .then(s => {
        setSettings(s);
        const raw = s.display_language;
        if (typeof raw === 'string' && isSupportedLanguage(raw)) {
          setLanguageState(raw);
        }
      })
      .catch(() => {
        // Stay on the English default — see doc comment above.
      });
  }, [pathname]);

  // Reads a fresh copy of settings immediately before writing, rather than
  // merging onto the possibly-stale cached `settings` state: this provider
  // and app/page.tsx's dashboard-order editor each keep their own
  // independent cached UserSettings and PUT a read-modify-write built from
  // it, with no shared store between them. Without this, saving a dashboard
  // reorder and then switching language in the same session (no navigation
  // in between, so this provider's own cache never refreshed) would PUT a
  // stale pre-reorder snapshot here and silently clobber the just-saved
  // dashboard_order. Falls back to the cached copy only if the refetch
  // itself fails, matching this component's existing "a failed background
  // load isn't fatal" stance.
  const setLanguage = useCallback(async (code: LanguageCode) => {
    const current = await api.getSettings().catch(() => settings);
    const next: UserSettings = { ...current, display_language: code };
    await api.putSettings(next);
    setSettings(next);
    setLanguageState(code);
  }, [settings]);

  const t = useCallback(
    (key: keyof typeof DICTIONARIES.en) => DICTIONARIES[language][key],
    [language]
  );

  return (
    <LanguageContext.Provider value={{ language, setLanguage, t }}>
      {children}
    </LanguageContext.Provider>
  );
}

export function useLanguage(): LanguageContextValue {
  const ctx = useContext(LanguageContext);
  if (!ctx) {
    throw new Error('useLanguage must be used within a LanguageProvider');
  }
  return ctx;
}
