## MODIFIED Requirements

### Requirement: Authenticated user can delete an owned health record
The system SHALL provide a `DELETE /api/data/{type}/{id}` endpoint. The endpoint SHALL permanently remove the record from storage when the authenticated user owns the record. The `{type}` parameter SHALL be validated against the known type registry; unknown types SHALL return 404. The `{id}` parameter SHALL be a UUID. If the record does not exist or belongs to a different user, the endpoint SHALL return 404 (not 403, to avoid information disclosure).

Deletion SHALL account for types that own dependent rows or files. A type in the registry MAY declare cleanup beyond the single-row delete; deleting `food_meal` SHALL remove the meal's `FoodItem` rows and its stored photo file in the same operation, so that a generic row delete cannot leave orphaned items or files behind.

#### Scenario: Successful delete of own record
- **WHEN** an authenticated user sends `DELETE /api/data/weight/<id>` where `<id>` is a record owned by that user
- **THEN** the server returns HTTP 204 and the record is permanently removed from the database

#### Scenario: Delete with unknown type
- **WHEN** an authenticated user sends `DELETE /api/data/unknown_type/<id>`
- **THEN** the server returns HTTP 404

#### Scenario: Delete of another user's record
- **WHEN** an authenticated user sends `DELETE /api/data/weight/<id>` where `<id>` belongs to a different user
- **THEN** the server returns HTTP 404 and the record is not deleted

#### Scenario: Delete of non-existent record
- **WHEN** an authenticated user sends `DELETE /api/data/steps/<id>` for an ID that does not exist
- **THEN** the server returns HTTP 404

#### Scenario: Unauthenticated delete attempt
- **WHEN** a request without a valid JWT sends `DELETE /api/data/steps/<id>`
- **THEN** the server returns HTTP 401

#### Scenario: Delete of a meal cascades to items and photo
- **WHEN** an authenticated user sends `DELETE /api/data/food_meal/<id>` for a meal they own that has items and a stored photo
- **THEN** the server returns HTTP 204, and the meal row, its `FoodItem` rows, and the photo file are all removed

#### Scenario: Delete of another user's meal
- **WHEN** an authenticated user sends `DELETE /api/data/food_meal/<id>` for a meal owned by a different user
- **THEN** the server returns HTTP 404, and neither the meal, its items, nor its photo are removed
