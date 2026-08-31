'use client';
import { createContext, useCallback, useContext, useEffect, useId, useRef, useState } from 'react';

interface BottomActionBarContextValue {
  register: (key: string, height: number) => void;
  unregister: (key: string) => void;
}

const BottomActionBarContext = createContext<BottomActionBarContextValue | null>(null);
const HeightContext = createContext<number | null>(null);

// Registry of every currently-mounted BottomActionBar, keyed by useId so two
// bars mounted at once (a transition between pages, or a future second bar)
// don't leave the toast reading a stale single value — it always reads the
// max of whatever is actually on screen.
export function BottomActionBarProvider({ children }: { children: React.ReactNode }) {
  const [heights, setHeights] = useState<Record<string, number>>({});

  const register = useCallback((key: string, height: number) => {
    setHeights(prev => (prev[key] === height ? prev : { ...prev, [key]: height }));
  }, []);

  const unregister = useCallback((key: string) => {
    setHeights(prev => {
      if (!(key in prev)) return prev;
      const next = { ...prev };
      delete next[key];
      return next;
    });
  }, []);

  const maxHeight = Object.values(heights).reduce((max, h) => Math.max(max, h), 0);

  return (
    <BottomActionBarContext.Provider value={{ register, unregister }}>
      <HeightContext.Provider value={maxHeight}>{children}</HeightContext.Provider>
    </BottomActionBarContext.Provider>
  );
}

// What the toast stack reads: the tallest currently-registered bar's height,
// or 0 when none is mounted. Throws outside the provider, matching useToast
// in Toast.tsx, for the same reason — a silent fallback would hide a missing
// provider instead of failing where it's introduced.
export function useBottomActionBarHeight(): number {
  const ctx = useContext(HeightContext);
  if (ctx === null) {
    throw new Error('useBottomActionBarHeight must be used within a BottomActionBarProvider');
  }
  return ctx;
}

// Shared fixed submit/confirm bar for `/food/manual/` and `/food/review/`.
// Anchored above the bottom navigation bar, not at the viewport edge: both
// pages are among the bar's own destinations, so without the offset the
// navigation bar would land on the button. A padding on the shell cannot do
// this — a fixed element is out of flow relative to the viewport, not to its
// ancestor. Its own bottom padding keeps the safe-area inset for the desktop
// case, where no navigation bar is beneath it to absorb it; below the
// breakpoint `--edge-inset-b` is `0px` because `--nav-block` already carries
// it (see ADR-008).
//
// Measures its own border-box height with a ResizeObserver and registers it
// while mounted, so the toast stack (Toast.tsx) can offset above it instead
// of guessing a literal height — see ADR-011. Unmounting (e.g.
// `showConfirmBar` going false) unregisters it and drops the toast back to
// its default position.
export default function BottomActionBar({ children }: { children: React.ReactNode }) {
  const ctx = useContext(BottomActionBarContext);
  if (!ctx) {
    throw new Error('BottomActionBar must be used within a BottomActionBarProvider');
  }
  const { register, unregister } = ctx;
  const key = useId();
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const observer = new ResizeObserver(entries => {
      const entry = entries[0];
      // borderBoxSize is what's actually wanted — content-box height would
      // exclude the bar's own padding and border, understating by far more
      // than a rounding error. getBoundingClientRect is also border-box, for
      // browsers that don't report borderBoxSize.
      const height = entry.borderBoxSize?.[0]?.blockSize ?? el.getBoundingClientRect().height;
      register(key, height);
    });
    observer.observe(el);
    return () => {
      observer.disconnect();
      unregister(key);
    };
  }, [key, register, unregister]);

  return (
    <div
      ref={ref}
      data-testid="bottom-action-bar"
      className="fixed bottom-[var(--nav-block)] left-0 right-0 z-30 bg-gray-50/95 dark:bg-gray-900/95 backdrop-blur border-t border-gray-200 dark:border-gray-700 px-6 py-3 pb-[calc(0.75rem+var(--edge-inset-b))]"
    >
      <div className="max-w-md mx-auto">{children}</div>
    </div>
  );
}
