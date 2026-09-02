import type { Me } from '@/lib/api';

/**
 * The resolved session, cached for the lifetime of the loaded page.
 *
 * `AuthenticatedShell` is rendered inside each page component, not in a
 * layout, so a client-side navigation unmounts and remounts it. Without this
 * cache its `me` would start as `null` on every navigation and the chrome it
 * gates — the header and the bottom bar — would be absent until `/users/me`
 * answered: the bar disappearing out from under the finger that just tapped
 * it, and the page body jumping up by the header's height and back. Seeding
 * the shell's initial state from here makes a remount paint the chrome on its
 * first frame instead. The fetch still runs on every mount, so a session that
 * has expired server-side still redirects to /login exactly as before.
 *
 * Module state rather than a context: a provider would have to live in the
 * root layout, which also wraps `/login` — putting a `/users/me` request, and
 * the 401 refresh retry behind it, on the one page that has no session yet.
 *
 * A full page load starts a fresh module, so nothing here survives into
 * another account's session; `clearSession` covers logging out within one.
 */
let cached: Me | null = null;

export function cachedSession(): Me | null {
  return cached;
}

export function rememberSession(me: Me): void {
  cached = me;
}

export function clearSession(): void {
  cached = null;
}

// Suppresses the login page's automatic Cf-Access exchange attempt — see
// useLogout, which sets this right after ending a session, and the login
// page, which reads it on mount and clears it only when the user clicks the
// explicit Access sign-in button.
//
// Without this, loading any page after logging out would silently
// re-authenticate through Access on the very next mount-time attempt, and
// logout would look broken: the user would never actually land on /login.
//
// sessionStorage, not localStorage: the suppression is per-tab and
// per-session on purpose, so it should not outlive the browser tab that
// logged out, and should not follow the user into a different tab that is
// still signed in.
const ACCESS_SIGN_IN_SUPPRESSED_KEY = 'hcw:accessSignInSuppressed';

export function accessSignInSuppressed(): boolean {
  try {
    return sessionStorage.getItem(ACCESS_SIGN_IN_SUPPRESSED_KEY) === '1';
  } catch {
    // sessionStorage may be unavailable (e.g. private browsing) — matches
    // lib/api.ts's guard around localStorage. Unsuppressed is the safe
    // default: the exchange simply gets attempted again, same as a first visit.
    return false;
  }
}

export function suppressAccessSignIn(): void {
  try {
    sessionStorage.setItem(ACCESS_SIGN_IN_SUPPRESSED_KEY, '1');
  } catch {
    // Same guard as above; the suppression just doesn't persist.
  }
}

export function clearAccessSignInSuppression(): void {
  try {
    sessionStorage.removeItem(ACCESS_SIGN_IN_SUPPRESSED_KEY);
  } catch {
    // Same guard as above.
  }
}
