'use client';
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react';
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

  // Holds setLanguage's in-flight GET+PUT, if any — see the pathname effect
  // below for why.
  const pendingWrite = useRef<Promise<unknown> | null>(null);
  // Bumped at the start of every setLanguage write, and compared by the
  // pathname effect before vs. after its own GET — see that effect's doc
  // comment for the specific race this closes that awaiting pendingWrite
  // alone does not: a write that starts (and, per pendingWrite, may even
  // finish) *after* the effect's GET is already on the wire.
  const writeGeneration = useRef(0);

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
  // Awaits any setLanguage write already in flight before issuing its own
  // GET, rather than racing it: a client-side navigation fired right after
  // changing the language (before that PUT has settled) would otherwise let
  // this effect's independent read return a pre-write snapshot and silently
  // revert the just-saved language back to its old value once it resolves,
  // even though the PUT already succeeded server-side — found in code
  // review after the sibling dashboard-order/language lost-update race
  // (below) was fixed. Waiting for the pending write first guarantees this
  // GET only ever runs after that write's own state update has already
  // landed, so it reads (and reapplies) the same value, not a stale one.
  //
  // That await alone only covers a write already in flight *before* this
  // GET starts, though — found in a later review round: if setLanguage's
  // write instead starts (and, since it's typically fast, even finishes)
  // while this GET is still on the wire, pendingWrite.current was null when
  // checked above, so nothing was awaited, and this GET's now-stale result
  // would still land on top of that write's already-applied state once it
  // resolves. writeGeneration closes that: captured before the GET starts
  // and compared after it resolves, a mismatch means some write touched
  // language state in between, so this GET's result is discarded — that
  // write's own state update is authoritative and already correct.
  useEffect(() => {
    let cancelled = false;
    (async () => {
      if (pendingWrite.current) {
        await pendingWrite.current.catch(() => {});
      }
      if (cancelled) return;
      const genAtStart = writeGeneration.current;
      try {
        const s = await api.getSettings();
        if (cancelled || writeGeneration.current !== genAtStart) return;
        setSettings(s);
        const raw = s.display_language;
        if (typeof raw === 'string' && isSupportedLanguage(raw)) {
          setLanguageState(raw);
        }
      } catch {
        // Stay on the English default — see doc comment above.
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [pathname]);

  // Uses api.updateSettings — a fresh read-modify-write, not a merge onto
  // the possibly-stale cached `settings` state — since this provider and
  // app/page.tsx's dashboard-order editor each write to the same
  // UserSettings blob with no shared store between them. Without this,
  // saving a dashboard reorder and then switching language in the same
  // session (no navigation in between, so this provider's own cache never
  // refreshed) would PUT a stale pre-reorder snapshot here and silently
  // clobber the just-saved dashboard_order. Falls back to the cached copy
  // only if the refetch itself fails, matching this component's existing
  // "a failed background load isn't fatal" stance.
  // Bumps writeGeneration and publishes the write via pendingWrite so the
  // effect above — whether already running or starting concurrently — waits
  // for or discards in favor of this write instead of racing it; see that
  // effect's comment.
  const setLanguage = useCallback(async (code: LanguageCode) => {
    writeGeneration.current += 1;
    const write = (async () => {
      const next = await api.updateSettings({ display_language: code }, settings);
      setSettings(next);
      setLanguageState(code);
    })();
    pendingWrite.current = write;
    try {
      await write;
    } finally {
      if (pendingWrite.current === write) {
        pendingWrite.current = null;
      }
    }
  }, [settings]);

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
  const value = useMemo(() => ({ language, setLanguage, t }), [language, setLanguage, t]);

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
