<!-- GENERATED FILE — DO NOT EDIT.
     Regenerate with: make projected-specs
     See openspec/specs.projected.README.md for details. -->

# dashboard-ui Specification

## Purpose
TBD - created by archiving change dashboard-instrument-panel. Update Purpose after archive.
## Requirements
### Requirement: Shared instrument-panel header
The system SHALL render a single shared header/navigation component (app name, active user, webhook link, custom foods link, import link, logout) on every authenticated page — the dashboard, every data page, the food logging flow, and the import page. Only one header implementation SHALL exist in the frontend; pages SHALL NOT each render their own copy.

#### Scenario: Header present on the dashboard
- **WHEN** an authenticated user loads the dashboard
- **THEN** the shared header SHALL be visible with the active user, webhook, custom foods, import, and logout controls

#### Scenario: Header present on a data page
- **WHEN** an authenticated user loads `/data/steps`
- **THEN** the same shared header component SHALL be visible, unchanged in structure from the dashboard's header

#### Scenario: Header present on food logging and import pages
- **WHEN** an authenticated user loads the photo upload, manual entry, or import page
- **THEN** the shared header SHALL be visible, even though the page content below it is unchanged by this capability

### Requirement: Dashboard vitals grid
The dashboard SHALL replace the 3-card summary and separate "Browse Data" grid with a single vitals grid covering the 8 primary metrics: `steps`, `heart_rate`, `sleep`, `heart_rate_variability`, `distance`, `weight`, `blood_pressure`, `oxygen_saturation`. Each card SHALL show the metric's current value, a 7-day sparkline, and a trend indicator, and SHALL link to that metric's data page.

#### Scenario: Vitals grid renders all 8 primary metrics
- **WHEN** an authenticated user with data in all 8 primary metrics loads the dashboard
- **THEN** the system SHALL render 8 vital cards, one per metric, each showing a current value and a sparkline

#### Scenario: Vital card links to its data page
- **WHEN** a user clicks a vital card
- **THEN** the system SHALL navigate to that metric's `/data/{type}` page

#### Scenario: Missing data for a metric
- **WHEN** the resolved user has no records for one of the 8 primary metrics in the last 7 days
- **THEN** that metric's card SHALL indicate no data rather than rendering an empty or broken sparkline

### Requirement: Secondary data types remain reachable
Registered types outside the 8-metric vitals grid SHALL remain reachable from the dashboard as a compact link list, preserving today's full-catalog access without giving every type a full vital card.

#### Scenario: Secondary type link navigates correctly
- **WHEN** a user clicks a secondary type's link on the dashboard
- **THEN** the system SHALL navigate to that type's `/data/{type}` page, identically to a vitals-grid card

### Requirement: Consistent per-metric color
Each registered data type SHALL be assigned exactly one accent color, used consistently for that type's dashboard card, sparkline, and data-page chart. The mapping SHALL be defined once and imported wherever a metric is rendered, not duplicated per page.

#### Scenario: Same color on dashboard and data page
- **WHEN** a user views a metric's color on its dashboard vital card and then opens that metric's data page
- **THEN** the accent color SHALL be identical in both places

### Requirement: Icon set replaces emoji
User-facing action controls (photo capture, manual entry, and other iconized actions previously rendered as emoji) SHALL use the app's inline-SVG icon set instead of emoji characters.

#### Scenario: Food logging actions use SVG icons
- **WHEN** a user views the "Log food" actions on the dashboard
- **THEN** the photo and manual-entry actions SHALL render inline SVG icons, not emoji glyphs

### Requirement: Needs-attention indicator
The dashboard SHALL show an indicator when the authenticated user has one or more logged meals needing attention (as counted by `GET /api/food/meals/needs-attention-count`), stating the count and linking to the meal history page (`/food/history/`). The indicator SHALL NOT be rendered when the count is zero.

#### Scenario: Indicator shown when meals need attention
- **GIVEN** the resolved user has 2 meals needing attention
- **WHEN** they load the dashboard
- **THEN** an indicator is visible stating that 2 meals need attention

#### Scenario: Indicator hidden when nothing needs attention
- **GIVEN** the resolved user has no meals needing attention (none logged, or all confirmed)
- **WHEN** they load the dashboard
- **THEN** no needs-attention indicator is rendered

#### Scenario: Indicator links to meal history
- **WHEN** a user clicks the needs-attention indicator
- **THEN** the system navigates to `/food/history/`

### Requirement: Customizable vitals grid order

The dashboard vitals grid SHALL support a per-user, persisted custom display order for its cards, independent of which metrics are shown. Users SHALL enter an explicit Edit/Customize mode to change the order; outside that mode, the grid SHALL render using the user's saved order (or the default order if none is saved) with no visible reorder controls.

#### Scenario: Default order for a user who has not customized

- **WHEN** a user who has never saved a custom order loads the dashboard
- **THEN** the vitals grid SHALL render in the application's default metric order

#### Scenario: Entering edit mode reveals reorder controls

- **WHEN** a user clicks the dashboard's Edit/Customize control
- **THEN** each vitals-grid card SHALL show move-up and move-down controls, and the move-up control SHALL be disabled on the first card while the move-down control SHALL be disabled on the last card

#### Scenario: Reordering and saving

- **WHEN** a user in edit mode moves a card and then clicks Done
- **THEN** the system SHALL persist the new order for that user and exit edit mode, and the grid SHALL immediately reflect the new order

#### Scenario: Saved order persists across sessions

- **WHEN** a user who previously saved a custom order loads the dashboard again (including from a different browser/device)
- **THEN** the vitals grid SHALL render in their saved order, not the default order

#### Scenario: Saved order tolerates the default metric set changing

- **WHEN** a user's saved order references a metric no longer in the default set, or the default set has gained a metric absent from their saved order
- **THEN** the system SHALL render only currently-valid metrics in the user's saved relative order, and SHALL append any metric missing from their saved order at the end, without erroring

