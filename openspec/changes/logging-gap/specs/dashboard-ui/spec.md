## ADDED Requirements

### Requirement: Food Card registry entry
The dashboard's card order/visibility registry (see "Customizable vitals grid order and visibility")
SHALL admit card entries with no backing `DataType` — Food Cards — alongside the existing
`DataType`-backed Vital Cards, sharing one saved order and one Edit/Customize mode. A Food Card
entry SHALL be reorderable and independently hideable exactly like a Vital Card entry, and SHALL
default to visible when a user has never customized their order.

Unlike a Vital Card, a Food Card entry SHALL NOT be subject to the "Presence-based visibility of
vitals grid cards" requirement — it has no `/api/data/{type}` presence signal to gate on, and SHALL
always be eligible to render (subject only to the user's own hidden/visible choice) and always
appear in Edit mode's card list, regardless of whether it currently has content to show. Whether a
Food Card has anything meaningful to display right now is that card's own internal content state
(e.g. a "not enough data yet" message), never a reason for the registry to hide or omit it.

The Logging Gap Card (see `logging-gap`) is this registry's first Food Card entry.

#### Scenario: Food Card entry appears in Edit mode regardless of its content state
- **GIVEN** a caller's Logging Gap Card currently has nothing to show ("not enough data yet")
- **WHEN** the caller enters Edit/Customize mode
- **THEN** the Logging Gap Card's entry appears in the reorder/show-hide list exactly as a Vital
  Card entry would, with move-up/move-down and show/hide controls

#### Scenario: Food Card entry defaults to visible
- **WHEN** a caller who has never customized their dashboard order loads the dashboard
- **THEN** the Logging Gap Card renders among the other default-visible cards, in the application's
  default position for it

#### Scenario: Hiding a Food Card persists like hiding a Vital Card
- **WHEN** a caller hides the Logging Gap Card in Edit mode and clicks Done
- **THEN** the system persists it as hidden while keeping its position in the saved order, the same
  way hiding a Vital Card behaves, and the read-only dashboard no longer renders it

#### Scenario: A Food Card is never hidden by the presence mechanism
- **GIVEN** a caller has a fully populated food log and weight history, so the Logging Gap Card has
  a computed value to show
- **WHEN** the dashboard evaluates which cards to render
- **THEN** the Logging Gap Card's visibility is determined only by the caller's own hidden/visible
  preference, never by a presence check
