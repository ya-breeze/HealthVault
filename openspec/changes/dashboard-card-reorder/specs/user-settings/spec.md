## ADDED Requirements

### Requirement: Per-user settings storage

The system SHALL provide a per-user, family-isolated settings store holding an arbitrary JSON object, addressable only by the authenticated user's own identity. At most one settings row SHALL exist per user.

#### Scenario: New user has no settings row yet

- **WHEN** an authenticated user who has never saved settings requests their settings
- **THEN** the system SHALL return an empty settings object rather than an error, and SHALL NOT create a row as a side effect of reading

#### Scenario: Settings are isolated per user

- **WHEN** two different users each save different settings
- **THEN** each user's settings requests SHALL return only their own saved values, never another user's

### Requirement: Settings read/write API

The system SHALL expose `GET /api/users/me/settings` to return the authenticated user's current settings object, and `PUT /api/users/me/settings` to replace it with a new full settings object. Both endpoints SHALL require authentication and return 401 for unauthenticated requests.

#### Scenario: Reading current settings

- **WHEN** an authenticated user sends `GET /api/users/me/settings`
- **THEN** the system SHALL return their stored settings object (or `{}` if none saved yet)

#### Scenario: Replacing settings

- **WHEN** an authenticated user sends `PUT /api/users/me/settings` with a JSON body
- **THEN** the system SHALL atomically replace their stored settings with that body, creating the row if it did not already exist

#### Scenario: Unauthenticated access is rejected

- **WHEN** a request to either endpoint has no valid authentication
- **THEN** the system SHALL respond 401 and SHALL NOT read or write any settings

#### Scenario: Malformed write is rejected

- **WHEN** an authenticated user sends `PUT /api/users/me/settings` with a body that is not valid JSON
- **THEN** the system SHALL respond 400 and SHALL leave any previously stored settings unchanged
