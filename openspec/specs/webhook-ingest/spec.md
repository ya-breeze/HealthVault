## Requirements

### Requirement: Webhook endpoint
The system SHALL accept `POST /webhook/{username}` where `{username}` identifies the target user. The endpoint SHALL be unauthenticated — no JWT or cookie is required. The request body SHALL be a JSON payload containing arrays of health records for any combination of supported types. The system SHALL return HTTP 204 on success.

#### Scenario: Successful webhook POST
- **WHEN** a client POSTs a valid JSON payload to `/webhook/{username}`
- **THEN** the system SHALL ingest all records in the payload and return HTTP 204

#### Scenario: Unknown username
- **WHEN** the `{username}` in the URL does not match any user in the system
- **THEN** the system SHALL return HTTP 404

#### Scenario: Empty payload
- **WHEN** a POST body contains no records in any type array
- **THEN** the system SHALL return HTTP 204 and write only the audit log entry

---

### Requirement: Webhook payload format
The JSON body SHALL be an object with optional top-level keys for each supported health type, each containing an array of records. The system SHALL also accept `app_version` (string) and `timestamp` (RFC3339 string) metadata fields at the top level.

Supported payload keys and their per-record fields:

| Key                  | Per-record fields                                                                 |
|----------------------|-----------------------------------------------------------------------------------|
| `steps`              | `start_time`, `end_time`, `count`                                                 |
| `heart_rate`         | `time`, `bpm`                                                                     |
| `heart_rate_variability` | `time`, `rmssd_millis`                                                        |
| `sleep`              | `session_end_time`, `duration_seconds`, `stages[]` (start_time derived)           |
| `distance`           | `start_time`, `end_time`, `meters`                                                |
| `active_calories`    | `start_time`, `end_time`, `calories`                                              |
| `total_calories`     | `start_time`, `end_time`, `calories`                                              |
| `weight`             | `time`, `kilograms`                                                               |
| `height`             | `time`, `meters`                                                                  |
| `blood_pressure`     | `time`, `systolic`, `diastolic`                                                   |
| `blood_glucose`      | `time`, `mmol_per_liter`                                                          |
| `oxygen_saturation`  | `time`, `percentage`                                                              |
| `body_temperature`   | `time`, `celsius`                                                                 |
| `skin_temperature`   | `time`, `delta_celsius`, `baseline_celsius`, `measurement_location`               |
| `respiratory_rate`   | `time`, `rate`                                                                    |
| `resting_heart_rate` | `time`, `bpm`                                                                     |
| `exercise`           | `start_time`, `end_time`, `duration_seconds`, `type`, optional fields             |
| `hydration`          | `start_time`, `end_time`, `liters`                                                |
| `nutrition`          | `start_time`, `end_time`, optional nutritional fields                             |
| `basal_metabolic_rate` | `time`, `watts`                                                                 |
| `body_fat`           | `time`, `percentage`                                                              |
| `lean_body_mass`     | `time`, `kilograms`                                                               |
| `vo2_max`            | `time`, `ml_per_kg_per_min`                                                       |
| `bone_mass`          | `time`, `kilograms`                                                               |
| `speed`              | `time`, `meters_per_second`                                                       |

Sleep stages within `sleep[].stages` SHALL carry `stage` (string), `start_time`, `end_time`, `duration_seconds`.

#### Scenario: Mixed-type payload
- **WHEN** a payload contains records for multiple types (e.g. `steps`, `heart_rate`, `sleep`)
- **THEN** the system SHALL ingest all types independently, inserting records into the corresponding tables

---

### Requirement: Upsert deduplication
The system SHALL upsert health records: for interval types, the conflict key is `(user_id, start_time)`; for point-in-time types, the conflict key is `(user_id, time)`. On conflict, all data fields SHALL be overwritten with the incoming values. Re-sending the same payload SHALL be idempotent for all types except Sleep.

#### Scenario: Re-send same steps record
- **WHEN** a steps record for a given `(user_id, start_time)` is received a second time with the same count
- **THEN** the system SHALL update the existing row and the database SHALL contain exactly one row for that key

#### Scenario: Re-send updated heart rate
- **WHEN** a heart rate record for a given `(user_id, time)` is received with a different `bpm` value
- **THEN** the system SHALL overwrite the existing row with the new `bpm`

---

### Requirement: Sleep deduplication semantics
Sleep records SHALL use DO NOTHING on conflict (keyed on `(user_id, session_end_time)`) rather than UPDATE. This prevents re-inserting orphaned child SleepStage rows for an already-stored session.

#### Scenario: Re-send same sleep session
- **WHEN** a sleep session with an already-stored `(user_id, session_end_time)` is received
- **THEN** the system SHALL silently skip the insert and not duplicate either the session or its stages

---

### Requirement: Webhook audit log
The system SHALL write every successfully parsed incoming webhook payload to the `webhook_payloads` table before record ingestion begins, capturing `user_id`, `received_at` (server time), `app_version`, `timestamp` (from the payload), and the full raw JSON string. Payloads that fail JSON parsing are rejected with HTTP 400 before any audit write.

#### Scenario: Audit log entry on success
- **WHEN** a valid webhook payload is received and ingested
- **THEN** a `webhook_payloads` row SHALL exist for that request

#### Scenario: Ingest errors are non-fatal
- **WHEN** a payload is saved to the audit log but record ingestion encounters an error
- **THEN** the system SHALL log the error and still return HTTP 204 (the raw payload is preserved)
