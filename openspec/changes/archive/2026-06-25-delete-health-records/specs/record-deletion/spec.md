## ADDED Requirements

### Requirement: Authenticated user can delete an owned health record
The system SHALL provide a `DELETE /api/data/{type}/{id}` endpoint. The endpoint SHALL permanently remove the record from storage when the authenticated user owns the record. The `{type}` parameter SHALL be validated against the known type registry; unknown types SHALL return 404. The `{id}` parameter SHALL be a UUID. If the record does not exist or belongs to a different user, the endpoint SHALL return 404 (not 403, to avoid information disclosure).

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

### Requirement: UI provides per-row delete with inline confirmation
The frontend data-type table SHALL display a delete affordance (trash icon) on each row. Activating the delete affordance SHALL transition the row into a confirm state rather than immediately deleting. In the confirm state the row SHALL display Confirm and Cancel controls. Confirming SHALL call the delete endpoint and, on success, remove the row from the displayed list without a full page reload. Cancelling SHALL return the row to its normal state. At most one row SHALL be in confirm state at a time — activating delete on a second row SHALL automatically cancel the first.

#### Scenario: Delete icon visible on each row
- **WHEN** the data-type table renders records
- **THEN** each row shows a trash icon button in a dedicated action column

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
