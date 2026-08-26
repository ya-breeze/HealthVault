'use client';
import { useEffect, useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import { api, type Me } from '@/lib/api';
import Header from '@/components/Header';
import BottomNav from '@/components/BottomNav';
import MoreSheet from '@/components/MoreSheet';

/**
 * The chrome every authenticated page renders: header above, bottom
 * navigation below, and the clearance that keeps the page's own content out
 * from under the bar. Replaces the bare `<Header />` each page used to
 * render, so the clearance is structural rather than something each of the
 * nine pages has to remember — the same reasoning that put tap-target sizing
 * inside TapTarget.
 *
 * Not placed in `app/layout.tsx`: that also wraps `/login`, which gets no
 * chrome at all.
 *
 * This component owns the session. `Header` used to fetch it and redirect on
 * failure; MoreSheet needs the same `me` for the webhook URL it renders, and
 * two independent `api.me()` calls would double the request on every page
 * load and could render the two surfaces from two different responses. So
 * the fetch lives here and `me` goes down as a prop. No context: one prop
 * through one shell is enough, and a context would be a second way to reach
 * the same session.
 */
export default function AuthenticatedShell({
  className,
  children,
}: {
  /**
   * The page-level wrapper classes each page used to put on its own outer
   * `<div>` — its `min-h-screen` and background, and on two pages a
   * `pb-24` for a submit bar of its own. The shell takes over that element
   * rather than nesting inside it, which is why the clearance below sits on
   * an inner wrapper: a `pb-*` from a page and the shell's own would
   * otherwise be two conflicting values of one property on one element.
   */
  className?: string;
  children: React.ReactNode;
}) {
  const router = useRouter();
  const [me, setMe] = useState<Me | null>(null);
  const [moreOpen, setMoreOpen] = useState(false);
  const moreButtonRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    api.me()
      .then(setMe)
      .catch(() => router.push('/login'));
  }, [router]);

  const closeMore = () => {
    setMoreOpen(false);
    // Focus goes back to the control that opened the sheet, which is where
    // the user's attention was — the sheet is unmounted by the time this
    // runs, so without it focus would fall back to the document body.
    moreButtonRef.current?.focus();
  };

  return (
    <div className={className}>
      {/* The app is statically exported and the auth check above is
          client-side, so an unauthenticated visitor deep-linking to `/` or
          `/food/history/` is served this page's HTML and only then
          redirected. Gating the chrome on a resolved session is what stops
          the header and bar being painted for someone who turns out to have
          no session — see mobile-navigation's "Bar does not flash on an
          unauthenticated deep link". */}
      {me && <Header me={me} />}
      {/* Clearance for in-flow content. Padding on an ancestor does nothing
          for a `position: fixed` descendant, which is why the two submit
          bars and the toast stack offset themselves by the same token
          instead. `--nav-block` is `0px` at and above the breakpoint, so
          this reserves nothing on desktop. */}
      <div data-testid="shell-content" className="pb-[var(--nav-block)]">{children}</div>
      {me && (
        <BottomNav
          moreOpen={moreOpen}
          moreButtonRef={moreButtonRef}
          onMoreClick={() => setMoreOpen(v => !v)}
        />
      )}
      {me && moreOpen && <MoreSheet me={me} onClose={closeMore} />}
    </div>
  );
}
