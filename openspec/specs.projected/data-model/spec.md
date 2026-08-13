<!-- GENERATED FILE — DO NOT EDIT.
     Regenerate with: make projected-specs
     See openspec/specs.projected.README.md for details. -->

## Purpose

Defines the typed database schema for ingested health metrics — family- and user-scoped tables with source-payload lineage — plus the exercise, nutrition, sleep, and webhook audit log record shapes.
## Requirements
### Requirement: Health metric types
The system SHALL persist health data in typed tables. Each table SHALL belong to a family (via `family_id`) and be associated with a user (via `user_id`).

Health metric tables are **ingested** data: every row SHALL carry a `source_payload_id` linking it to the raw webhook or import that produced it. This lineage rule applies to the ingested metric types listed below. It does NOT apply to user-authored food logging tables (`FoodMeal`, `FoodItem`, `CustomFood`, `FoodCalibrationSample`), which have no upstream payload and are specified separately under "Food logging tables".

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

#### Scenario: User-authored food record stored without payload lineage
- **WHEN** a user-authored food logging record is created
- **THEN** the system SHALL persist it with the correct `user_id` and `family_id` and SHALL NOT require or synthesize a `source_payload_id`

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

This table is written **only** by webhook ingest and file import. Photo and manual food logging SHALL NOT write to it, because those records have no source payload and would be indistinguishable from ingested rows, producing undetectable double counting for any consumer that sums the table.

#### Scenario: Partial nutrition record
- **WHEN** a nutrition entry is ingested with only `calories` populated
- **THEN** the system SHALL persist the record with null for all other nutritional fields

#### Scenario: Food logging does not write nutrition rows
- **WHEN** a user confirms a photo-logged or manually entered meal
- **THEN** the system SHALL NOT create, update, or delete any row in the `Nutrition` table

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

### Requirement: Food logging tables

The system SHALL persist user-authored food logging data in five family-scoped tables. Each SHALL embed the shared tenant model (`id`, `created_at`, `updated_at`, `deleted_at`, `family_id`) and carry a `user_id`. None SHALL carry a `source_payload_id`.

| Table                     | Purpose                                                        | Time anchor    |
|---------------------------|------------------------------------------------------------------|----------------|
| `FoodMeal`                | One logged meal: photo path, status, aggregate macros           | `logged_at`    |
| `FoodItem`                | One food within a meal: reference, weight, 7 scaled macros      | via `meal_id`  |
| `CustomFood`              | A user's own per-100g food profile                              | —              |
| `FoodCalibrationSample`   | A weighed-food ground-truth photo for model benchmarking        | `captured_at`  |
| `FoodSearchTranslation`   | A user's cached free-text-to-USDA-vocabulary query translation  | —              |

`FoodMeal.status` SHALL be one of `processing`, `pending_clarification`, `pending_review`, `confirmed`, `failed`, and `FoodMeal.logged_at` SHALL always be non-zero. Nutrient field names SHALL match the existing `Nutrition` model (`dietary_fiber_grams`, `sodium_grams`) so the two are directly comparable.

`FoodItem` SHALL carry `macro_source`, one of:

| Value       | Meaning                                                     | In the meal aggregate |
|-------------|---------------------------------------------------------------|-----------------------|
| `reference` | Bound to an `fdc_id`, `off_code`, or `custom_food_id`; macros scaled by weight | yes    |
| `manual`    | Macro values supplied directly by the user                    | yes                    |
| `none`      | Unresolved; macros zero, awaiting user resolution              | no                     |

`fdc_id`, `off_code`, and `custom_food_id` SHALL all be nullable, and at most one of the three SHALL be set on a given `FoodItem`. `macro_source` replaces a plain matched/unmatched boolean because "bound to a reference food" and "has usable macros" are different questions, and a manually entered item is the case where they diverge.

`FoodItem` SHALL also carry `preparation`, `state`, and `brand`, each permitted to be empty for unknown. They are persisted rather than merely used in-flight so that a later clarification answer can re-run food lookup without re-analyzing the photo. `brand` additionally determines whether the Open Food Facts index is queried during matching (see `usda-nutrition-database` "Match Selection and Explicit Non-Match").

`CustomFood` SHALL be uniquely indexed on `(user_id, name)`, so that name-based precedence over USDA and Open Food Facts entries has exactly one winner.

`FoodSearchTranslation` SHALL be uniquely indexed on `(user_id, original_query)`, where `original_query` is the trimmed, lowercased free-text search string, so that each user has at most one cached translation per normalized query. It carries no reference-source fields (`fdc_id`, `off_code`, `custom_food_id`) and does not participate in the FoodItem reference-source exclusivity rule below — it caches a translated search term, not a bound reference food.

There SHALL be no unique constraint on `(user_id, logged_at)` for `FoodMeal`, because a user may legitimately log more than one meal at the same recorded time.

#### Scenario: Meal stored with items

- **WHEN** a meal is created with recognized food items
- **THEN** the system SHALL persist the `FoodMeal` row and its `FoodItem` rows with matching `family_id`, the same `user_id` as the parent meal, and a `meal_id` link

#### Scenario: Items are user-scoped, not only family-scoped

- **WHEN** a `FoodItem` row is created
- **THEN** it SHALL carry the owning `user_id`, because the shared tenant model supplies only `family_id` and every ownership rule in this capability is scoped by user

#### Scenario: Two meals at the same logged time

- **WHEN** a user logs two separate meals that carry the same `logged_at` value
- **THEN** both SHALL persist as distinct rows without conflict

#### Scenario: Tenant fields assigned explicitly

- **WHEN** any food logging row is created
- **THEN** the system SHALL assign `id` and `family_id` explicitly, because the shared tenant model provides no `BeforeCreate` hook to populate them

#### Scenario: A FoodItem cannot bind to more than one reference source

- **WHEN** a `FoodItem` is created or updated with more than one of `fdc_id`, `off_code`, and `custom_food_id` set
- **THEN** the system SHALL reject it, since exactly which field is set is what identifies the reference source and more than one set would be ambiguous

#### Scenario: FoodSearchTranslation rows are private to the user who created them

- **WHEN** a `FoodSearchTranslation` row is created
- **THEN** it SHALL carry the owning `user_id`, and a lookup for that cached translation SHALL only match rows owned by the requesting user

