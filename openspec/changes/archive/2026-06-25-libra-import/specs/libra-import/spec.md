## ADDED Requirements

### Requirement: Libra CSV import endpoint
The system SHALL provide `POST /api/import/libra` accepting a multipart form upload with a `file` field containing a Libra CSV export. The endpoint SHALL require authentication. On success it SHALL return HTTP 200 with a JSON object containing integer counts for each imported type (e.g. `{"weight": 1209, "body_fat": 6}`).

#### Scenario: Successful import
- **WHEN** an authenticated user POSTs a valid Libra CSV file
- **THEN** the system returns HTTP 200 with `{"weight": N, "body_fat": M}` where N and M are the counts of records written

#### Scenario: Missing file field
- **WHEN** the request does not include a `file` field in the multipart form
- **THEN** the system returns HTTP 400

#### Scenario: Wrong CSV version
- **WHEN** the uploaded file has a `#Version:` header with a value other than `6`
- **THEN** the system returns HTTP 422

#### Scenario: Non-kg units
- **WHEN** the uploaded file has a `#Units:` header with a value other than `kg`
- **THEN** the system returns HTTP 422

#### Scenario: Unauthenticated request
- **WHEN** the request has no valid session
- **THEN** the system returns HTTP 401

### Requirement: Libra CSV parsing
The system SHALL parse the Libra CSV format: semicolon-delimited rows, `#`-prefixed comment/header lines skipped, columns `date;weight;weight trend;body fat;body fat trend;muscle mass;muscle mass trend;log`. The `date` column SHALL be parsed as an RFC3339 timestamp. The `weight` column SHALL be stored as kilograms. The `body fat` column SHALL be imported as a `BodyFat` percentage record when non-empty.

#### Scenario: Weight row imported
- **WHEN** a data row has a non-empty `weight` value
- **THEN** a `Weight` record is created with that timestamp and kilogram value

#### Scenario: Body fat row imported when present
- **WHEN** a data row has a non-empty `body fat` value
- **THEN** a `BodyFat` record is created with the same timestamp and percentage value

#### Scenario: Ignored columns
- **WHEN** a row contains `weight trend`, `body fat trend`, `muscle mass`, `muscle mass trend`, or `log` values
- **THEN** those values are not stored

### Requirement: Per-calendar-date deduplication
The system SHALL deduplicate input rows to the first occurrence per UTC calendar date before importing. Subsequent rows for the same date SHALL be silently dropped.

#### Scenario: Duplicate rows for the same date
- **WHEN** the CSV contains two rows with timestamps on the same UTC calendar date
- **THEN** only the first row is imported; the second is silently ignored

### Requirement: Idempotent re-import
Re-uploading the same CSV SHALL NOT create duplicate records. The system SHALL upsert on `(user_id, time)`.

#### Scenario: Re-upload same file
- **WHEN** a user uploads the same Libra CSV twice
- **THEN** record counts in the database are unchanged after the second upload
