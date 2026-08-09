## ADDED Requirements

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
