# user-settings Specification

## Purpose
TBD - created by archiving change dashboard-card-reorder. Update Purpose after archive.
## Requirements
### Requirement: Per-user settings storage

The system SHALL provide a per-user, family-isolated settings store holding an arbitrary JSON object, addressable only by the authenticated user's own identity. At most one settings row SHALL exist per user.

Two keys in that object are given specific meaning by the `food-day-completeness` capability and are documented here as part of the settings contract, though the store itself remains an opaque blob with no schema enforcement:

- `timezone` — an IANA timezone name (e.g. `Europe/Warsaw`) used to compute the boundary of a Logged Day for food-completeness purposes. Absent, empty, or not a valid IANA zone name SHALL be treated as `UTC` by any reader, never as an error.
- `usual_meals_per_day` — a positive integer, the user's own expected Eating Occasion count per day, used as the auto-Complete threshold. Absent or not a positive integer SHALL be treated as `3` by any reader.

Neither key SHALL be validated or rejected by the settings write endpoint itself (see "Settings read/write API" — the store stays a schema-free blob); validation, defaulting, and interpretation are the reading feature's responsibility, per the existing pattern already followed by `dashboard_order` and `display_language`.

#### Scenario: New user has no settings row yet

- **WHEN** an authenticated user who has never saved settings requests their settings
- **THEN** the system SHALL return an empty settings object rather than an error, and SHALL NOT create a row as a side effect of reading

#### Scenario: Settings are isolated per user

- **WHEN** two different users each save different settings
- **THEN** each user's settings requests SHALL return only their own saved values, never another user's

#### Scenario: Missing or invalid timezone/usual_meals_per_day are not write-time errors

- **WHEN** an authenticated user writes settings containing no `timezone` key, no `usual_meals_per_day` key, or values that are not a valid IANA zone name / positive integer, respectively
- **THEN** the write SHALL succeed unchanged (the store does not validate these keys); any feature reading them back SHALL apply its own default (`UTC`, `3`) rather than erroring

### Requirement: Settings read/write API

The system SHALL expose `GET /api/users/me/settings` to return the authenticated user's current settings object, and `PUT /api/users/me/settings` to replace it with a new full settings object. Both endpoints SHALL require authentication and return 401 for unauthenticated requests. When a `PUT` changes the stored `timezone` value to something different from what it was before the write, it SHALL additionally delete all of the caller's existing day confirmations, per the `food-day-completeness` capability's "Day confirmation storage and lifecycle" requirement — this is the one side effect this otherwise schema-free store has on data outside itself. This comparison is on the raw stored string (including absent vs. present), not on the effective, defaulted-to-UTC zone: writing an explicit `"UTC"` over a previously-absent or previously-invalid `timezone` value counts as a change and triggers the cascade delete, even though both resolve to the same effective zone. The settings write and that cascade delete SHALL be performed within the same database transaction, so a failure partway through cannot leave a stale confirmation tied to a `timezone` the settings row no longer reflects.

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
requirement, three do: the dashboard-order editor (on `/`), the Display Language switcher (on
`/settings`), and the profile form (`user-profile`, also on `/settings`). A feature performing its
own independent `GET`-then-`PUT` outside the shared queue SHALL NOT be considered compliant, even
if it happens not to race in ordinary use — the failure surfaces whenever two writes land in the
same session close enough together that a stale in-flight read can still clobber the other's
already-saved write. The queue is mounted at the root layout, so it SHALL keep serializing writes
across a client-side route change between `/` and `/settings` — a full page reload starts a new
session and is out of scope.

#### Scenario: Two writers in the same session both persist

- **WHEN** a user saves a dashboard reorder on `/` and then, in the same session, navigates
  client-side to `/settings` and changes their Display Language
- **THEN** both the reordered `dashboard_order` and the new `display_language` SHALL be present on
  the next read — neither write SHALL be silently lost

#### Scenario: Three writers in the same session all persist

- **WHEN** a user saves a dashboard reorder on `/`, then, in the same session, navigates
  client-side to `/settings` and changes their Display Language and saves the profile form there
  with no navigation between those two
- **THEN** all three changes SHALL be present on the next read

