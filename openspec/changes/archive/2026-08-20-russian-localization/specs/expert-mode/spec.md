## ADDED Requirements

### Requirement: Per-Screen Expert Mode Toggle

The frontend SHALL provide an Expert Mode toggle, independently available on every screen that
displays Food Item or Custom Food names: the meal recognition/confirmation screen and the Custom
Food catalog screen. The food history list is deliberately not one of them — it shows meal names
and daily totals, never an individual Food Item's name, and opening a meal from it navigates to the
recognition/confirmation screen, which carries the toggle. Expert Mode state SHALL be held only in
client-side UI state for the current screen — it
SHALL NOT be persisted to `UserSettings`, a URL parameter, or any other storage, so it resets to
off on every page load or navigation.

While Expert Mode is on for a screen, every Food Item or Custom Food shown on that screen SHALL
display its Canonical Name alongside its Display Name. While Expert Mode is off (the default),
only the Display Name SHALL be shown. "Shown on that screen" includes names rendered in transient
panels the screen opens, not only its persistent list — in particular the item-resolution panel's
candidate list, which lists the user's own Custom Foods.

This toggle is distinct from the `Expert` authoring mode of the reanalysis correction interface
(see `food-photo-recognition` "Expert Component Guidance for Reanalysis"), which sits on the same
screen: that one changes what is submitted for analysis, this one only changes what an already-
analyzed screen displays. Because both are visible at once on the review screen, the toggle's
visible label SHALL state what it reveals rather than reading as an unqualified "Expert" control.

#### Scenario: Enabling Expert Mode on the confirmation screen

- **WHEN** a user enables Expert Mode on the meal confirmation screen
- **THEN** every recognized item on that screen SHALL show its Canonical Name alongside its
  Display Name

#### Scenario: Expert Mode applies to the item-resolution panel's candidates

- **WHEN** Expert Mode is on for the meal review screen, and a user opens an item's resolution panel
  and searches, and one of the returned candidates is one of their own Custom Foods with a recorded
  Canonical Name
- **THEN** that candidate SHALL show its Canonical Name alongside its Display Name, the same as the
  meal's own items on that screen

#### Scenario: Expert Mode does not persist across page loads

- **WHEN** a user enables Expert Mode on a screen and then reloads the page or navigates away and
  back
- **THEN** Expert Mode SHALL be off again on the reloaded/revisited screen

#### Scenario: Expert Mode on one screen does not affect another

- **WHEN** a user enables Expert Mode on the Custom Food catalog screen
- **THEN** the meal recognition/confirmation screen SHALL still have Expert Mode off unless
  separately enabled there

#### Scenario: An item with no recorded Canonical Name shows only its Display Name in Expert Mode

- **WHEN** Expert Mode is enabled on a screen showing a Food Item or Custom Food whose
  `canonical_name` is empty (e.g. it predates this change, or its Display Language was already
  English)
- **THEN** the system SHALL show only its Display Name, without an empty or placeholder
  Canonical Name field
