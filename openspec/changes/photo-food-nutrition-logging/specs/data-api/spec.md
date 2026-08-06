## MODIFIED Requirements

### Requirement: Generic data query endpoint
The system SHALL expose `GET /api/data/{type}` (requires authentication) where `{type}` is one of the 26 registered types. The endpoint SHALL return a JSON array of all records for the resolved user within the requested time range.

Supported types: `steps`, `heart_rate`, `heart_rate_variability`, `sleep`, `distance`, `active_calories`, `total_calories`, `weight`, `height`, `blood_pressure`, `blood_glucose`, `oxygen_saturation`, `body_temperature`, `skin_temperature`, `respiratory_rate`, `resting_heart_rate`, `exercise`, `hydration`, `nutrition`, `basal_metabolic_rate`, `body_fat`, `lean_body_mass`, `vo2_max`, `bone_mass`, `speed`, `food_meal`.

`food_meal` is anchored on `logged_at` and returns meal rows with their aggregate macros but **without** their nested items; the item breakdown is available from `GET /api/food/meals/{id}`. Consumers SHALL NOT sum `food_meal` and `nutrition` together, as they are independent producers of nutrient facts with no shared deduplication key.

The `food_meal` response SHALL be limited to an explicit column allowlist — `id`, `logged_at`, `name`, `status`, and the 7 aggregate macros. It SHALL NOT include `photo_path` or `raw_response`. The generic query path returns whole rows and the frontend renders every column outside a small denylist, so registering the type without an allowlist would publish a server filesystem path and the full raw model response into the API and into rendered table cells.

Because `logged_at` is this type's time anchor, it SHALL also be recognized by the frontend's time-column detection, which otherwise handles only `time`, `start_time`, and `timestamp`.

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

### Requirement: Family member data access
All data query endpoints SHALL accept an optional `?user=<username>` query parameter. If absent, the authenticated user's own data is returned. If present, the system SHALL look up the named user and verify they share the same `family_id` as the caller; if they do not, the system SHALL return HTTP 403.

This applies to `food_meal` as it does to every other registered type: meal **metadata and aggregate macros** are visible to family members through `GET /api/data/food_meal`, exactly as weight or nutrition already are. It does **not** extend to meal photos or to any mutation — those live under `/api/food/*`, which is owner-only and returns 404 to a family member. A family member can therefore see that a meal totalled 2,100 kcal, and can neither open its photo nor edit it. This split is deliberate: registering the type must not silently widen access to the photo.

#### Scenario: Query own data (no ?user param)
- **WHEN** an authenticated user calls a data endpoint without `?user=`
- **THEN** the system SHALL return data belonging to that user

#### Scenario: Query family member data
- **WHEN** the `?user=` param names a user in the same family
- **THEN** the system SHALL return that family member's data

#### Scenario: Query user from different family
- **WHEN** the `?user=` param names a user not in the caller's family
- **THEN** the system SHALL return HTTP 403

#### Scenario: Query non-existent user
- **WHEN** the `?user=` param names a user that does not exist
- **THEN** the system SHALL return HTTP 403

#### Scenario: Family member reads meal macros but not the photo
- **WHEN** a user calls `GET /api/data/food_meal?user=<family member>` and then requests one of those meals' photos via `GET /api/food/meals/{id}/photo`
- **THEN** the first call SHALL return HTTP 200 with the meals' aggregate macros, and the second SHALL return HTTP 404

## ADDED Requirements

### Requirement: Food route group
The system SHALL expose a `/api/food/*` route group behind the same JWT authentication middleware as `/api/data/*`. Every endpoint SHALL scope its records to the authenticated user and SHALL return HTTP 401 when unauthenticated and HTTP 404 — not 403 — when the target record belongs to another user, matching the convention already established for record deletion.

| Method | Path                                          | Purpose                                        |
|--------|-----------------------------------------------|------------------------------------------------|
| POST   | `/api/food/meals`                             | Multipart photo upload, analysis, meal creation |
| POST   | `/api/food/meals/manual`                      | Create a meal with no photo                     |
| GET    | `/api/food/meals/{id}`                        | Meal detail with items                          |
| GET    | `/api/food/meals/{id}/photo`                  | Stream the stored meal photo                    |
| POST   | `/api/food/meals/{id}/retry`                  | Re-run analysis on the stored photo             |
| POST   | `/api/food/meals/{id}/clarify`                | Submit clarification answers                    |
| PATCH  | `/api/food/meals/{id}/items/{item_id}`        | Resolve one item: bind a food, set macros, set weight |
| PUT    | `/api/food/meals/{id}/confirm`                | Finalize items and weights                      |
| POST   | `/api/food/custom`                            | Create a custom food                            |
| GET    | `/api/food/custom`                            | List the user's custom foods                    |
| PUT    | `/api/food/custom/{id}`                       | Correct a custom food's values                  |
| DELETE | `/api/food/custom/{id}`                       | Delete a custom food                            |
| GET    | `/api/food/search`                            | Search custom + USDA foods                      |
| POST   | `/api/food/calibration-samples`               | Save a weighed ground-truth sample              |
| GET    | `/api/food/calibration-samples`               | List owned samples                              |
| DELETE | `/api/food/calibration-samples/{id}`          | Delete an owned sample and its photo            |
| GET    | `/api/food/calibration-samples/{id}/photo`    | Stream the stored sample photo                  |

#### Scenario: Unauthenticated food request
- **WHEN** a request without a valid JWT calls any `/api/food/*` endpoint
- **THEN** the system SHALL return HTTP 401

#### Scenario: Cross-user food record access
- **WHEN** an authenticated user requests a meal, custom food, or calibration sample owned by a different user
- **THEN** the system SHALL return HTTP 404 and SHALL NOT disclose whether the record exists
