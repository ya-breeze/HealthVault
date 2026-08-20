import { useEffect, useRef, type RefObject } from 'react';

/**
 * Keeps a ref pointed at the newest value of something an effect needs to
 * *read* but must not *depend on*.
 *
 * This exists for `t`. The translate function is `useCallback(…, [language])`
 * in LanguageContext, so its identity changes every time the user switches
 * Display Language. An effect that only needs `t` to word an error message
 * still has to list it under exhaustive-deps, and then the whole effect
 * re-runs on a language switch — refetching and replacing state that had
 * nothing to do with language. That misfire shipped twice on this branch:
 * meal history discarded every "Load older" page and jumped back to the top,
 * and the dashboard overwrote an in-progress card reorder with the stored
 * order. Both were found in code review, not by the type checker or the
 * lint rule, because adding `t` to the dep array is exactly what the lint
 * rule asks for — it has no way to know the effect only reads `t` on the
 * failure path.
 *
 * Reading `ref.current` inside the effect keeps the message in the language
 * that is current when the error actually happens, while leaving the effect's
 * dependencies to describe only what should genuinely re-trigger it.
 *
 * Note this is for values read inside callbacks and effect bodies, never for
 * values read during render: a ref update is invisible to rendering, so a
 * component that renders `ref.current` will not re-render when it changes.
 */
export function useLatest<T>(value: T): RefObject<T> {
  const ref = useRef(value);
  // Written in an effect rather than during render so the ref is not mutated
  // mid-render, which would be a side effect in render and can tear under
  // concurrent rendering. Effects for a given commit run before any event
  // handler or async continuation that could observe the ref, so callers
  // still see the value from the latest committed render.
  useEffect(() => {
    ref.current = value;
  }, [value]);
  return ref;
}
