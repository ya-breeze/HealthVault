<!-- GENERATED FILE — DO NOT EDIT.
     Regenerate with: make projected-specs
     See openspec/specs.projected.README.md for details. -->

# user-settings Specification

## Purpose
TBD - created by archiving change dashboard-card-reorder. Update Purpose after archive.
## Requirements
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

### Requirement: Concurrent-write safety across settings editors

`PUT /api/users/me/settings` replaces the entire settings object with no server-side merge (see
"Replacing settings" above), so round-trip safety against concurrent writes from different frontend
features is entirely the frontend's responsibility. The frontend SHALL serialize every write to the
settings object — from every feature that writes to it — through one shared queue, so that no two
writes issued in the same session can interleave and cause one to silently overwrite the other's
already-saved change with a stale read-modify-write.

This applies regardless of how many independent features write to the blob. As of this
requirement, three do: the dashboard-order editor, the Display Language switcher, and the profile
form (`user-profile`). A feature performing its own independent `GET`-then-`PUT` outside the shared
queue SHALL NOT be considered compliant, even if it happens not to race in ordinary use — the
failure only surfaces when two writes land in the same session with no navigation in between.

#### Scenario: Two writers in the same session both persist

- **WHEN** a user saves a dashboard reorder and then, in the same session with no navigation in
  between, changes their Display Language
- **THEN** both the reordered `dashboard_order` and the new `display_language` SHALL be present on
  the next read — neither write SHALL be silently lost

#### Scenario: Three writers in the same session all persist

- **WHEN** a user saves a dashboard reorder, changes their Display Language, and saves the profile
  form, all in the same session with no navigation in between any of them
- **THEN** all three changes SHALL be present on the next read

