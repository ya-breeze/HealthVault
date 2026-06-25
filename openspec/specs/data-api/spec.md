## Requirements

### Requirement: User profile endpoint
The system SHALL expose `GET /api/users/me` (requires authentication) that returns the authenticated user's `id`, `username`, and `family_id` as a JSON object.

#### Scenario: Authenticated request
- **WHEN** an authenticated user calls `GET /api/users/me`
- **THEN** the system SHALL return HTTP 200 with `{"id": "<uuid>", "username": "<name>", "family_id": "<uuid>"}`

#### Scenario: Unauthenticated request
- **WHEN** the request carries no valid token
- **THEN** the system SHALL return HTTP 401

---

### Requirement: Generic data query endpoint
The system SHALL expose `GET /api/data/{type}` (requires authentication) where `{type}` is one of the 25 registered health metric types. The endpoint SHALL return a JSON array of all records for the resolved user within the requested time range.

Supported types: `steps`, `heart_rate`, `heart_rate_variability`, `sleep`, `distance`, `active_calories`, `total_calories`, `weight`, `height`, `blood_pressure`, `blood_glucose`, `oxygen_saturation`, `body_temperature`, `skin_temperature`, `respiratory_rate`, `resting_heart_rate`, `exercise`, `hydration`, `nutrition`, `basal_metabolic_rate`, `body_fat`, `lean_body_mass`, `vo2_max`, `bone_mass`, `speed`.

#### Scenario: Query known type
- **WHEN** an authenticated user calls `GET /api/data/steps`
- **THEN** the system SHALL return HTTP 200 with a JSON array of step records for that user in the default time range

#### Scenario: Query unknown type
- **WHEN** the `{type}` path segment does not match any registered type
- **THEN** the system SHALL return HTTP 404

#### Scenario: No records in range
- **WHEN** there are no records for the requested type and time range
- **THEN** the system SHALL return HTTP 200 with an empty JSON array `[]` (not `null`)

---

### Requirement: Time range parameters
All data query endpoints (`/api/data/{type}` and `/api/data/summary`) SHALL accept `?from=` and `?to=` query parameters in RFC3339 format. If `from` is absent or unparseable, it SHALL default to 7 days before the current UTC time. If `to` is absent or unparseable, it SHALL default to the current UTC time.

#### Scenario: Explicit time range
- **WHEN** the request includes `?from=2024-01-01T00:00:00Z&to=2024-01-31T23:59:59Z`
- **THEN** the system SHALL return only records whose primary time field falls within that range

#### Scenario: Default time range applied
- **WHEN** no `from` or `to` parameters are provided
- **THEN** the system SHALL apply a 7-day window ending at the current time

---

### Requirement: Family member data access
All data query endpoints SHALL accept an optional `?user=<username>` query parameter. If absent, the authenticated user's own data is returned. If present, the system SHALL look up the named user and verify they share the same `family_id` as the caller; if they do not, the system SHALL return HTTP 403.

#### Scenario: Query own data (no ?user param)
- **WHEN** an authenticated user calls a data endpoint without `?user=`
- **THEN** the system SHALL return data belonging to that user

#### Scenario: Query family member data
- **WHEN** the `?user=` param names a user in the same family
- **THEN** the system SHALL return that family member's data

#### Scenario: Query user from different family
- **WHEN** the `?user=` param names a user not in the caller's family
- **THEN** the system SHALL return HTTP 403

#### Scenario: Query non-existent user
- **WHEN** the `?user=` param names a user that does not exist
- **THEN** the system SHALL return HTTP 403

---

### Requirement: Summary endpoint
The system SHALL expose `GET /api/data/summary` (requires authentication) that returns aggregate health statistics for the resolved user over the requested time range. The response SHALL be a JSON object with:
- `steps` — total step count (integer)
- `avg_heart_rate` — average BPM across all heart rate records (float)
- `sleep_seconds` — total sleep duration in seconds (integer)

The route SHALL be registered before `GET /api/data/{type}` to prevent the router from treating `"summary"` as a `{type}` variable.

#### Scenario: Summary for user with data
- **WHEN** an authenticated user calls `GET /api/data/summary`
- **THEN** the system SHALL return HTTP 200 with `steps`, `avg_heart_rate`, and `sleep_seconds` for the default 7-day window

#### Scenario: Summary with no data
- **WHEN** there are no records for the user in the requested range
- **THEN** the system SHALL return HTTP 200 with `steps: 0`, `avg_heart_rate: 0`, `sleep_seconds: 0`

#### Scenario: Summary with explicit time range
- **WHEN** `?from=` and `?to=` are provided
- **THEN** the system SHALL aggregate only records within that range

#### Scenario: Summary for family member
- **WHEN** `?user=<family-member>` is provided
- **THEN** the system SHALL return aggregated data for that family member subject to the same access check as the data endpoint
