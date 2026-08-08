## Purpose

Lets an authenticated user permanently delete their own health records, via a `DELETE` endpoint and a per-row confirm-then-delete UI in the data table.
## Requirements
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

### Requirement: UI provides per-row delete with inline confirmation
The frontend data-type table SHALL display a delete affordance (trash icon) on each row when viewing the authenticated user's own data. Activating the delete affordance SHALL transition the row into a confirm state rather than immediately deleting. In the confirm state the row SHALL display Confirm and Cancel controls. Confirming SHALL call the delete endpoint and, on success, remove the row from the displayed list without a full page reload. Cancelling SHALL return the row to its normal state. At most one row SHALL be in confirm state at a time — activating delete on a second row SHALL automatically cancel the first. The delete affordance SHALL NOT be shown when viewing another family member's data.

#### Scenario: Delete icon visible on each row
- **WHEN** the data-type table renders records for the authenticated user's own data
- **THEN** each row shows a trash icon button in a dedicated action column

#### Scenario: Delete icon hidden when viewing family member data
- **WHEN** the data-type table renders records for a family member (?user= param set)
- **THEN** no delete affordance or Actions column is shown

#### Scenario: Click trash icon enters confirm state
- **WHEN** the user clicks the trash icon on a row
- **THEN** that row is highlighted and shows Confirm and Cancel buttons; the trash icon is no longer shown

#### Scenario: Cancel returns row to normal
- **WHEN** the user clicks Cancel on a row in confirm state
- **THEN** the row returns to its normal appearance with the trash icon

#### Scenario: Confirm deletes the record
- **WHEN** the user clicks Confirm on a row in confirm state
- **THEN** the frontend calls `DELETE /api/data/{type}/{id}`, the row disappears from the table on success, and no full page reload occurs

#### Scenario: Only one row in confirm state at a time
- **WHEN** the user clicks the trash icon on row B while row A is already in confirm state
- **THEN** row A returns to normal and row B enters confirm state

#### Scenario: Delete failure shows error
- **WHEN** the delete API call fails (network error, server error)
- **THEN** an error message is displayed and the row remains in the table

