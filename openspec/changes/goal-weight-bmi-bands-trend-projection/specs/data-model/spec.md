## MODIFIED Requirements

### Requirement: Health metric types
The system SHALL persist health data in typed tables. Each table SHALL belong to a family (via `family_id`) and be associated with a user (via `user_id`).

Health metric tables are **ingested** data: every row SHALL carry a `source_payload_id` linking it to the raw webhook or import that produced it. This lineage rule applies to the ingested metric types listed below. It does NOT apply to user-authored food logging tables (`FoodMeal`, `FoodItem`, `CustomFood`, `FoodCalibrationSample`, `FoodSearchTranslation`), which have no upstream payload and are specified separately under "Food logging tables". It also does NOT apply to any point-in-time record created through the manual write endpoint (`POST /api/data/{type}` — see `data-api`), regardless of type: a manually-entered `weight`, `height`, or `weight_goal` record has no upstream webhook or import payload either, and SHALL NOT require or synthesize a `source_payload_id`.

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
| WeightGoal           | `kilograms`        | float64                |
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

`WeightGoal` differs from every other point-in-time type in this table in one respect: it is never
ingested from a webhook or import. Every row is created through the manual write endpoint. "Latest
record wins" (the most recent `time` is the user's current goal) falls out of ordinary
latest-record-by-time queries — no separate "current goal" field or table exists.

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

#### Scenario: User-authored food record stored without payload lineage
- **WHEN** a user-authored food logging record is created
- **THEN** the system SHALL persist it with the correct `user_id` and `family_id` and SHALL NOT require or synthesize a `source_payload_id`

#### Scenario: Manually-written record stored without payload lineage
- **WHEN** a `weight`, `height`, or `weight_goal` record is created through the manual write endpoint rather than ingested
- **THEN** the system SHALL persist it with the correct `user_id` and `family_id` and SHALL NOT require or synthesize a `source_payload_id`

#### Scenario: Goal weight latest-record-wins
- **WHEN** a user has more than one `weight_goal` record
- **THEN** the system's current-goal reads (chart goal line, BMI/goal consumers) SHALL use the record with the most recent `time`
