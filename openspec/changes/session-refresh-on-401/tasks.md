## 1. Implement transparent refresh-on-401

- [ ] 1.1 In `frontend/lib/api.ts`, add a module-level `refreshPromise: Promise<boolean> | null` and a `refreshAccessToken()` helper that POSTs `/api/auth/refresh` (via plain `fetch`, `credentials: 'include'`), returns `true`/`false` for success/failure, and clears `refreshPromise` in a `finally` block.
- [ ] 1.2 Add `fetchWithAuthRetry(path, options)` that performs the request, and on a 401 whose `path` is not `/auth/login` or `/auth/refresh`, awaits (or starts) `refreshPromise`, and if it resolves `true`, retries the request exactly once with the same `options`; otherwise returns the original 401 response.
- [ ] 1.3 Rewire `apiRawFetch` to call `fetchWithAuthRetry` instead of `fetch` directly.
- [ ] 1.4 Rewire `apiFetchNoBody` to call `fetchWithAuthRetry` instead of its own duplicated `fetch` call.
- [ ] 1.5 Rewire `apiFetchForm` to call `fetchWithAuthRetry` instead of its own duplicated `fetch` call (keep the multipart/no-JSON-header behavior).
- [ ] 1.6 Verify `reanalyzeMeal` (which calls `apiRawFetch` directly and branches on 502/412 before the generic `!res.ok` check) still gets a fully-retried response, since it goes through `apiRawFetch` → `fetchWithAuthRetry`.

## 2. Tests

- [ ] 2.1 Add an E2E test in `e2e/tests/auth.spec.ts`: log in, then use `context.addCookies` to overwrite the `kin_access` cookie with an invalid value (simulating an expired/invalid access token while the real `kin_refresh` cookie from login is still valid), reload the dashboard, and assert the page stays on `/` (no redirect to `/login`) and renders authenticated content.
- [ ] 2.2 Add an E2E test: log in, then clear/corrupt both `kin_access` and `kin_refresh` cookies, reload, and assert the page redirects to `/login` (refresh-failure path still works).

## 3. Validate

- [ ] 3.1 Run `make lint` / `tsc --noEmit` (or project equivalent) on the frontend.
- [ ] 3.2 Deploy the feature branch to `hcw-wip` and run the full Playwright suite (`e2e/`) against it, including the two new tests from section 2.
- [ ] 3.3 Manually confirm in the WIP deployment that a session survives past the 15-minute access-token TTL without a relogin prompt (either by waiting or via the cookie-corruption trick from 2.1).
