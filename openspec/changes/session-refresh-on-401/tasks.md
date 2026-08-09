## 1. Implement transparent refresh-on-401

- [ ] 1.1 In `frontend/lib/api.ts`, add a module-level `refreshPromise: Promise<boolean> | null` for same-tab dedup, plus the constants `AUTH_REFRESH_LOCK = 'hcw-auth-refresh'` and `LAST_REFRESH_KEY = 'hcw:lastAuthRefreshAt'`.
- [ ] 1.2 Add `refreshAccessToken()`: POSTs `/api/auth/refresh` (plain `fetch`, `credentials: 'include'`), returns `true`/`false`, and on success writes `localStorage.setItem(LAST_REFRESH_KEY, String(Date.now()))`.
- [ ] 1.3 Add `coordinatedRefresh()`: if `refreshPromise` is set, return it (same-tab dedup). Otherwise capture `lockRequestedAt = Date.now()`, set `refreshPromise` to a function that, if `navigator.locks` is available, calls `navigator.locks.request(AUTH_REFRESH_LOCK, async () => { ... })`; inside the lock, read `localStorage.getItem(LAST_REFRESH_KEY)` — if it's `>= lockRequestedAt`, return `true` without calling the network (another tab already refreshed); otherwise call `refreshAccessToken()`. If `navigator.locks` is unavailable, fall back to calling `refreshAccessToken()` directly (same-tab-only dedup, documented residual risk on legacy browsers). Clear `refreshPromise` in a `finally` block.
- [ ] 1.4 Add `fetchWithAuthRetry(path, options)`: performs the request; on a 401 whose `path` is not `/auth/login` or `/auth/refresh`, awaits `coordinatedRefresh()`. If it resolves `true`, retry the request once. If that retry still 401s *and* the refresh was a skip (no network call made, i.e. the local `lastRefreshAt` looked fresh but wasn't), perform one real `refreshAccessToken()` call and retry once more before giving up. Otherwise return the original/retried response.
- [ ] 1.5 Rewire `apiRawFetch` to call `fetchWithAuthRetry` instead of `fetch` directly.
- [ ] 1.6 Rewire `apiFetchNoBody` to call `fetchWithAuthRetry` instead of its own duplicated `fetch` call.
- [ ] 1.7 Rewire `apiFetchForm` to call `fetchWithAuthRetry` instead of its own duplicated `fetch` call (keep the multipart/no-JSON-header behavior).
- [ ] 1.8 Verify `reanalyzeMeal` (which calls `apiRawFetch` directly and branches on 502/412 before the generic `!res.ok` check) still gets a fully-retried response, since it goes through `apiRawFetch` → `fetchWithAuthRetry`.

## 2. Tests

- [ ] 2.1 Add an E2E test in `e2e/tests/auth.spec.ts`: log in, then use `context.addCookies` to overwrite the `kin_access` cookie with an invalid value (simulating an expired/invalid access token while the real `kin_refresh` cookie from login is still valid), reload the dashboard, and assert the page stays on `/` (no redirect to `/login`) and renders authenticated content.
- [ ] 2.2 Add an E2E test: log in, then clear/corrupt both `kin_access` and `kin_refresh` cookies, reload, and assert the page redirects to `/login` (refresh-failure path still works).
- [ ] 2.3 Add a multi-tab E2E test: log in on one `page`, open a second `page` in the same `context` (Playwright contexts share cookies and `localStorage` across pages, modeling two tabs of one browser), corrupt the `kin_access` cookie so both pages' next request 401s, then trigger an authenticated request on both pages at roughly the same time. Assert: (a) both pages end up authenticated (neither redirects to `/login`), and (b) only one `POST /api/auth/refresh` network call is observed (via `page.route`/`context.route` interception counting requests to that path) — proving the losing tab did not resubmit the already-rotated refresh token.

## 3. Validate

- [ ] 3.1 Run `make lint` / `tsc --noEmit` (or project equivalent) on the frontend.
- [ ] 3.2 Deploy the feature branch to `hcw-wip` and run the full Playwright suite (`e2e/`) against it, including the new tests from section 2.
- [ ] 3.3 Manually confirm in the WIP deployment that a session survives past the 15-minute access-token TTL without a relogin prompt (either by waiting or via the cookie-corruption trick from 2.1).
- [ ] 3.4 Manually confirm in the WIP deployment, using two real browser tabs against the same login, that corrupting/expiring the access token does not force a relogin in either tab and does not revoke the other tab's session.
