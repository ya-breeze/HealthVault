# ADR-012: Cloudflare-Verified Identity as a Second, Optional Sign-In

## Status
Accepted

## Context and Problem Statement

Every public hostname on `ikoro.in` sits behind Cloudflare Access, and the policy in front of
HealthVault requires a Google login. A user who reaches `https://healthvault.ikoro.in/` has
therefore already proved who they are before the app's own login page renders — and the app then
asks for a HealthVault username and password anyway, checked with a bcrypt compare against a
password seeded once through `HCW_SEED_USERS` and never rotated. `backend/pkg/server/login_limiter.go`
(ADR-009) exists entirely to protect that weak, static secret; Cloudflare hands the app a signed
assertion of a verified Google identity on every request, and the app was throwing it away. How
should that assertion reach a HealthVault session, without weakening the one thing that makes the
LAN port (which bypasses Cloudflare entirely) safe to leave open — an attacker on the LAN can set
`Cf-Access-Jwt-Assertion` to anything they like?

## Decision Drivers

- The backend is reachable two ways: through the Cloudflare tunnel (where the header is genuine)
  and directly on the LAN (where it is attacker-controlled). Any design has to treat the header as
  untrusted input and re-derive trust from something an attacker cannot forge.
- `RequireAuth` (`backend/pkg/server/middleware.go`) already gates every `/api` route with one
  check: a `kin_access` JWT signed with `HCW_JWT_SECRET`. Every downstream handler, the blacklist,
  and refresh rotation are built against that one mechanism.
- A verified Google identity is proof of *who*, not proof of *authorized to use this app*. The
  Access policy is edited in Cloudflare, a different system from HealthVault's user table.
- The password form is the only way in when Cloudflare is not reachable (LAN access, an outage,
  local dev) — this project's WIP stack, e2e target, and `make run-backend` all serve plain HTTP
  with no tunnel in front.
- The Android side of HealthVault (a Health Connect exporter living outside this repository) posts
  to `POST /webhook/{username}`, unauthenticated, and cannot complete an interactive Google login.

## Considered Options

- **Teach `RequireAuth` to accept an Access Assertion directly, on every request** — no new
  endpoint, but it puts an RSA signature check and a JWKS lookup on the hot path of every API call,
  and gives the app two kinds of authenticated request to reason about (JWT-cookie and
  Access-Assertion) everywhere `RequireAuth` is trusted, instead of one.
- **An exchange endpoint, `POST /api/auth/cf-access`, that mints the same session `Login` does** —
  verification happens once, at sign-in, and the result is the same `kin_access`/`kin_refresh`
  cookie pair every other path already produces. `RequireAuth` and everything behind it stay
  untouched.
- **Trust `Cf-Access-Authenticated-User-Email` directly** — Cloudflare does set this header, and
  reading it would avoid writing a JWKS verifier at all. Rejected outright: the header carries no
  signature. On the LAN port, where the same request path is reachable with no tunnel in front, it
  is exactly as forgeable as any other client-supplied header — trusting it would mean the LAN port
  authenticates as whoever an attacker types into a header, defeating the reason Access exists at
  all.
- **Auto-provision a HealthVault user from any verified email the Access policy admits** — no config
  to maintain, but it means widening the Access policy (an edit in a different system, by a
  different tool) silently creates HealthVault accounts. Rejected.
- **An explicit `HCW_CF_ACCESS_EMAIL_MAP` (`email:username`, comma-separated, shaped like the
  existing `HCW_SEED_USERS`)** — a verified email that is not in the map gets 403, so authorization
  stays a HealthVault-side decision distinct from Cloudflare's identity check.
- **Remove the password form once Access sign-in works** — would strand every LAN/no-tunnel
  deployment path listed above with no way to sign in. Rejected.

## Decision Outcome

Chosen: **an exchange endpoint, backed by a real signature check and an explicit email map, that
mints the same session type the password path does, with the password form kept permanently.**

`backend/pkg/cfaccess.Verifier` fetches Cloudflare's JWKS from
`https://<team domain>/cdn-cgi/access/certs`, caches it for 10 minutes, and validates a
`Cf-Access-Jwt-Assertion` as RS256 with the issuer and audience pinned to configured values and
60 seconds of `exp`/`nbf` leeway. This — not the presence of any header — is the entire security
boundary: a signature check against Cloudflare's own published keys is the one thing a forged
header on the LAN port cannot pass. `Cf-Access-Authenticated-User-Email` is never read anywhere in
this code, on purpose, for the reason above.

`POST /api/auth/cf-access` (`backend/pkg/server/auth_cf_access.go`) is the only place that calls
the verifier. On success it maps the assertion's `email` claim through `HCW_CF_ACCESS_EMAIL_MAP` to
a HealthVault username, loads that user, and mints exactly what `Login` mints —
`generateAccessToken` for 15 minutes and `authdb.CreateRefreshToken` for a year, set through the
same `cookies.SetAccessCookie`/`SetRefreshCookie` calls. `RequireAuth`, the blacklist, and refresh
rotation needed no changes at all. It skips the login limiter deliberately: that machinery exists
to slow down guessing against a static secret, and there is no secret here to guess — the gate is
a signature check, not a bcrypt compare.

The endpoint answers 404, not 401, while any of the team domain, the AUD tag, or the email map is
unset — this is how the frontend tells "this deployment has no Access in front" apart from "your
sign-in failed" without a separate probe endpoint, and it is what keeps the WIP stack, the e2e
target, and local dev working with no configuration at all.

The password form (`frontend/app/login/page.tsx`) stays exactly as it was, permanently. The
frontend attempts the exchange on mount and routes home on success, but the form is never removed
or conditionally hidden behind Access being configured — it is the only way in over the LAN, and on
an outage.

### Android finding

The Android side of HealthVault is a Health Connect exporter that lives outside this repository
and posts to `POST /webhook/{username}` (`backend/pkg/server/webhook.go`), deliberately
unauthenticated and identified by path. A background exporter cannot complete an interactive
Google login, so it can never present an Access Assertion, and it keeps reaching the backend the
way it does today — over the LAN, or through a route that does not sit behind an Access policy.
`/mcp` is the same case: it authenticates with the static `HCW_MCP_TOKEN` bearer, and this change
leaves it untouched. Exposing either path publicly would need a Cloudflare Access **service token**
(a machine-to-machine credential distinct from the Google-login policy this ADR builds on) or a
bypass policy scoped to that route — this change does neither; it is a finding recorded for a
future decision, not a gap closed here.

### Consequences

- A verified Google identity absent from `HCW_CF_ACCESS_EMAIL_MAP` is a 403, not an
  auto-provisioned account — widening the Cloudflare Access policy alone never creates a
  HealthVault user.
- The `kin-core` `User` model needed no new column: the email-to-username mapping lives entirely in
  HealthVault's own config, not in a model owned by another repository.
- Every deployment without `HCW_CF_ACCESS_TEAM_DOMAIN`/`HCW_CF_ACCESS_AUD`/`HCW_CF_ACCESS_EMAIL_MAP`
  set behaves exactly as before this change — the endpoint 404s, the frontend never shows the Access
  control as available, and the password form is unaffected.
- The password form and its login-limiter protection (ADR-009) are not superseded by this ADR — the
  two sign-in paths are independent, and removing either is a separate decision this ADR does not
  make.
- Logging out needed one piece of new client state (a `sessionStorage` suppression flag,
  `frontend/lib/session.ts`) so that ending a session does not silently re-authenticate through
  Access on the very next page load; it is per-tab and per-session by design.
