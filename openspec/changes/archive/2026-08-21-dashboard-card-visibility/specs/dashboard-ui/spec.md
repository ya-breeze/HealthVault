## RENAMED Requirements

- FROM: `### Requirement: Customizable vitals grid order`
- TO: `### Requirement: Customizable vitals grid order and visibility`

## MODIFIED Requirements

### Requirement: Dashboard vitals grid
The dashboard SHALL replace the 3-card summary and separate "Browse Data" grid with a single vitals grid covering the 8 primary metrics: `steps`, `heart_rate`, `sleep`, `heart_rate_variability`, `distance`, `weight`, `blood_pressure`, `oxygen_saturation`. Each card SHALL show the metric's current value, a 7-day sparkline, and a trend indicator, and SHALL link to that metric's data page. Which of those 8 cards actually render is governed by the "Customizable vitals grid order and visibility" requirement.

#### Scenario: Vitals grid renders all 8 primary metrics
- **WHEN** an authenticated user with data in all 8 primary metrics, who has hidden none of them, loads the dashboard
- **THEN** the system SHALL render 8 vital cards, one per metric, each showing a current value and a sparkline

#### Scenario: Vital card links to its data page
- **WHEN** a user clicks a vital card
- **THEN** the system SHALL navigate to that metric's `/data/{type}` page

#### Scenario: Missing data for a metric
- **WHEN** the resolved user has no records for one of the 8 primary metrics in the last 7 days
- **THEN** that metric's card SHALL indicate no data rather than rendering an empty or broken sparkline

### Requirement: Customizable vitals grid order and visibility

The dashboard vitals grid SHALL support a per-user, persisted custom display order AND per-card visibility for its cards. Users SHALL enter an explicit Edit/Customize mode to change order or visibility; outside that mode, the grid SHALL render only the visible cards, in the user's saved order (or the default order, all visible, if none is saved), with no visible reorder or visibility controls.

#### Scenario: Default order for a user who has not customized

- **WHEN** a user who has never saved a custom order/visibility loads the dashboard
- **THEN** the vitals grid SHALL render all metrics in the application's default order, none hidden

#### Scenario: Entering edit mode reveals reorder controls

- **WHEN** a user clicks the dashboard's Edit/Customize control
- **THEN** each vitals-grid card SHALL show move-up, move-down, and a show/hide control, the move-up control SHALL be disabled on the first card, the move-down control SHALL be disabled on the last card, and cards currently hidden SHALL still be shown (visually distinguished as hidden) so they can be found and re-shown

#### Scenario: Hiding a card

- **WHEN** a user in edit mode toggles a visible card's show/hide control to hidden, and clicks Done
- **THEN** the system SHALL persist that card as hidden while keeping its position in the saved order, and the read-only grid SHALL no longer render that card

#### Scenario: Re-showing a hidden card restores its position

- **WHEN** a user in edit mode toggles a previously-hidden card back to visible, and clicks Done
- **THEN** the read-only grid SHALL render that card again in the same position it held before it was hidden, not appended at the end

#### Scenario: Hiding every card shows a placeholder

- **WHEN** a user hides every card in the vitals grid and clicks Done
- **THEN** the system SHALL persist the all-hidden state without error, and the read-only grid SHALL render a placeholder message instead of an empty grid

#### Scenario: Reordering and saving

- **WHEN** a user in edit mode moves a card and then clicks Done
- **THEN** the system SHALL persist the new order (and each card's visibility) for that user and exit edit mode, and the grid SHALL immediately reflect the new order and visibility

#### Scenario: Saved order persists across sessions

- **WHEN** a user who previously saved a custom order/visibility loads the dashboard again (including from a different browser/device)
- **THEN** the vitals grid SHALL render in their saved order, showing only their visible cards

#### Scenario: Saved order tolerates the default metric set changing

- **WHEN** a user's saved order references a metric no longer in the default set, or the default set has gained a metric absent from their saved order
- **THEN** the system SHALL render only currently-valid metrics in the user's saved relative order and visibility, and SHALL append any metric missing from their saved order at the end as visible, without erroring

#### Scenario: Hidden cards are never revealed before the saved settings load

- **WHEN** a user's saved order and visibility have not yet loaded, or failed to load
- **THEN** the dashboard SHALL render a loading or error placeholder in place of the vitals grid, and SHALL NOT render any card until the saved visibility is known, so that cards the user hid are never briefly (or, on failure, indefinitely) shown

#### Scenario: A failed settings load is recoverable without a page reload

- **WHEN** the saved order and visibility failed to load and the user activates the placeholder's retry control
- **THEN** the system SHALL re-attempt the load, and on success SHALL render the vitals grid in the user's saved order and visibility with the customize control re-enabled

#### Scenario: Saved order in the pre-visibility shape still loads

- **WHEN** a user's saved settings still hold the earlier plain list-of-types shape (no per-entry visibility) from before this capability existed
- **THEN** the system SHALL treat every entry in that saved order as visible and render it exactly as it did before, without erroring
