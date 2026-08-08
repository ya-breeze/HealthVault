## MODIFIED Requirements

### Requirement: Generic data query endpoint
The system SHALL expose `GET /api/data/{type}` (requires authentication) where `{type}` is one of the 26 registered types. The endpoint SHALL return a JSON array of all records for the resolved user within the requested time range.

Supported types: `steps`, `heart_rate`, `heart_rate_variability`, `sleep`, `distance`, `active_calories`, `total_calories`, `weight`, `height`, `blood_pressure`, `blood_glucose`, `oxygen_saturation`, `body_temperature`, `skin_temperature`, `respiratory_rate`, `resting_heart_rate`, `exercise`, `hydration`, `nutrition`, `basal_metabolic_rate`, `body_fat`, `lean_body_mass`, `vo2_max`, `bone_mass`, `speed`, `food_meal`.

`food_meal` is anchored on `logged_at` and returns meal rows with their aggregate macros but **without** their nested items; the item breakdown is available from `GET /api/food/meals/{id}`. Consumers SHALL NOT sum `food_meal` and `nutrition` together, as they are independent producers of nutrient facts with no shared deduplication key.

The `food_meal` response SHALL be limited to an explicit column allowlist — `id`, `logged_at`, `name`, `status`, and the 7 aggregate macros. It SHALL NOT include `photo_path` or `raw_response`. The generic query path returns whole rows and the frontend renders every column outside a small denylist, so registering the type without an allowlist would publish a server filesystem path and the full raw model response into the API and into rendered table cells.

Because `logged_at` is this type's time anchor, it SHALL also be recognized by the frontend's time-column detection, which otherwise handles only `time`, `start_time`, and `timestamp`.

The endpoint SHALL additionally accept an optional `?bucket=` parameter with value `day`, `week`, or `month`. When present and the type is not `food_meal`, the system SHALL return one aggregated row per bucket instead of raw records, each with `bucket_start` (RFC3339, the bucket's start in UTC), `count` (number of raw records in the bucket), and:
- for cumulative types (`steps`, `distance`, `active_calories`, `total_calories`, `hydration`, `exercise`, `sleep`): `sum` — the bucket's summed value column (`sleep` sums `duration_seconds` per night rather than per calendar bucket boundary).
- for point-in-time types (every other non-`food_meal` type, including both columns of `blood_pressure` reported as `systolic_avg`/`systolic_min`/`systolic_max` and `diastolic_avg`/`diastolic_min`/`diastolic_max`): `avg`, `min`, and `max` of the value column within the bucket.

`?bucket=` on `food_meal` or an unrecognized bucket value SHALL return HTTP 400. Omitting `?bucket=` SHALL preserve today's raw-record response exactly, so existing callers are unaffected.

#### Scenario: Query known type
- **WHEN** an authenticated user calls `GET /api/data/steps`
- **THEN** the system SHALL return HTTP 200 with a JSON array of step records for that user in the default time range

#### Scenario: Query meals through the generic registry
- **WHEN** an authenticated user calls `GET /api/data/food_meal`
- **THEN** the system SHALL return HTTP 200 with a JSON array of that user's meals in the time range, each with its aggregate macros and without nested items

#### Scenario: Meal rows omit internal columns
- **WHEN** a `food_meal` row is returned by the generic query endpoint
- **THEN** the object SHALL NOT contain `photo_path` or `raw_response`

#### Scenario: Query unknown type
- **WHEN** the `{type}` path segment does not match any registered type
- **THEN** the system SHALL return HTTP 404

#### Scenario: No records in range
- **WHEN** there are no records for the requested type and time range
- **THEN** the system SHALL return HTTP 200 with an empty JSON array `[]` (not `null`)

#### Scenario: Bucketed query for a cumulative type
- **WHEN** an authenticated user calls `GET /api/data/steps?bucket=month`
- **THEN** the system SHALL return HTTP 200 with one JSON object per month containing `bucket_start`, `count`, and `sum`, and SHALL NOT include individual raw step records

#### Scenario: Bucketed query for a point-in-time type
- **WHEN** an authenticated user calls `GET /api/data/heart_rate?bucket=week`
- **THEN** the system SHALL return HTTP 200 with one JSON object per week containing `bucket_start`, `count`, `avg`, `min`, and `max`

#### Scenario: Bucketed query for blood pressure
- **WHEN** an authenticated user calls `GET /api/data/blood_pressure?bucket=day`
- **THEN** each returned bucket SHALL include `systolic_avg`, `systolic_min`, `systolic_max`, `diastolic_avg`, `diastolic_min`, and `diastolic_max`

#### Scenario: Bucket parameter rejected for food_meal
- **WHEN** a request calls `GET /api/data/food_meal?bucket=day`
- **THEN** the system SHALL return HTTP 400

#### Scenario: Invalid bucket value
- **WHEN** a request includes `?bucket=hour` or any value other than `day`, `week`, or `month`
- **THEN** the system SHALL return HTTP 400

#### Scenario: Omitting bucket preserves existing behavior
- **WHEN** a request omits `?bucket=`
- **THEN** the system SHALL return raw records exactly as it did before this capability was added
