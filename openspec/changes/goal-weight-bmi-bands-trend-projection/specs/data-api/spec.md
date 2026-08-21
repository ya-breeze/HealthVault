## ADDED Requirements

### Requirement: Manual record write endpoint
The system SHALL expose `POST /api/data/{type}` (requires authentication), allowlisted to exactly
three types: `weight`, `height`, `weight_goal`. Every other registered type SHALL continue to
reject POST.

The request body SHALL be `{"value": <float64>, "time": "<RFC3339, optional>"}`. `time` SHALL
default to the current time when omitted. A missing, non-numeric, or non-positive (`<= 0`)
`value` SHALL return HTTP 400. On success the system SHALL return HTTP 201 with the created
record in the same JSON shape `GET /api/data/{type}` returns for a single row.

Records created through this endpoint SHALL NOT require or carry a `source_payload_id` (see
`data-model`'s "Health metric types" requirement).

The record SHALL always be created for the authenticated caller. Unlike `GET /api/data/{type}`,
this endpoint SHALL NOT honor a `?user=` family-member override — it does not call the `GET`
path's `resolveUser` helper at all. This matches `DELETE /api/data/{type}/{id}`'s existing
convention of scoping mutations strictly to the caller.

A write whose `(user_id, time)` collides with an existing record of that type — which can only
happen with an explicitly supplied `time`, since an omitted `time` always defaults to the current
timestamp — SHALL return HTTP 409 Conflict and SHALL NOT modify the existing record. This relies
on the type's existing unique `(user_id, time)` constraint rather than a separate upsert path — no
new conflict-resolution logic is introduced by this endpoint beyond surfacing that constraint as a
409.

#### Scenario: Write an allowlisted type
- **WHEN** an authenticated user calls `POST /api/data/weight_goal` with `{"value": 70.0}`
- **THEN** the system SHALL return HTTP 201 with the created record, `kilograms: 70.0`, and a
  `time` defaulted to now

#### Scenario: Write with explicit time
- **WHEN** an authenticated user calls `POST /api/data/height` with `{"value": 1.75, "time": "2026-01-01T00:00:00Z"}`
- **THEN** the system SHALL return HTTP 201 with a record whose `time` is exactly
  `2026-01-01T00:00:00Z`

#### Scenario: Write rejected for a non-allowlisted type
- **WHEN** an authenticated user calls `POST /api/data/steps` with any body
- **THEN** the system SHALL return HTTP 403 and SHALL NOT create a record

#### Scenario: Write rejected for a multi-column type
- **WHEN** an authenticated user calls `POST /api/data/blood_pressure` with any body
- **THEN** the system SHALL return HTTP 403 and SHALL NOT create a record

#### Scenario: Missing value rejected
- **WHEN** an authenticated user calls `POST /api/data/weight` with a body that omits `value` or
  where `value` is not numeric
- **THEN** the system SHALL return HTTP 400 and SHALL NOT create a record

#### Scenario: Non-positive value rejected
- **WHEN** an authenticated user calls `POST /api/data/height` with `{"value": 0}` or a negative
  `value`
- **THEN** the system SHALL return HTTP 400 and SHALL NOT create a record

#### Scenario: Write collides with an existing record at the same time
- **WHEN** an authenticated user calls `POST /api/data/weight_goal` with an explicit `time` that
  already has a `weight_goal` record for that user
- **THEN** the system SHALL return HTTP 409 and SHALL NOT modify the existing record

#### Scenario: Unauthenticated write rejected
- **WHEN** a request to `POST /api/data/weight` carries no valid token
- **THEN** the system SHALL return HTTP 401

#### Scenario: Write always targets the caller, ignoring `?user=`
- **WHEN** an authenticated user calls `POST /api/data/weight_goal?user=<family-member>` with
  `{"value": 70.0}`
- **THEN** the system SHALL create the record for the authenticated caller, not the named family
  member, exactly as if `?user=` had been omitted

## MODIFIED Requirements

### Requirement: Generic data query endpoint
The system SHALL expose `GET /api/data/{type}` (requires authentication) where `{type}` is one of the 27 registered types. The endpoint SHALL return a JSON array of all records for the resolved user within the requested time range.

Supported types: `steps`, `heart_rate`, `heart_rate_variability`, `sleep`, `distance`, `active_calories`, `total_calories`, `weight`, `height`, `weight_goal`, `blood_pressure`, `blood_glucose`, `oxygen_saturation`, `body_temperature`, `skin_temperature`, `respiratory_rate`, `resting_heart_rate`, `exercise`, `hydration`, `nutrition`, `basal_metabolic_rate`, `body_fat`, `lean_body_mass`, `vo2_max`, `bone_mass`, `speed`, `food_meal`.

`food_meal` is anchored on `logged_at` and returns meal rows with their aggregate macros but **without** their nested items; the item breakdown is available from `GET /api/food/meals/{id}`. Consumers SHALL NOT sum `food_meal` and `nutrition` together, as they are independent producers of nutrient facts with no shared deduplication key.

The `food_meal` response SHALL be limited to an explicit column allowlist — `id`, `logged_at`, `name`, `status`, and the 7 aggregate macros. It SHALL NOT include `photo_path` or `raw_response`. The generic query path returns whole rows and the frontend renders every column outside a small denylist, so registering the type without an allowlist would publish a server filesystem path and the full raw model response into the API and into rendered table cells.

Because `logged_at` is this type's time anchor, it SHALL also be recognized by the frontend's time-column detection, which otherwise handles only `time`, `start_time`, and `timestamp`.

The endpoint SHALL additionally accept an optional `?bucket=` parameter with value `day` or `month` — the UI's Week and Month zoom levels both request `day` buckets and differ only in time range, so no separate week-sized bucket exists. When present and the type is not `food_meal`, the system SHALL return one aggregated row per bucket instead of raw records, each with `bucket_start` (RFC3339, the bucket's start in UTC), `count` (number of raw records in the bucket), and:
- for cumulative types (`steps`, `distance`, `active_calories`, `total_calories`, `hydration`, `exercise`, `sleep`): `sum` — the bucket's summed value column (`sleep` sums `duration_seconds` per night rather than per calendar bucket boundary).
- for `nutrition` (also cumulative, but with seven value columns): `sum_calories`, `sum_protein_grams`, `sum_carbs_grams`, `sum_fat_grams`, `sum_sugar_grams`, `sum_sodium_grams`, `sum_dietary_fiber_grams` — each column summed independently within the bucket.
- for point-in-time types (every other non-`food_meal` type, including both columns of `blood_pressure` reported as `systolic_avg`/`systolic_min`/`systolic_max` and `diastolic_avg`/`diastolic_min`/`diastolic_max`): `avg`, `min`, and `max` of the value column within the bucket.

`?bucket=` on `food_meal` or an unrecognized bucket value SHALL return HTTP 400. Omitting `?bucket=` SHALL preserve today's raw-record response exactly, so existing callers are unaffected.

#### Scenario: Query known type
- **WHEN** an authenticated user calls `GET /api/data/steps`
- **THEN** the system SHALL return HTTP 200 with a JSON array of step records for that user in the default time range

#### Scenario: Query goal weight through the generic registry
- **WHEN** an authenticated user calls `GET /api/data/weight_goal`
- **THEN** the system SHALL return HTTP 200 with a JSON array of that user's goal-weight records in the time range, in the same shape as `GET /api/data/weight`

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
- **WHEN** an authenticated user calls `GET /api/data/heart_rate?bucket=day`
- **THEN** the system SHALL return HTTP 200 with one JSON object per day containing `bucket_start`, `count`, `avg`, `min`, and `max`

#### Scenario: Bucketed query for nutrition
- **WHEN** an authenticated user calls `GET /api/data/nutrition?bucket=day`
- **THEN** each returned bucket SHALL include `sum_calories`, `sum_protein_grams`, `sum_carbs_grams`, `sum_fat_grams`, `sum_sugar_grams`, `sum_sodium_grams`, and `sum_dietary_fiber_grams`

#### Scenario: Bucketed query for blood pressure
- **WHEN** an authenticated user calls `GET /api/data/blood_pressure?bucket=day`
- **THEN** each returned bucket SHALL include `systolic_avg`, `systolic_min`, `systolic_max`, `diastolic_avg`, `diastolic_min`, and `diastolic_max`

#### Scenario: Bucket parameter rejected for food_meal
- **WHEN** a request calls `GET /api/data/food_meal?bucket=day`
- **THEN** the system SHALL return HTTP 400

#### Scenario: Invalid bucket value
- **WHEN** a request includes `?bucket=week`, `?bucket=hour`, or any value other than `day` or `month`
- **THEN** the system SHALL return HTTP 400

#### Scenario: Omitting bucket preserves existing behavior
- **WHEN** a request to a registered type omits `?bucket=`
- **THEN** the system SHALL return the existing raw-record response, unaffected by the bucketing feature
