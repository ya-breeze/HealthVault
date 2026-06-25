## Requirements

### Requirement: Health metric types
The system SHALL persist health data in typed tables. Each table SHALL belong to a family (via `family_id`) and be associated with a user (via `user_id`). Every row SHALL carry a `source_payload_id` linking it to the raw webhook or import that produced it.

Types are grouped into three categories based on their temporal shape:

**Interval types** — have `start_time` and `end_time` with a unique constraint on `(user_id, start_time)`:

| Type             | Key field        | Unit            |
|------------------|------------------|-----------------|
| Steps            | `count`          | integer count   |
| Distance         | `meters`         | float64         |
| ActiveCalories   | `calories`       | float64 kcal    |
| TotalCalories    | `calories`       | float64 kcal    |
| Hydration        | `liters`         | float64         |
| Exercise         | see below        | —               |
| Nutrition        | see below        | —               |
| Sleep            | see below        | —               |

**Point-in-time types** — have a single `time` field with a unique constraint on `(user_id, time)`:

| Type                 | Key field          | Unit                   |
|----------------------|--------------------|------------------------|
| HeartRate            | `bpm`              | integer beats/min      |
| HeartRateVariability | `rmssd_millis`     | float64 ms             |
| Weight               | `kilograms`        | float64                |
| Height               | `meters`           | float64                |
| BloodGlucose         | `mmol_per_liter`   | float64                |
| OxygenSaturation     | `percentage`       | float64                |
| BodyTemperature      | `celsius`          | float64                |
| RespiratoryRate      | `rate`             | float64 breaths/min    |
| RestingHeartRate     | `bpm`              | integer beats/min      |
| BasalMetabolicRate   | `watts`            | float64                |
| BodyFat              | `percentage`       | float64                |
| LeanBodyMass         | `kilograms`        | float64                |
| VO2Max               | `ml_per_kg_per_min`| float64                |
| BoneMass             | `kilograms`        | float64                |
| Speed                | `meters_per_second`| float64                |

**Multi-value point-in-time types** — single `time` field, unique on `(user_id, time)`, multiple measurement fields:

| Type            | Fields                                                              |
|-----------------|---------------------------------------------------------------------|
| BloodPressure   | `systolic` (float64), `diastolic` (float64)                        |
| SkinTemperature | `delta_celsius` (float64), `baseline_celsius` (nullable float64), `measurement_location` (integer) |

#### Scenario: Interval record stored
- **WHEN** an interval health record is ingested for a user
- **THEN** the system SHALL persist it with the correct `user_id`, `family_id`, `source_payload_id`, `start_time`, `end_time`, and type-specific measurement fields

#### Scenario: Point-in-time record stored
- **WHEN** a point-in-time health record is ingested for a user
- **THEN** the system SHALL persist it with the correct `user_id`, `family_id`, `source_payload_id`, `time`, and measurement field(s)

---

### Requirement: Exercise record
The system SHALL store exercise sessions with required fields `start_time`, `end_time`, `duration_seconds`, and `exercise_type` (string). Optional fields SHALL be nullable: `distance_meters`, `steps`, `avg_cadence_spm`, `max_cadence_spm`, `stride_length_m`. The unique constraint SHALL be on `(user_id, start_time)`.

#### Scenario: Exercise with optional fields absent
- **WHEN** an exercise record is ingested without distance or cadence data
- **THEN** the system SHALL persist the record with null values for those optional fields

#### Scenario: Exercise with optional fields present
- **WHEN** an exercise record is ingested with `distance_meters` and `steps`
- **THEN** the system SHALL persist those values alongside the required fields

---

### Requirement: Nutrition record
The system SHALL store nutrition entries with required fields `start_time` and `end_time`. All nutritional values SHALL be nullable: `calories`, `protein_grams`, `carbs_grams`, `fat_grams`, `sugar_grams`, `sodium_grams`, `dietary_fiber_grams`, `name`. The unique constraint SHALL be on `(user_id, start_time)`.

#### Scenario: Partial nutrition record
- **WHEN** a nutrition entry is ingested with only `calories` populated
- **THEN** the system SHALL persist the record with null for all other nutritional fields

---

### Requirement: Sleep record
The system SHALL store sleep sessions with `start_time`, `session_end_time` (unique anchor), and `duration_seconds`. A sleep record MAY have zero or more child `SleepStage` rows linked by `sleep_id`. Each stage SHALL have `stage` (string), `start_time`, `end_time`, and `duration_seconds`. The unique constraint on Sleep SHALL be on `(user_id, session_end_time)`.

#### Scenario: Sleep with stages
- **WHEN** a sleep session is ingested with stage breakdown
- **THEN** the system SHALL persist the parent Sleep record and all associated SleepStage rows

#### Scenario: Sleep without stages
- **WHEN** a sleep session is ingested without stage data
- **THEN** the system SHALL persist only the parent Sleep record with an empty stages list

---

### Requirement: Webhook audit log
The system SHALL store every raw incoming webhook payload in a `webhook_payloads` table with `user_id`, `received_at`, `app_version`, `payload_ts`, and the full raw JSON string. This table is append-only; no deduplication is applied.

#### Scenario: Webhook payload recorded
- **WHEN** a webhook POST is received
- **THEN** a row SHALL be written to `webhook_payloads` once the payload is parsed; subsequent ingest errors do not prevent the row from being stored
