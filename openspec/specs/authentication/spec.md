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
