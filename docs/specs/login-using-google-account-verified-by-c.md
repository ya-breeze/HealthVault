# Sign in with the Google identity Cloudflare Access already verified
Idea: ya-breeze/idea-forge#16

## Why

Every public hostname on `ikoro.in` sits behind Cloudflare Access, and the policy in front of HealthVault requires a Google login. A user who opens `https://healthvault.ikoro.in/` has therefore already proved who they are before the app's own HTML is served. The app then ignores that proof and asks for a username and password anyway: `frontend/app/login/page.tsx` renders two fields, `api.login` posts them to `/api/auth/login`, and `backend/pkg/server/auth.go` runs a bcrypt compare against the seeded password from `HCW_SEED_USERS`. That is two sign-ins for one person, and the second one adds nothing the first did not already establish.

The cost is not only the extra typing. The password is a static string set once through an environment variable and never rotated, and it is the weakest link in a chain whose strong link is Google. The lockout machinery in `backend/pkg/server/login_limiter.go` — an eviction-tiered map, a bcrypt concurrency cap, an exponential backoff schedule — exists entirely to protect that weak link. Cloudflare hands the app a signed assertion of a verified Google identity on every request, and the app throws it away.

Now is a reasonable time because the pieces are all in place. The deployment already terminates at nginx (`nginx/nginx.conf`) behind the tunnel, so the `Cf-Access-Jwt-Assertion` header reaches the backend unmodified — nginx drops headers containing underscores, and this one has none. The backend already parses JWTs with `github.com/golang-jwt/jwt/v5` and already mints its own session cookies through `generateAccessToken` and `cookies.SetAccessCookie`. The frontend already recovers from a 401 transparently in `fetchWithAuthRetry`, so there is a natural place to slot a second recovery step.

The idea asks what this does to the Android flow, and the answer is: nothing, because there is nothing to change. The Android side of HealthVault is a Health Connect exporter that lives outside this repository and posts to `POST /webhook/{username}`, handled by `backend/pkg/server/webhook.go`. That endpoint is deliberately unauthenticated and identifies the user from the path. A background exporter cannot complete an interactive Google login, so it can never present an Access Assertion, and it must keep reaching the backend the way it does today — over the LAN, or through a route that does not sit behind an Access policy. The same holds for `/mcp`, which authenticates with the static `HCW_MCP_TOKEN` bearer. This change must leave both paths exactly as they are, and that constraint is a finding worth writing down rather than a gap to close here.

## How

Add a second way to obtain a HealthVault session, and change nothing about the first one.

A new package `backend/pkg/cfaccess` verifies an Access Assertion. It fetches Cloudflare's JWKS from `https://<team domain>/cdn-cgi/access/certs`, caches the key set, and validates the token as RS256 with the expected issuer and audience. Verification is the whole security boundary here, and it has to be, because the stack is also reachable directly on the LAN without passing through Cloudflare at all. Anyone who can reach that port can set a `Cf-Access-Jwt-Assertion` header to whatever they like. A signature check against Cloudflare's published keys, pinned to the configured AUD tag, is what makes a forged header worthless. For the same reason the code never reads `Cf-Access-Authenticated-User-Email`: that header carries no signature and is trivially spoofed off-tunnel.

The JWKS is parsed by hand — base64url `n` and `e` into an `rsa.PublicKey` — rather than pulling in a JWKS library. It is about thirty lines against a stable, published format, and it keeps `backend/go.mod` unchanged.

A new endpoint `POST /api/auth/cf-access` exchanges a verified assertion for the ordinary session cookies. It mints exactly what `Login` mints, so every downstream handler, the `RequireAuth` middleware, the blacklist, and the refresh rotation all keep working untouched. This is the main structural choice, and the alternative was to teach `RequireAuth` to accept an assertion directly on every request. That was rejected: it would put an RSA signature check on the hot path of every API call, and it would give the app two kinds of authenticated request to reason about instead of one. An exchange endpoint keeps the session model single.

A verified Google identity is not by itself an authorization to use the app. The endpoint maps the assertion's `email` claim to a HealthVault username through a new `HCW_CF_ACCESS_EMAIL_MAP` setting, shaped like the existing `HCW_SEED_USERS` (`email:username`, comma-separated). An email that verifies but is absent from the map gets 403. Auto-provisioning a user from any email the Access policy happens to admit is deliberately excluded: the policy is edited in a different system by a different tool, and a widened policy should not silently create accounts. The map is also why the kin-core `User` model needs no new column — that model belongs to another repository, and a config-level map avoids a migration to a type this project does not own.

The feature is off unless configured. With the team domain, the AUD tag, or the map unset, the endpoint answers 404. That matters for more than tidiness: the WIP stack, the e2e target, and `make run-backend` all serve plain HTTP on the LAN with no Cloudflare in front, and 404 lets the frontend tell "this deployment has no Access" apart from "your sign-in failed" in one request, with no separate probe endpoint.

In the browser, `fetchWithAuthRetry` gains a second recovery step: on a 401 that a refresh could not fix, it tries the exchange once and retries. The login page tries the exchange on mount and routes to `/` when it succeeds, so a user who lands there is never asked for a password they may not have. When the exchange 404s, the page renders exactly the form it renders today. The password form stays permanently — it is the only way in over the LAN, and removing it would strand the deployment behind a dependency on Cloudflare being up.

Logout needs one deliberate piece of state. Without it, ending a session and then loading any page would silently re-authenticate through Access, and logout would look broken. So `useLogout` sets a `sessionStorage` flag that suppresses the automatic exchange, and the login page clears it only when the user clicks the Access sign-in button. The flag is per-tab and per-session on purpose: it should not outlive the browser tab that logged out.

New login-page strings stay hardcoded English, matching the page as it is. `frontend/lib/i18n/en.ts` documents its coverage list explicitly, and the login page is not on it; adding it is a separate change.

Deliberately excluded: a direct Google OAuth integration in the app (Cloudflare already does that work); any change to `/webhook/{username}` or `/mcp`; removing password login or the login limiter; auto-provisioning users; and any Android-side change, since that code is not in this repository. The Android finding gets recorded in the ADR instead.

One honest limit on validation: the e2e stack is plain HTTP on the LAN and never passes through Cloudflare, so no end-to-end test can present a real Access Assertion. The e2e suite covers the feature-off behaviour — the endpoint 404s, the password form still renders, login and logout are unchanged. Real verification coverage lives in Go unit tests that generate an RSA key, serve a JWKS from `httptest`, and sign their own tokens.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e`

## Ground rules
This spec is implemented by an automated pass running unattended. **There is no approval step and nothing is waiting for one** — do not look for a tick, a marker, or a sign-off anywhere, and do not wait for one.

Tick the boxes in this file as the work is completed; they are the record of progress, and the pipeline reads them to decide whether the change is finished.

Out of scope, deliberately: do NOT mark the pull request ready for review and do NOT merge it. Those are the pipeline's own final steps, run once the task list is complete. The operator reviews the pull request and merges it themselves; that is the only gate this work passes through, so leave it in a state worth reading.

### Task 1: Verify an Access Assertion

- [ ] Add `CFAccessTeamDomain`, `CFAccessAUD` and `CFAccessEmailMap` to `Config` in `backend/pkg/config/config.go`, read from `HCW_CF_ACCESS_TEAM_DOMAIN`, `HCW_CF_ACCESS_AUD` and `HCW_CF_ACCESS_EMAIL_MAP`, all defaulting to empty
- [ ] Pass the three new variables through `docker-compose.yml`'s `backend` service, defaulting to empty like `HCW_MCP_TOKEN` does
- [ ] Create `backend/pkg/cfaccess/verifier.go` with an `Identity` struct (`Email`, `Subject`), a `Verifier` type, `New(teamDomain, aud string) *Verifier`, and `Verify(ctx context.Context, token string) (Identity, error)`
- [ ] Fetch the key set from `https://<team domain>/cdn-cgi/access/certs` with a 5-second timeout, and decode each RSA entry's base64url `n` and `e` into an `rsa.PublicKey` using `encoding/base64` and `math/big`, adding no module dependency
- [ ] Cache the key set for 10 minutes behind a mutex, and refetch on an unknown `kid` at most once a minute so an attacker cannot drive unbounded outbound requests
- [ ] Validate with `github.com/golang-jwt/jwt/v5`: accept RS256 only, require `iss` equal to `https://<team domain>`, require `aud` to contain the configured AUD tag, and check `exp`/`nbf` with 60 seconds of leeway
- [ ] Document in the package comment why `Cf-Access-Authenticated-User-Email` is never read: the LAN port bypasses Cloudflare, so an unsigned header is spoofable and the signature is the only real check
- [ ] Add `backend/pkg/cfaccess/verifier_test.go` covering a valid token, a wrong AUD, a wrong issuer, an expired token, an `alg: none` token, an unknown `kid`, and an unreachable JWKS endpoint
- [ ] Mark completed

### Task 2: Exchange a verified identity for a session

- [ ] Add `cfVerifier *cfaccess.Verifier` and `cfEmailMap map[string]string` fields to `authHandlers` in `backend/pkg/server/auth.go`
- [ ] Parse `HCW_CF_ACCESS_EMAIL_MAP` into that map in `Run` (`backend/pkg/server/server.go`), lower-casing and trimming each email, and return a startup error on a malformed entry the way `database.SeedUsers` does
- [ ] Create `backend/pkg/server/auth_cf_access.go` with a `CFAccess` handler that reads the assertion from the `Cf-Access-Jwt-Assertion` header and falls back to the `CF_Authorization` cookie
- [ ] Respond 404 when the team domain, the AUD tag, or the map is unset, so a deployment with no Cloudflare in front reports the feature as absent rather than as a failed sign-in
- [ ] Respond 401 on a missing or unverifiable assertion, and 403 with an `unknown_identity` error body when the assertion verifies but its email is not in the map or its username has no user row
- [ ] On success mint the same session `Login` mints — `generateAccessToken` for 15 minutes, `authdb.CreateRefreshToken` for a year, both set through `cookies.SetAccessCookie` and `cookies.SetRefreshCookie` — and return `{"status":"ok"}`
- [ ] Register `POST /api/auth/cf-access` beside the other auth routes in `Run`, outside the `RequireAuth` subrouter, and leave `/api/auth/login`, `/webhook/{username}` and `/mcp` untouched
- [ ] Comment on why this handler skips the login limiter: there is no guessable secret to spread over attempts, and a signature check is not a bcrypt compare
- [ ] Add `backend/pkg/server/auth_cf_access_test.go` covering disabled config to 404, a spoofed unsigned header to 401, a verified but unmapped email to 403, and a mapped email to 200 with both cookies present
- [ ] Mark completed

### Task 3: Use the exchange from the browser

- [ ] Add `api.cfAccessLogin()` to `frontend/lib/api.ts`, posting to `/auth/cf-access`, and add that path to `isAuthExemptPath` so it cannot recurse into its own retry
- [ ] Extend `fetchWithAuthRetry` so a 401 that `coordinatedRefresh` could not fix tries the exchange once before giving up, and retries the original request when it succeeds
- [ ] Add `accessSignInSuppressed()` and `suppressAccessSignIn()` helpers backed by `sessionStorage` in `frontend/lib/session.ts`, guarded against a `sessionStorage` that throws, matching how `lib/api.ts` guards `localStorage`
- [ ] Call `suppressAccessSignIn()` from `frontend/components/useLogout.ts` alongside `clearSession()`, so logging out does not immediately re-authenticate through Access
- [ ] In `frontend/app/login/page.tsx`, attempt the exchange on mount unless suppressed: route to `/` on success, hide the Access control on 404, and on 401 or 403 show the password form plus a short message and an explicit sign-in button that clears the suppression flag and retries
- [ ] Keep the username and password form and its 429 lockout handling exactly as they are, and keep the new strings hardcoded English to match the rest of the page
- [ ] Add cases to `frontend/lib/api.test.ts` for the new 401 recovery: refresh fails then exchange succeeds and the request is retried, and both fail so the original 401 is returned
- [ ] Mark completed

### Task 4: Validate against a stack with the feature off

- [ ] Add `e2e/tests/cf-access.spec.ts` with a header comment stating that the e2e target is plain HTTP on the LAN and never passes through Cloudflare, so no test here can present a real Access Assertion, and that the verification coverage lives in `backend/pkg/cfaccess`
- [ ] Assert `POST /api/auth/cf-access` answers 404 on the deployed stack and sets no cookies
- [ ] Assert the login page still renders the username and password fields and does not hang on the mount-time exchange attempt
- [ ] Assert password login still reaches the dashboard, and that logging out lands on `/login` and stays there rather than bouncing back to `/`
- [ ] Run `make lint`, `make test` and `make test-e2e` against the deployed stack and fix what they report
- [ ] Mark completed

### Task 5: Record the decision

- [ ] Add `docs/adr/ADR-012-cloudflare-verified-identity-as-a-second-sign-in.md` with `Status: Proposed`, covering the exchange-endpoint choice over middleware, the signature-only trust rule, the explicit email map over auto-provisioning, and why password login stays
- [ ] Record the Android finding in that ADR: the Health Connect exporter posts to the unauthenticated `POST /webhook/{username}` and cannot complete an interactive Google login, so it can never carry an Access Assertion and must keep its current route; exposing that path publicly would need an Access service token or a bypass policy, which this change does not do
- [ ] Add glossary entries for **Access Assertion** and **Access Identity Map** to `CONTEXT.md`, each with its `_Avoid_` line, following the existing entries' shape
- [ ] Document the three new environment variables and the feature-off default in `frontend/README.md`'s configuration notes or the equivalent backend notes, whichever already lists `HCW_MCP_TOKEN`
- [ ] Flip ADR-012 from `Proposed` to `Accepted` as the last commit on the branch
- [ ] Mark completed
