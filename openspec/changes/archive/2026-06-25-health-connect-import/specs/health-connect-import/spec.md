## ADDED Requirements

### Requirement: Upload Health Connect archive
The system SHALL accept a multipart/form-data POST to `/api/import/health-connect` with a file field named `file` containing a zip archive that includes a Health Connect SQLite database (`health_connect_export.db`). The endpoint SHALL require JWT authentication identical to other `/api/*` routes.

#### Scenario: Successful import
- **WHEN** an authenticated user POSTs a valid Health Connect zip to `/api/import/health-connect`
- **THEN** the system SHALL extract the embedded SQLite database, parse all supported record types, ingest them with deduplication, and return HTTP 200 with a JSON object containing per-type import counts (e.g. `{"steps": 5392, "heart_rate": 271942, ...}`)

#### Scenario: Unauthenticated request
- **WHEN** a request is made without a valid JWT
- **THEN** the system SHALL return HTTP 401

#### Scenario: Missing file field
- **WHEN** the multipart request contains no `file` field
- **THEN** the system SHALL return HTTP 400

#### Scenario: Zip does not contain HC database
- **WHEN** the uploaded zip does not contain `health_connect_export.db`
- **THEN** the system SHALL return HTTP 422 with a descriptive error message

### Requirement: Parse and ingest all supported HC record types
The system SHALL read and ingest the following HC tables when present and non-empty: `heart_rate_record_table` (joined with `heart_rate_record_series_table`), `steps_record_table`, `sleep_session_record_table` (joined with `sleep_stages_table`), `exercise_session_record_table`, `distance_record_table`, `total_calories_burned_record_table`, `oxygen_saturation_record_table`, `SpeedRecordTable` (joined with `speed_record_table`).

#### Scenario: All supported types present
- **WHEN** the HC database contains data in all supported tables
- **THEN** the system SHALL ingest records from every table and include each type's count in the response

#### Scenario: Partial data
- **WHEN** some supported tables are empty
- **THEN** the system SHALL ingest the non-empty tables and return zero counts for the empty ones

### Requirement: Unit and enum conversion
The system SHALL convert HC-native representations to HealthVault's internal format before ingesting: timestamps from epoch milliseconds to RFC3339, energy from joules to kcal (÷ 4184), sleep stage integers to string names, exercise type integers to string names. Unknown enum values SHALL be mapped to `"unknown"`.

#### Scenario: Energy conversion
- **WHEN** a `total_calories_burned` record has `energy = 95000` (joules)
- **THEN** the ingested record SHALL store `calories ≈ 22.7` (kcal)

#### Scenario: Unknown sleep stage
- **WHEN** a sleep stage row has an unrecognised `stage_type` integer
- **THEN** the ingested stage SHALL have `stage = "unknown"` and the import SHALL not fail

### Requirement: Deduplication
The system SHALL apply the same upsert semantics as the webhook: records with duplicate (user_id, time) or (user_id, start_time) keys SHALL be updated, not duplicated. Re-importing the same archive SHALL be idempotent.

#### Scenario: Re-import same archive
- **WHEN** the user imports the same zip twice
- **THEN** the second import SHALL succeed and record counts in the database SHALL remain unchanged

### Requirement: Import UI
The frontend SHALL provide an Import page accessible from the main navigation, containing a file picker that accepts `.zip` files and a submit button. After a successful import, the page SHALL display the per-type counts returned by the API.

#### Scenario: Successful upload from UI
- **WHEN** a user selects a valid Health Connect zip and clicks Import
- **THEN** the UI SHALL show a loading indicator during upload and display the import summary on success

#### Scenario: Server error displayed
- **WHEN** the server returns an error response
- **THEN** the UI SHALL display the error message to the user
