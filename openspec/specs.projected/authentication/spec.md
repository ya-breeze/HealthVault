<!-- GENERATED FILE — DO NOT EDIT.
     Regenerate with: make projected-specs
     See openspec/specs.projected.README.md for details. -->

## Purpose

Authenticates users via short-lived access tokens plus long-lived rotating refresh tokens, delivered as
HttpOnly cookies, so API requests can be verified per-request while sessions survive across browser restarts
without keeping a password on the client.
## Requirements
### Requirement: Login
The system SHALL accept `POST /api/auth/login` with a JSON body containing `username` and `password`. On success it SHALL issue an access token (15-minute TTL) and a refresh token (365-day TTL), deliver both as HttpOnly cookies, and return HTTP 200 with `{"status": "ok"}`. The endpoint SHALL be unauthenticated (no JWT required).

#### Scenario: Successful login
- **WHEN** a user POSTs valid credentials
- **THEN** the system SHALL set `access_token` and `refresh_token` cookies and return HTTP 200

#### Scenario: Unknown username
- **WHEN** a user POSTs a username that does not exist
- **THEN** the system SHALL return HTTP 401 with no cookies set

#### Scenario: Wrong password
- **WHEN** a user POSTs the correct username but an incorrect password
- **THEN** the system SHALL return HTTP 401 with no cookies set

---

### Requirement: Logout
The system SHALL accept `POST /api/auth/logout`. If an access token cookie is present it SHALL be added to a DB-backed blacklist keyed by token string and expiry time. Both auth cookies SHALL be cleared. The endpoint SHALL return HTTP 204.

#### Scenario: Successful logout
- **WHEN** an authenticated user POSTs to `/api/auth/logout`
- **THEN** the access token SHALL be blacklisted and both cookies SHALL be cleared with HTTP 204

#### Scenario: Logout without token
- **WHEN** the request carries no access token cookie
- **THEN** the system SHALL still return HTTP 204 (no-op, best-effort)

---

### Requirement: Refresh token rotation
The system SHALL accept `POST /api/auth/refresh`. It SHALL validate the refresh token from the cookie, rotate it (invalidate the old one, issue a new one with a fresh 365-day TTL), and issue a new access token. Both new tokens SHALL be delivered as cookies. The endpoint SHALL return HTTP 204.

#### Scenario: Valid refresh token
- **WHEN** a user POSTs with a valid refresh token cookie
- **THEN** the system SHALL rotate the refresh token and issue a new access token, returning HTTP 204

#### Scenario: Invalid or expired refresh token
- **WHEN** the refresh token is absent, expired, or already rotated
- **THEN** the system SHALL return HTTP 401 with no new cookies set

---

### Requirement: RequireAuth middleware
All routes under `/api` (except `/api/auth/*`) SHALL be protected by middleware that validates the access token via the `access_token` cookie. Blacklisted tokens SHALL be rejected. The middleware SHALL inject the parsed claims (user_id, family_id) into the request context for downstream handlers.

#### Scenario: Valid token via cookie
- **WHEN** a request to a protected route carries a valid access token cookie
- **THEN** the request SHALL proceed and claims SHALL be available in context

#### Scenario: Valid token via Authorization header
- **WHEN** a request to a protected route carries `Authorization: Bearer <valid-token>` with no
  `access_token` cookie
- **THEN** the system SHALL return HTTP 401 — **correction**: this codebase's pinned `kin-core`
  v0.1.0 middleware validates the `access_token` cookie only and has no `Authorization: Bearer`
  header path; a prior version of this requirement incorrectly claimed header support existed

#### Scenario: Missing token
- **WHEN** a request to a protected route carries no token
- **THEN** the system SHALL return HTTP 401

#### Scenario: Blacklisted token
- **WHEN** a request carries a token that has been blacklisted (e.g. after logout)
- **THEN** the system SHALL return HTTP 401

#### Scenario: Expired token
- **WHEN** a request carries a token whose TTL has elapsed
- **THEN** the system SHALL return HTTP 401

### Requirement: JWT claims
Access tokens SHALL carry `user_id` (UUID) and `family_id` (UUID) as custom claims alongside standard JWT registered claims (`sub`, `exp`). The `family_id` claim enables family-scoped data access checks without additional DB lookups in the middleware hot path.

#### Scenario: Claims available after auth
- **WHEN** a protected handler reads claims from context
- **THEN** it SHALL find the correct `user_id` and `family_id` matching the authenticated user

### Requirement: Frontend transparent refresh on 401
The frontend API layer (`frontend/lib/api.ts`) SHALL treat a 401 response from any authenticated endpoint other than `/api/auth/login` and `/api/auth/refresh` as a potentially-expired access token, not an immediate session failure. On such a 401, it SHALL call `POST /api/auth/refresh` once and, if that succeeds, retry the original request exactly once using the same request options. Only if the refresh call itself fails, or the retried request also returns a non-2xx status, SHALL the error surface to the caller (which today results in a redirect to `/login`).

#### Scenario: Access token expired, refresh token valid
- **WHEN** an authenticated request returns 401 because the access token has expired, and a valid refresh token cookie is present
- **THEN** the frontend SHALL call `/api/auth/refresh`, receive new auth cookies, retry the original request once, and return its result to the caller without the caller observing any error

#### Scenario: Refresh token also invalid or expired
- **WHEN** an authenticated request returns 401 and the subsequent `/api/auth/refresh` call also returns 401
- **THEN** the frontend SHALL surface the original 401 (or the refresh failure) to the caller, which redirects to `/login`

#### Scenario: Concurrent requests hitting 401 around the same time in one tab
- **WHEN** multiple authenticated requests are in flight in the same browser tab and more than one receives a 401 before any refresh has completed
- **THEN** the frontend SHALL issue only one `/api/auth/refresh` call, and all affected requests SHALL wait on its result before each retrying once

#### Scenario: Concurrent requests hitting 401 across multiple tabs of the same browser
- **WHEN** two or more tabs of the same browser (sharing the same `kin_refresh` cookie) each receive a 401 around the same time
- **THEN** the frontend SHALL issue at most one `POST /api/auth/refresh` call across those tabs; a tab that loses the coordination race SHALL NOT submit the refresh token that another tab has already rotated, and SHALL instead retry its original request using the cookies set by the winning tab's refresh

#### Scenario: 401 from the login or refresh endpoints themselves
- **WHEN** `/api/auth/login` returns 401 (bad credentials) or `/api/auth/refresh` returns 401 (dead refresh token)
- **THEN** the frontend SHALL NOT attempt a further refresh-and-retry for that request, and SHALL surface the 401 directly to the caller

#### Scenario: Retry also fails after a successful refresh
- **WHEN** `/api/auth/refresh` succeeds but the retried original request still returns a non-2xx status
- **THEN** the frontend SHALL surface that response's error to the caller without attempting a second refresh

### Requirement: Login attempt limiting

The system SHALL track failed `POST /api/auth/login` attempts per username (case-insensitively —
an accepted defense-in-depth simplification, not a claim that account lookup itself is
case-insensitive) in a trailing 15-minute sliding window. An unknown username and a known username
with an incorrect password SHALL be recorded identically, so that whether a lockout is in effect
for a given username SHALL NOT be usable to infer whether that username exists.

When 5 failed attempts accumulate for a username within the window, that username SHALL enter a
lockout: further `POST /api/auth/login` requests for that username, including ones presenting
correct credentials, SHALL be rejected with HTTP 429 and a JSON body of
`{"error": "too_many_attempts", "retry_after_seconds": <n>}`, with no cookies set, until the
lockout expires. Entering a lockout SHALL clear that username's trailing-window failure count (but
not its backoff level), so that escalation to the next lockout duration requires a fresh batch of 5
failed attempts recorded after the previous lockout expires.

Lockout duration SHALL start at 1 minute and double on each subsequent lockout triggered for the
same username without an intervening successful login, capped at 30 minutes, and SHALL reset to
1 minute after 24 hours have passed with no failed attempts recorded for that username. A
successful login for a username SHALL immediately clear its failure count and backoff level.

A lockout in effect for a username SHALL NOT affect that username's existing, already-issued
access or refresh tokens: `POST /api/auth/refresh`, `POST /api/auth/logout`, and `RequireAuth`
-protected requests SHALL continue to work normally regardless of any concurrent lockout on
`POST /api/auth/login` for the same user.

This limiter SHALL be maintained in-process (no new database table or `kin-core` dependency) and
SHALL NOT require any background goroutine to expire or evict its state.

The check for whether a username is currently permitted to attempt login, and the reservation of
capacity for this attempt, SHALL happen as a single atomic operation performed before credential
verification begins. Credential verification (bcrypt) SHALL NOT be completed, in whole or in part,
before this admission check, so that multiple concurrent login requests for the same username can
never collectively have more than 5 attempts undergoing credential verification at once, regardless
of how long credential verification takes. Only an attempt whose credential verification actually
fails SHALL be recorded against that username's failure count and be capable of tripping a lockout;
an admitted attempt that succeeds SHALL NOT be counted as a failure, even transiently, and a burst
of concurrent successful logins for a username SHALL NOT be capable of triggering a lockout for
that username.

The limiter's state SHALL be bounded in size. When admitting a new, not-yet-tracked username would
exceed that bound and no existing tracked username is eligible for eviction (i.e. every tracked
username currently has an active lockout, a nonzero recent-failure count, an attempt currently
undergoing credential verification, or recent activity), the system SHALL reject the new
username's login attempt with the same HTTP 429 lockout response rather than admitting it
untracked. An established username's lockout, failure count, or in-flight verification attempt
SHALL NOT be evicted to make room for a different, newly-seen username.

A login attempt for a username that does not exist SHALL still perform a bcrypt comparison
(against a fixed placeholder hash) before returning HTTP 401, so that an unknown-username response
is not measurably faster than a known-username/wrong-password response.

#### Scenario: Fifth failed attempt trips the lockout

- **GIVEN** a username has had 4 failed login attempts within the last 15 minutes
- **WHEN** a 5th failed attempt is made for that username
- **THEN** the system SHALL reject that attempt as usual (HTTP 401) and SHALL reject any further
  login attempt for that username with HTTP 429 until the lockout expires

#### Scenario: Correct credentials during a lockout are still rejected

- **GIVEN** a username is currently locked out
- **WHEN** a login request for that username presents the correct password
- **THEN** the system SHALL return HTTP 429 with `{"error": "too_many_attempts", ...}` rather than
  succeeding

#### Scenario: Unknown username counts toward the same limiter

- **GIVEN** a username that does not exist has had 4 failed login attempts within the last 15
  minutes
- **WHEN** a 5th login attempt is made for that same username string
- **THEN** the system SHALL apply the same lockout as it would for a real account with 5 failed
  password attempts

#### Scenario: Successful login resets the counter

- **GIVEN** a username has 3 failed login attempts recorded within the window
- **WHEN** that username then logs in successfully
- **THEN** the system SHALL clear its failure count and backoff level, so a subsequent single
  failed attempt does not immediately re-trigger a lockout

#### Scenario: Backoff escalates on repeated lockouts

- **GIVEN** a username has already triggered and served one 1-minute lockout, with no successful
  login since
- **WHEN** that username accumulates 5 more failed attempts and trips a second lockout
- **THEN** the second lockout's duration SHALL be 2 minutes, not 1

#### Scenario: Existing session survives a lockout on the same account

- **GIVEN** a user is already logged in (holds a valid access/refresh token pair) and their
  username subsequently becomes locked out due to failed login attempts (e.g. an attacker guessing
  their password)
- **WHEN** the user's app makes an authenticated request or calls `POST /api/auth/refresh` using
  their existing tokens
- **THEN** the system SHALL serve that request normally, unaffected by the concurrent login
  lockout on their username

#### Scenario: Backoff resets after a quiet day

- **GIVEN** a username triggered a lockout more than 24 hours ago and has had no failed attempts
  since
- **WHEN** that username next accumulates 5 failed attempts and trips a new lockout
- **THEN** the new lockout's duration SHALL be 1 minute, not a continuation of the previous
  escalated backoff

#### Scenario: Concurrent attempts cannot exceed the threshold

- **GIVEN** a username has no failed attempts recorded and is not locked out
- **WHEN** more than 5 login requests for that username arrive concurrently, all with incorrect
  passwords, and credential verification for each takes measurable time
- **THEN** at most 5 of them SHALL be admitted to attempt credential verification, and every
  request beyond the 5th SHALL be rejected with HTTP 429, even though all requests observed the
  username as not-yet-locked at the moment they arrived

#### Scenario: Concurrent correct-password attempts never trip a lockout

- **GIVEN** a username has no failed attempts recorded and is not locked out
- **WHEN** 5 or more login requests for that username arrive concurrently, all with the correct
  password, and credential verification for each takes measurable time
- **THEN** none of them SHALL result in a lockout being triggered, and any request rejected while
  credential-verification capacity was temporarily full SHALL NOT count toward the username's
  failure total — a login attempt for that username made immediately after all concurrent requests
  finish SHALL succeed rather than being rejected with HTTP 429

#### Scenario: A saturated limiter fails closed for new usernames

- **GIVEN** the limiter's tracked-username state is at its size bound and every tracked username
  currently has an active lockout, a nonzero recent-failure count, an in-flight (currently
  verifying) attempt, or recent activity
- **WHEN** a login attempt is made for a username with no existing tracked state
- **THEN** the system SHALL reject that attempt with HTTP 429, rather than admitting it without
  recording or rate-limiting it

#### Scenario: An in-flight attempt is never evicted while credential verification is pending

- **GIVEN** a username has just been admitted for its very first login attempt (zero confirmed
  failures, no lockout, no prior recorded activity) and that attempt's credential verification is
  still in progress
- **WHEN** the limiter's lazy sweep or ceiling eviction runs concurrently, for any reason including
  the map being at its size bound
- **THEN** that username's entry SHALL NOT be evicted, and the in-flight attempt SHALL resolve
  (via `recordFailure` or `recordSuccess`) against the same entry it was admitted into

#### Scenario: Unknown username takes comparable time to a wrong password

- **GIVEN** a username that does not exist in the system
- **WHEN** a login attempt is made for that username
- **THEN** the system SHALL perform a bcrypt comparison before returning HTTP 401, so that the
  response is not measurably faster than a login attempt for an existing username with an
  incorrect password

