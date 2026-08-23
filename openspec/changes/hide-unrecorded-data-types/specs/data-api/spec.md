## ADDED Requirements

### Requirement: Data type presence endpoint

The system SHALL expose `GET /api/data-types/presence` (requires authentication) that returns, for every type in the type registry (all 26 registered types, spanning both the vitals-grid metrics and the secondary types), whether the resolved user has ever recorded at least one row of that type — computed over all time, independent of any time range.

The response SHALL be a JSON object mapping each registered type name to a boolean (`true` if at least one record exists for that type, `false` if none). A successful (`200`) response SHALL include an entry for every registered type; a type absent from the map on a `200` response indicates a malformed or partial response and SHALL be treated by callers the same as a fetch failure — never as `false`.

This endpoint SHALL accept the same optional `?user=<username>` family-member parameter as other data endpoints (see "Family member data access"), subject to the same same-family authorization check.

#### Scenario: Presence for a user with some recorded types

- **WHEN** an authenticated user who has recorded `steps` and `weight` but nothing else calls `GET /api/data-types/presence`
- **THEN** the system SHALL return HTTP 200 with a JSON object containing one entry per registered type, `steps: true` and `weight: true`, and every other type `false`

#### Scenario: Presence for a user with no recorded data at all

- **WHEN** an authenticated user with zero records of any type calls `GET /api/data-types/presence`
- **THEN** the system SHALL return HTTP 200 with a JSON object containing one entry per registered type, every value `false`

#### Scenario: Presence via family member access

- **WHEN** the `?user=` param names a user in the caller's family
- **THEN** the system SHALL return that family member's presence map, subject to the same access check as other data endpoints

#### Scenario: Query user from different family

- **WHEN** the `?user=` param names a user not in the caller's family
- **THEN** the system SHALL return HTTP 403

#### Scenario: Unauthenticated request

- **WHEN** the request carries no valid token
- **THEN** the system SHALL return HTTP 401
