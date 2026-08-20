## ADDED Requirements

### Requirement: Per-Screen Expert Mode Toggle

The frontend SHALL provide an Expert Mode toggle, independently available on the meal
recognition/confirmation screen, the food history detail view, and the Custom Food catalog
screen. Expert Mode state SHALL be held only in client-side UI state for the current screen — it
SHALL NOT be persisted to `UserSettings`, a URL parameter, or any other storage, so it resets to
off on every page load or navigation.

While Expert Mode is on for a screen, every Food Item or Custom Food shown on that screen SHALL
display its Canonical Name alongside its Display Name. While Expert Mode is off (the default),
only the Display Name SHALL be shown.

#### Scenario: Enabling Expert Mode on the confirmation screen

- **WHEN** a user enables Expert Mode on the meal confirmation screen
- **THEN** every recognized item on that screen SHALL show its Canonical Name alongside its
  Display Name

#### Scenario: Expert Mode does not persist across page loads

- **WHEN** a user enables Expert Mode on a screen and then reloads the page or navigates away and
  back
- **THEN** Expert Mode SHALL be off again on the reloaded/revisited screen

#### Scenario: Expert Mode on one screen does not affect another

- **WHEN** a user enables Expert Mode on the food history detail view
- **THEN** the Custom Food catalog screen and the confirmation screen SHALL each independently
  still have Expert Mode off unless separately enabled

#### Scenario: An item with no recorded Canonical Name shows only its Display Name in Expert Mode

- **WHEN** Expert Mode is enabled on a screen showing a Food Item or Custom Food whose
  `canonical_name` is empty (e.g. it predates this change, or its Display Language was already
  English)
- **THEN** the system SHALL show only its Display Name, without an empty or placeholder
  Canonical Name field
