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
