## MODIFIED Requirements

### Requirement: Presence-based filtering of the More Data section

The More Data section SHALL list only secondary (non-primary) registered types for which the resolved user has at least one recorded data point of any age, per `GET /api/data-types/presence` and the same fail-open rules as the vitals grid (a fetch failure, or a type missing from a successful response, means that type is shown). When zero secondary types have presence, the More Data section SHALL NOT render at all — no heading, no empty list. The section's container SHALL carry `data-testid="more-data"`. This presence-based filtering is independent of, and evaluated before, the user's own hide/show preference for the section as a whole (see "User-hidden More Data section") — a user-hidden section SHALL NOT render regardless of what presence filtering alone would have produced, including during a presence-fetch failure. In the read-only view, the section additionally SHALL NOT render until the user's saved hide/show preference has loaded, so that a user-hidden section is never briefly shown before that preference is known.

#### Scenario: Secondary type with data appears in More Data

- **WHEN** the resolved user has at least one record ever for a secondary type, and has not hidden the More Data section
- **THEN** that type's link SHALL appear in the More Data section

#### Scenario: Secondary type with no data ever is omitted

- **WHEN** the resolved user has zero records ever for a secondary type
- **THEN** that type's link SHALL NOT appear in the More Data section

#### Scenario: More Data section absent when nothing qualifies

- **WHEN** the resolved user has zero records ever for every secondary type
- **THEN** the dashboard SHALL NOT render the More Data section (no `more-data` testid element present)

#### Scenario: Presence fetch failure shows all secondary types

- **WHEN** the presence fetch fails, and the user has not hidden the More Data section
- **THEN** the system SHALL render every secondary type in the More Data section, as if every type had presence

#### Scenario: More Data section waits for presence and saved preference before rendering

- **WHEN** the presence fetch has not yet resolved (and has not failed), or the user's saved hide/show preference has not yet loaded (or failed to load)
- **THEN** the dashboard SHALL NOT render the More Data section as filtered-in until both the presence fetch and the saved preference have resolved

## ADDED Requirements

### Requirement: User-hidden More Data section

The dashboard SHALL support a per-user, persisted preference to hide the entire More Data section, independent of presence filtering. Users SHALL toggle this preference from a show/hide control on the More Data section's heading, visible only in the dashboard's existing Edit/Customize mode (the same mode used for vitals-grid card order and visibility), and SHALL persist it via the existing Done action. The control SHALL only be offered when the section would otherwise render (i.e. at least one secondary type currently has presence); when nothing has presence, there is nothing to hide and no control SHALL be shown. Once hidden, the section SHALL NOT render in the read-only view under any presence outcome, including a presence-fetch failure that would otherwise show every secondary type.

#### Scenario: Entering edit mode reveals the More Data hide toggle

- **GIVEN** the resolved user has at least one secondary type with presence
- **WHEN** a user clicks the dashboard's Edit/Customize control
- **THEN** the More Data section heading SHALL show a show/hide control

#### Scenario: No hide toggle when nothing has presence

- **GIVEN** the resolved user has zero secondary types with presence
- **WHEN** a user clicks the dashboard's Edit/Customize control
- **THEN** the More Data section, and its show/hide control, SHALL NOT render

#### Scenario: Hiding the More Data section

- **WHEN** a user in edit mode toggles the More Data section's show/hide control to hidden, and clicks Done
- **THEN** the system SHALL persist the section as hidden, and the read-only view SHALL no longer render the More Data section

#### Scenario: Re-showing the More Data section

- **WHEN** a user in edit mode toggles a previously-hidden More Data section back to visible, and clicks Done
- **THEN** the read-only view SHALL render the More Data section again, subject to presence filtering as normal

#### Scenario: Hidden section stays hidden in edit mode, but discoverable

- **GIVEN** the resolved user has hidden the More Data section and at least one secondary type has presence
- **WHEN** the user enters edit mode
- **THEN** the More Data section (heading, toggle, and link list) SHALL still render, visually distinguished as hidden, so it can be found and re-shown

#### Scenario: User-hidden section overrides presence fail-open

- **GIVEN** the resolved user has hidden the More Data section
- **WHEN** the presence fetch fails
- **THEN** the More Data section SHALL still NOT render in the read-only view, even though a presence-fetch failure would otherwise show every secondary type

#### Scenario: Hidden preference persists across sessions

- **WHEN** a user who previously hid the More Data section loads the dashboard again (including from a different browser/device)
- **THEN** the More Data section SHALL remain hidden in the read-only view, subject to presence filtering as normal once re-shown
