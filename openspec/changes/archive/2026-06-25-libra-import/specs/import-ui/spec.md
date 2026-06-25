## ADDED Requirements

### Requirement: Libra import card on Import page
The Import page (`/import`) SHALL display a dedicated card for Libra CSV import below the existing Health Connect card. The card SHALL include a file input restricted to `.csv` files, an Import button, and a result table showing per-type counts after a successful import.

#### Scenario: Libra card visible
- **WHEN** an authenticated user navigates to `/import`
- **THEN** both a Health Connect card and a Libra card are visible on the page

#### Scenario: Successful Libra import shows counts
- **WHEN** the user selects a `.csv` file and clicks Import
- **THEN** a result table appears showing `weight` and `body_fat` record counts

#### Scenario: Import error displayed
- **WHEN** the server returns an error response
- **THEN** an error message is displayed within the Libra card

#### Scenario: Loading state during upload
- **WHEN** the import is in progress
- **THEN** the Import button is disabled and shows a loading indicator
