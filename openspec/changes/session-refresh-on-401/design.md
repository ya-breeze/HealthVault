## Context

`frontend/lib/api.ts` has three independent low-level request helpers that each call `fetch` directly:
`apiRawFetch` (JSON, used by `apiFetch` and `reanalyzeMeal`), `apiFetchNoBody` (JSON, no response body expected),
and `apiFetchForm` (multipart `FormData` uploads). None of them react to a 401 beyond letting it surface as an
`ApiError`/`Error` to the caller, which every page currently treats as "not logged in."

The backend already implements everything needed to recover a session without a full relogin: `POST
/api/auth/refresh` reads the `kin_refresh` cookie (scoped to that path, so it isn't sent on other requests),
rotates it, and issues a fresh 15-minute access token as a `kin_access` cookie. Both cookies are `HttpOnly`, so
the frontend cannot read or reason about their expiry directly — it can only observe a 401 and choose to call
`/api/auth/refresh`.

The frontend is a static export (`output: 'export'`), so this must be plain client-side `fetch` logic — no
Next.js middleware or server-side session handling is available.

The user accesses this app from both mobile and desktop, and realistically may have it open in more than one
tab of the same browser at once (a desktop browser with two tabs, or a phone browser that keeps a background
tab alive). Tabs of the same browser share the `kin_refresh` cookie but not JS memory, so any design that only
dedupes refresh calls within a single tab is unsafe: see the cross-tab rotation race under Decisions.

## Goals / Non-Goals

**Goals:**
- A 401 from any authenticated endpoint transparently attempts one `POST /api/auth/refresh`, and on success
  retries the original request once, before the caller ever sees a failure.
- Multiple requests that 401 around the same time — whether in the same tab (e.g. `page.tsx` firing several
  `api.data()` calls right after `api.me()`) or across different tabs of the same browser — trigger at most
  one `/api/auth/refresh` call, not one per request or per tab.
- Callers (`Header.tsx`, `app/page.tsx`, etc.) need no changes — they keep reacting to a final 401 by
  redirecting to `/login`, which now only fires when the refresh token itself is dead.

**Non-Goals:**
- No change to token TTLs, cookie flags, or the backend `/api/auth/refresh` semantics — they already match the
  `authentication` spec.
- No proactive/background refresh (e.g. a timer that refreshes before the 15 minutes elapse). Reactive
  refresh-on-401 is sufficient and simpler; a background timer would run even on idle tabs for no benefit.
- No retry beyond one attempt — if the retried request also 401s, that is treated as a real auth failure, not
  looped on.

## Decisions

**Single shared low-level fetch function, wrapped with refresh-retry, used by all three helpers.**
Introduce `fetchWithAuthRetry(path, options)` as the one place that calls `fetch` and inspects the response
status. `apiRawFetch`, `apiFetchNoBody`, and `apiFetchForm` keep their existing signatures (different headers,
form vs JSON) but delegate the actual network call + retry decision to this shared function, so the retry logic
is written once. Alternative considered: duplicate the retry check in each of the three helpers — rejected,
that's exactly the kind of divergence that let the original bug (`apiFetchNoBody`/`apiFetchForm` already
duplicate `apiRawFetch`'s fetch call today) go unnoticed.

**Two-layer dedup: in-tab promise, plus cross-tab coordination via the Web Locks API.**

Same-tab dedup is a `let refreshPromise: Promise<boolean> | null = null` at module scope, as originally
designed: the first 401 in a tab starts the refresh call and stores its promise; concurrent 401s in that same
tab reuse it. This alone is *not* safe across tabs — and the risk isn't hypothetical:

`kin-core`'s `RotateRefreshToken` (`authdb/refresh_token.go:71-77`) treats reuse of an already-rotated refresh
token as **theft detection**: it revokes every active refresh token for that user, i.e. it force-logs-out all
of that user's other tabs and devices. Two tabs of the same browser share the `kin_refresh` cookie but not JS
memory. If both 401 around the same time, both can read the same (still-valid, not-yet-rotated) refresh token
and both call `/api/auth/refresh` concurrently. Whichever request the backend processes first rotates the
token and succeeds; the second request now presents an already-revoked token, hits `ErrTokenCompromised`, and
takes down every session for that user — the opposite of this change's goal.

Fixing this by loosening `RotateRefreshToken`'s reuse detection (e.g. a grace period tolerating reuse within a
few seconds) was considered and rejected for this change: `kin-core` is a shared module also used by KinCart,
GeekBudgetBE, Diary, and GeekBudgetBE-external-importer, several of which run against real production data.
Changing security-sensitive token-rotation semantics there has a blast radius far beyond this fix and would
need its own review and version bump across every consumer — out of scope here.

Instead, coordinate refreshes across tabs entirely on the frontend. The critical detail (see the timing bug
below) is *when* the coordination marker is captured — it must be the moment the original request was sent,
not the moment its 401 came back:

1. `fetchWithAuthRetry` captures `dispatchedAt = Date.now()` immediately before firing the request's *first*
   `fetch` call — before it knows whether that request will 401. This timestamp answers "could this request
   have gone out carrying a stale cookie?", which only the send time can answer; the receive time cannot, since
   a slow request can be dispatched before a refresh and still not 401 until after it.
2. On a 401 (past the login/refresh exclusions below), acquire an exclusive lock via
   `navigator.locks.request('hcw-auth-refresh', ...)`, carrying `dispatchedAt` into the callback. Web Locks are
   scoped per browser profile/origin and shared across tabs, so only one tab's callback runs at a time; others
   queue automatically and the lock is released even if a tab crashes or is closed.
3. Inside the lock callback, read `localStorage.getItem('hcw:lastAuthRefreshAt')`. If that timestamp is
   `>= dispatchedAt`, some other tab's refresh completed at or after this request was sent — the current
   `kin_access` cookie is guaranteed fresh, so skip calling `/api/auth/refresh` (it would reuse a token that tab
   already rotated) and go straight to retrying the original request.
4. Otherwise, call `/api/auth/refresh`. On success, write `localStorage.setItem('hcw:lastAuthRefreshAt', String(Date.now()))`
   before releasing the lock.
5. Retry the original request exactly once, whether the refresh was skipped (step 3) or actually performed
   (step 4). If that retry still fails, surface the failure to the caller — no further fallback. Because the
   skip decision in step 3 is provably correct (see below), there is no failure mode left for a second, lock-
   bypassing refresh attempt to paper over; adding one would only reintroduce the exact race this design
   prevents, so it is deliberately not there.
6. **Feature detection fallback**: `navigator.locks` is unsupported only on very old browsers (pre–Safari
   15.4/2022, pre–Firefox 96/2022; all real 2026 mobile/desktop browsers support it). If absent, fall back to
   the same-tab-only `refreshPromise` dedup — accepting the cross-tab race as a residual risk on that small,
   legacy set of browsers rather than blocking the fix on it.

**Why `dispatchedAt` (send time) rather than 401-receive time makes the skip decision provably correct:** if a
refresh completes at or after the moment a request was sent, that refresh's `Set-Cookie` response is guaranteed
to have already landed in the browser's cookie store by the time this code retries (retries only happen after
the original response — and thus after the refresh — resolves). So `lastAuthRefreshAt >= dispatchedAt` is a
sound proof that the retry will carry a fresh cookie, with no gap for a stale-timestamp read to slip through.
Capturing the marker at 401-receive time instead (the original version of this design) does not have this
property: a request dispatched before a refresh can still receive its 401 *after* that refresh completes,
making its receive-time marker look newer than the refresh and wrongly concluding no refresh happened.

Alternative considered: a hand-rolled `localStorage`-based mutex (write a lock-holder id + timestamp, poll for
release, expire stale locks). Rejected in favor of Web Locks — it already handles queuing, release-on-crash,
and cross-tab ordering correctly, where a hand-rolled version would need to reinvent all of that with more
room for its own races.

**Skip the retry path for `/auth/login` and `/auth/refresh` themselves.**
A 401 from `/auth/login` is "wrong credentials," not "expired session" — attempting a refresh there would be
nonsensical (no access token was ever issued) and could mask the real error. A 401 from `/auth/refresh` means
the refresh token itself is dead — retrying differently doesn't help; it should just fail through so the caller
redirects to `/login`. These two paths are recognized by suffix match on the request path and never enter the
retry branch.

**Retry the original request exactly once, using the same `options` object.**
`fetch`'s `RequestInit.body` for JSON calls is already a fully-serialized string, and for `apiFetchForm` a
`FormData` object — neither is a stream that gets consumed by a failed request, so passing the same `options`
to `fetch` a second time is safe and re-sends the same payload.

## Risks / Trade-offs

- **[Risk]** If the refresh call itself hangs or is slow, all queued requests (in this tab, and other tabs
  waiting on the Web Lock) wait on it → user sees a longer pause instead of an immediate redirect.
  **Mitigation**: acceptable trade-off — a brief pause is strictly better than an unnecessary relogin, and the
  refresh endpoint is a single fast DB round-trip.
- **[Risk]** A stale in-flight `refreshPromise` could theoretically survive across unrelated 401s if not
  cleared correctly. **Mitigation**: clear it in a `finally` block immediately after the refresh call settles,
  before any retries run, so a bug here fails toward "too many refresh calls" rather than "stuck forever."
- **[Risk]** Cross-tab dedup depends on `navigator.locks` availability. On the small set of legacy browsers
  without it, the original cross-tab rotation race (a losing tab's request hits `ErrTokenCompromised` and logs
  out all of that user's sessions) is still possible. **Mitigation**: explicitly accepted as a residual risk
  for that legacy fallback, not silently ignored — see Decisions. All browsers this user is expected to use
  (recent mobile and desktop) support Web Locks.
- **[Risk]** The `localStorage.getItem('hcw:lastAuthRefreshAt')` timestamp check could theoretically read a
  stale value (e.g. a bug in the write-then-release ordering leaving a losing tab reading before the winner
  writes). **Mitigation**: the write happens inside the same Web Lock callback, before release, so ordering is
  guaranteed — combined with using send-time (`dispatchedAt`) rather than receive-time as the comparison marker
  (see Decisions), the skip decision has no known gap left. There is deliberately no bypass-the-lock fallback
  for "the skip guessed wrong" (an earlier draft of this design had one, and a reviewer correctly pointed out
  it could itself cause the same rotation race by calling `/api/auth/refresh` outside both dedup layers) — if
  the skip decision is ever wrong, the retry fails and the caller redirects to `/login`, which is a correct,
  safe (if suboptimal) outcome, not a logic error to route around with a second unguarded refresh.
- **[Trade-off]** This only fixes the reactive path (401 already happened). A user's very first request after
  15+ minutes idle now costs two round-trips (refresh + retry) instead of one. Acceptable — that's still much
  better than a full relogin, and matches the Non-Goal of not adding proactive/background refresh complexity.

## Migration Plan

Purely additive frontend change, no data migration. Deploy like any other frontend change: land on
`feature/session-refresh-on-401`, validate against `hcw-wip` (this project has no dogfood tier for the
frontend), then ship to `main` — `hcw-prod` follows `main` automatically. Rollback is a plain revert; no
backend or schema changes are involved.
