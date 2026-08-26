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
All routes under `/api` (except `/api/auth/*`) SHALL be protected by middleware that validates the access token. The token MAY be carried as the `access_token` cookie or as an `Authorization: Bearer <token>` header. Blacklisted tokens SHALL be rejected. The middleware SHALL inject the parsed claims (user_id, family_id) into the request context for downstream handlers.

#### Scenario: Valid token via cookie
- **WHEN** a request to a protected route carries a valid access token cookie
- **THEN** the request SHALL proceed and claims SHALL be available in context

#### Scenario: Valid token via Authorization header
- **WHEN** a request carries `Authorization: Bearer <valid-token>` with no cookie
- **THEN** the request SHALL proceed and claims SHALL be available in context

#### Scenario: Missing token
- **WHEN** a request to a protected route carries no token
- **THEN** the system SHALL return HTTP 401

#### Scenario: Blacklisted token
- **WHEN** a request carries a token that has been blacklisted (e.g. after logout)
- **THEN** the system SHALL return HTTP 401

#### Scenario: Expired token
- **WHEN** a request carries a token whose TTL has elapsed
- **THEN** the system SHALL return HTTP 401

---

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

The system SHALL track failed `POST /api/auth/login` attempts per username (case-insensitively,
matching how `FindUserByName` resolves accounts) in a trailing 15-minute sliding window. An unknown
username and a known username with an incorrect password SHALL be recorded identically, so that
whether a lockout is in effect for a given username SHALL NOT be usable to infer whether that
username exists.

When 5 failed attempts accumulate for a username within the window, that username SHALL enter a
lockout: further `POST /api/auth/login` requests for that username, including ones presenting
correct credentials, SHALL be rejected with HTTP 429 and a JSON body of
`{"error": "too_many_attempts", "retry_after_seconds": <n>}`, with no cookies set, until the
lockout expires.

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

