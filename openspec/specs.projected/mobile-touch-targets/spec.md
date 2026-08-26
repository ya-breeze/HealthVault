<!-- GENERATED FILE — DO NOT EDIT.
     Regenerate with: make projected-specs
     See openspec/specs.projected.README.md for details. -->

# mobile-touch-targets Specification

## Purpose
TBD - created by archiving change mobile-tap-targets. Update Purpose after archive.
## Requirements
### Requirement: Minimum Tap Target Size
Every interactive control (button, icon-button, or link acting as a button) in the food entry,
edit, review, history, and custom-foods flows, in the app header/toast dismiss control, on the
`/settings` route (including the Profile form's fields and the relocated Display Language switcher),
and on the data detail route (`/data/[type]`, including its per-record delete control, that
control's inline confirmation step, and the zoom range tabs), SHALL have a rendered tap target of at
least 48×48 CSS pixels, measured as the element's clickable bounding box (including padding), not
just its visible icon or text glyph.

#### Scenario: Delete control on a meal item meets the minimum
- **WHEN** the meal review page is rendered at a mobile viewport width
- **THEN** the delete control on a meal item row has a bounding box of at least 48×48 pixels

#### Scenario: Food search-result picker meets the minimum
- **WHEN** a food search result list is rendered at a mobile viewport width
- **THEN** each selectable result row/button has a bounding box of at least 48 pixels in height

#### Scenario: Header and toast controls meet the minimum
- **WHEN** the app header or a toast notification with a dismiss control is rendered at a mobile viewport width
- **THEN** the dismiss/nav controls have a bounding box of at least 48×48 pixels

#### Scenario: Settings page controls meet the minimum
- **WHEN** the `/settings` route is rendered at a mobile viewport width
- **THEN** the profile form's inputs/selects and the Display Language switcher each have a bounding
  box of at least 48×48 pixels

#### Scenario: Data detail record delete control meets the minimum
- **WHEN** the `/data/[type]` route is rendered at a mobile viewport width for a type that has records
- **THEN** each record row's delete control has a bounding box of at least 48×48 pixels

#### Scenario: Data detail delete confirmation controls meet the minimum
- **WHEN** the delete control on a record row of the `/data/[type]` route has been activated at a
  mobile viewport width, placing that row in its inline confirmation state
- **THEN** the confirm and cancel controls of that confirmation each have a bounding box of at least
  48×48 pixels

#### Scenario: Data detail zoom range tabs meet the minimum
- **WHEN** the `/data/[type]` route is rendered at a mobile viewport width
- **THEN** each of the Day, Week, Month and Year zoom range tabs has a bounding box of at least
  48×48 pixels

### Requirement: Shared Tap Target Enforcement Component
The frontend SHALL provide a shared component that enforces the 48×48 pixel minimum tap target for any interactive control that uses it, so that meeting the minimum does not depend on each call site remembering to size itself correctly. The component SHALL pass through all native button/anchor attributes (including but not limited to `title`, `aria-label`, `data-testid`, `disabled`, and event handlers) unchanged, so that migrating an existing control to the shared component does not alter how that control is identified by other code (e.g. by existing end-to-end tests).

#### Scenario: New control using the shared component meets the minimum without manual sizing
- **WHEN** a button or icon-button is implemented using the shared tap-target component with no additional sizing classes
- **THEN** its rendered bounding box is at least 48×48 pixels

#### Scenario: Migrating an existing control preserves its identifying attributes
- **WHEN** an existing button with a `title`, `aria-label`, or `data-testid` attribute is migrated to the shared tap-target component
- **THEN** the rendered element still carries that same attribute unchanged

