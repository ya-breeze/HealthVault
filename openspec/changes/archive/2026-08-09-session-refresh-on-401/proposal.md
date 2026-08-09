## Why

Users on the production dashboard are forced to re-login after any ~15 minutes of inactivity, even though a valid 365-day refresh token cookie exists. Root cause: the backend already issues a short-lived (15-minute) access token plus a working `POST /api/auth/refresh` endpoint that rotates the refresh token and mints a new access token, but the frontend never calls it. `frontend/lib/api.ts` treats any 401 the same as "not authenticated" and every caller (`Header.tsx`, `app/page.tsx`) immediately redirects to `/login`, discarding the still-valid refresh token instead of silently recovering the session.

## What Changes

- Centralize 401 handling in `frontend/lib/api.ts`: when any API call gets a 401, transparently call `POST /api/auth/refresh` once, and if it succeeds, retry the original request exactly once.
- Only surface a 401 to the caller (which today triggers a redirect to `/login`) if the refresh call itself fails (refresh token missing, expired, already rotated, or blacklisted).
- No changes to page-level code (`Header.tsx`, `app/page.tsx`) — they keep their existing `.catch(() => router.push('/login'))` pattern, which now only fires on a genuinely dead session.
- No backend changes — `POST /api/auth/refresh` already implements the needed rotation semantics per the existing `authentication` spec.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `authentication`: adds a requirement that the frontend API layer SHALL transparently refresh an expired access token on 401 and retry once before treating the session as invalid.

## Impact

- `frontend/lib/api.ts` — the shared fetch helpers (`apiRawFetch`, `apiFetch`, `apiFetchNoBody`, `apiFetchForm`) gain a 401-retry-after-refresh path.
- No API contract changes, no new endpoints, no backend code changes.
- Affects every authenticated page indirectly (they all go through `frontend/lib/api.ts`), reducing spurious relogin prompts.
