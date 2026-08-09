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

## Goals / Non-Goals

**Goals:**
- A 401 from any authenticated endpoint transparently attempts one `POST /api/auth/refresh`, and on success
  retries the original request once, before the caller ever sees a failure.
- Multiple requests that 401 around the same time (e.g. `page.tsx` firing several `api.data()` calls right
  after `api.me()`) trigger at most one `/api/auth/refresh` call, not one per request.
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

**Module-level in-flight refresh promise, not a mutex or queue library.**
A single `let refreshPromise: Promise<boolean> | null = null` at module scope. The first 401 to arrive starts
the refresh call and stores its promise; concurrent 401s reuse the same promise instead of issuing their own
`/api/auth/refresh` calls. Once it settles, the variable is cleared so the next expiry starts a fresh refresh.
This is sufficient for a single-tab browser client — no need for cross-tab coordination (each tab has its own
JS heap; a cross-tab race just means at most one extra refresh call, which is harmless since refresh rotation
is designed to be safe to call whenever the cookie is valid).

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

- **[Risk]** If the refresh call itself hangs or is slow, all queued requests wait on it → user sees a longer
  pause instead of an immediate redirect. **Mitigation**: acceptable trade-off — a brief pause is strictly
  better than an unnecessary relogin, and the refresh endpoint is a single fast DB round-trip.
- **[Risk]** A stale in-flight `refreshPromise` could theoretically survive across unrelated 401s if not
  cleared correctly. **Mitigation**: clear it in a `finally` block immediately after the refresh call settles,
  before any retries run, so a bug here fails toward "too many refresh calls" rather than "stuck forever."
- **[Trade-off]** This only fixes the reactive path (401 already happened). A user's very first request after
  15+ minutes idle now costs two round-trips (refresh + retry) instead of one. Acceptable — that's still much
  better than a full relogin, and matches the Non-Goal of not adding proactive/background refresh complexity.

## Migration Plan

Purely additive frontend change, no data migration. Deploy like any other frontend change: land on
`feature/session-refresh-on-401`, validate against `hcw-wip` (this project has no dogfood tier for the
frontend), then ship to `main` — `hcw-prod` follows `main` automatically. Rollback is a plain revert; no
backend or schema changes are involved.
