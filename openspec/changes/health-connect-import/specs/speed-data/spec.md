## ADDED Requirements

### Requirement: Store speed samples
The system SHALL store point-in-time speed measurements (in metres per second) associated with a user, modelled identically to `HeartRate`. The `speeds` table SHALL have a unique constraint on `(user_id, time)` to support idempotent upserts.

#### Scenario: Speed record stored
- **WHEN** a speed sample is ingested for a user at a given timestamp
- **THEN** the system SHALL persist it with the correct `user_id`, `time`, and `meters_per_second` values

#### Scenario: Duplicate speed sample upserted
- **WHEN** a speed sample is ingested for a (user_id, time) that already exists
- **THEN** the system SHALL update the existing row rather than insert a duplicate

### Requirement: Query speed data via API
The system SHALL expose speed samples through the existing generic data endpoint at `GET /api/data/speed`, accepting the same `?from=`, `?to=`, and `?user=` query parameters as other data types.

#### Scenario: Query speed in time range
- **WHEN** an authenticated user requests `GET /api/data/speed?from=<t1>&to=<t2>`
- **THEN** the system SHALL return a JSON array of speed records within the requested range for that user
