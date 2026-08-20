'use client';
import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { usePathname } from 'next/navigation';
import { api, UserSettings } from '@/lib/api';
import { DICTIONARIES, LanguageCode, isSupportedLanguage } from '@/lib/i18n';
import { useSerialQueue } from '@/lib/useSerialQueue';

interface LanguageContextValue {
  language: LanguageCode;
  // Async and can reject (a failed PUT) — callers that show their own error
  // feedback should await and catch, matching every other settings-mutating
  // control in this app (see app/page.tsx's handleDone).
  setLanguage: (code: LanguageCode) => Promise<void>;
  t: (key: keyof typeof DICTIONARIES.en) => string;
  // Queues an arbitrary UserSettings PUT behind this same provider's
  // claim() queue — the same fresh read-modify-write api.updateSettings
  // does for setLanguage above, just for a caller-chosen patch. Lets any
  // other screen that writes to the shared UserSettings blob (currently
  // app/page.tsx's dashboard-order editor) serialize its writes against
  // setLanguage's instead of keeping an independent GET/PUT of its own —
  // two independent read-modify-writes on the same blob can otherwise race
  // and silently drop whichever save loses. Found in code review.
  updateSettings: (patch: Partial<UserSettings>) => Promise<UserSettings>;
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
  const pathname = usePathname();

  // Serializes every GET (this provider's own periodic settings load, right
  // below) and PUT (setLanguage) against this same UserSettings blob so they
  // can never interleave — see useSerialQueue's doc comment. This replaces a
  // hand-rolled generation-counter+pending-write pair that needed two
  // separate review-round fixes (a write whose PUT ultimately failed still
  // discarded a legitimate concurrent GET's result; and the counter closed
  // over `settings`, which defeated the context-value memoization below) —
  // both classes of bug are structurally impossible with a strict queue
  // instead: nothing this provider does can ever run out of issue order.
  const { claim } = useSerialQueue();

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
  //
  // Queued via claim() rather than fired directly: a client-side navigation
  // right after changing the language (before that PUT has settled) would
  // otherwise let this GET run concurrently with the write and land either
  // before or after it unpredictably — in the worst case, a stale read
  // resolving after the write silently reverts the just-saved language back
  // to its old value even though the PUT already succeeded server-side.
  // Queuing both this GET and setLanguage's PUT behind the same claim()
  // makes that impossible: this GET only ever starts once every
  // earlier-claimed operation (including a same-tick setLanguage write) has
  // already fully applied its own state update, so it reads — and
  // reapplies — that same current value, never a stale one.
  useEffect(() => {
    let cancelled = false;
    claim(async () => {
      if (cancelled) return;
      try {
        const s = await api.getSettings();
        if (cancelled) return;
        const raw = s.display_language;
        if (typeof raw === 'string' && isSupportedLanguage(raw)) {
          setLanguageState(raw);
        }
      } catch {
        // Stay on the English default — see doc comment above.
      }
    });
    return () => {
      cancelled = true;
    };
  }, [pathname, claim]);

  // Uses api.updateSettings — a fresh read-modify-write, not a merge onto
  // the possibly-stale cached `settings` state — since this provider and
  // app/page.tsx's dashboard-order editor each write to the same
  // UserSettings blob with no shared store between them. Without this,
  // saving a dashboard reorder and then switching language in the same
  // session (no navigation in between, so this provider's own cache never
  // refreshed) would PUT a stale pre-reorder snapshot here and silently
  // clobber the just-saved dashboard_order. A failed save rejects (including
  // when the refetch inside updateSettings is what failed) and Header shows a
  // toast; the language state is left as it was, so the control never claims a
  // preference the server didn't store. Queued via claim() — see the pathname
  // effect's doc comment above for the race this closes.
  const setLanguage = useCallback((code: LanguageCode): Promise<void> => {
    return claim(async () => {
      await api.updateSettings({ display_language: code });
      setLanguageState(code);
    });
  }, [claim]);

  // See the context value's doc comment. Deliberately not used by
  // setLanguage above — claim() can't be nested (claiming from inside an
  // already-claimed operation would wait on itself and never resolve) — so
  // setLanguage keeps its own inline claim() call instead of routing through
  // this.
  const updateSettings = useCallback((patch: Partial<UserSettings>): Promise<UserSettings> => {
    return claim(() => api.updateSettings(patch));
  }, [claim]);

  const t = useCallback(
    (key: keyof typeof DICTIONARIES.en) => DICTIONARIES[language][key],
    [language]
  );

  // Keeps the <html lang> attribute (static "en" markup in the root layout,
  // which has no wiring to this provider's runtime language state) in sync
  // with the user's actual Display Language, so screen readers and browser
  // translate/spell-check tooling treat Russian-rendered content as Russian
  // rather than as English — the exact audience this feature targets.
  useEffect(() => {
    document.documentElement.lang = language;
  }, [language]);

  // Memoized so consumers (Header, MealItemRow per item, ExpertModeToggle,
  // ReviewClient, custom/history pages) don't all re-render on every
  // navigation just because this provider re-renders (its own pathname
  // effect above fires on every route change) — without this, a fresh
  // object literal here would give the context value a new identity on
  // every render even though language/setLanguage/t are themselves already
  // stable/unchanged.
  const value = useMemo(
    () => ({ language, setLanguage, t, updateSettings }),
    [language, setLanguage, t, updateSettings]
  );

  return (
    <LanguageContext.Provider value={value}>
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
