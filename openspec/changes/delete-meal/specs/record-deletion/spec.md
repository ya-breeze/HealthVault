## ADDED Requirements

### Requirement: Meal review page provides delete with inline confirmation
The meal review page (`/food/review/?meal={id}`) SHALL display a delete affordance for the meal being viewed, available regardless of the meal's status. Activating it SHALL transition into a confirm state showing Confirm and Cancel controls, mirroring the existing trash-icon confirm-then-delete interaction already used for meal items and the generic data-table row delete. Confirming SHALL call `DELETE /api/data/food_meal/{id}` via the existing `deleteRecord` client helper. On success the page SHALL show a success notification and navigate to `/food/history`. On failure the page SHALL show an inline error and remain in the confirm state so the user can retry. While a delete request is in flight, the Confirm control SHALL be disabled to prevent a duplicate request.

#### Scenario: Delete affordance visible on the review page
- **WHEN** the owner opens the review page for one of their meals, of any status
- **THEN** the page shows a delete affordance for that meal

#### Scenario: Click delete enters confirm state
- **WHEN** the user activates the delete affordance
- **THEN** the page shows Confirm and Cancel controls in place of the delete affordance

#### Scenario: Cancel returns to normal
- **WHEN** the user clicks Cancel while in the confirm state
- **THEN** the page returns to its normal appearance with the delete affordance shown again, and no request is sent

#### Scenario: Confirm deletes the meal and returns to history
- **WHEN** the user clicks Confirm
- **THEN** the frontend calls `DELETE /api/data/food_meal/{id}`, and on a successful response shows a success notification and navigates to `/food/history`

#### Scenario: Delete failure keeps the user on the review page
- **WHEN** the delete request fails (network error or server error)
- **THEN** the page shows an inline error, remains on the review page in the confirm state, and does not navigate away

#### Scenario: Confirm is disabled while the request is in flight
- **WHEN** the user has clicked Confirm and the request has not yet completed
- **THEN** the Confirm control is disabled, preventing a second delete request for the same meal
