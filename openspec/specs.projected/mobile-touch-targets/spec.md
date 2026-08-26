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
and — on the data detail route (`/data/[type]`) — the per-record delete control together with that
control's inline confirmation step, SHALL have a rendered tap target of at least 48×48 CSS pixels,
measured as the element's clickable bounding box (including padding), not just its visible icon or
text glyph.

The data detail route's zoom range tabs and nutrition macro selector tabs are compact-by-design
segmented controls. They SHALL meet the same 48×48 minimum wherever the primary pointing device is
coarse (a touch screen), and MAY render smaller where the primary pointing device is fine (a mouse).
This is a pointer-type condition, not a viewport-width one: a phone held in landscape is wider than
a typical mobile breakpoint and must still get the minimum.

The data detail route's list above is exhaustive: the record-entry form's text and number inputs on
that route are outside this requirement's scope.

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

#### Scenario: Data detail zoom range tabs meet the minimum on a touch pointer
- **WHEN** the `/data/[type]` route is rendered on a device whose primary pointing device is coarse
- **THEN** each of the Day, Week, Month and Year zoom range tabs has a bounding box of at least
  48×48 pixels, at any viewport width

#### Scenario: Data detail nutrition macro tabs meet the minimum on a touch pointer
- **WHEN** the `/data/nutrition` route is rendered on a device whose primary pointing device is coarse
- **THEN** each macro selector tab (Calories, Protein, Carbs, Fat, Sugar, Sodium, Fiber) has a
  bounding box of at least 48×48 pixels, at any viewport width

#### Scenario: Data detail delete control keeps the minimum on a mouse pointer
- **WHEN** the `/data/[type]` route is rendered on a device whose primary pointing device is fine
- **THEN** each record row's delete control still has a bounding box of at least 48×48 pixels
- **AND** the zoom range tabs may render smaller than the minimum

### Requirement: Shared Tap Target Enforcement Component
The frontend SHALL provide a shared component that enforces the 48×48 pixel minimum tap target for any interactive control that uses it, so that meeting the minimum does not depend on each call site remembering to size itself correctly. The component SHALL pass through all native button/anchor attributes (including but not limited to `title`, `aria-label`, `data-testid`, `disabled`, and event handlers) unchanged, so that migrating an existing control to the shared component does not alter how that control is identified by other code (e.g. by existing end-to-end tests).

The component SHALL offer exactly one named opt-out, for controls that are compact by design: a call site MAY declare that the control renders at its natural size where the primary pointing device is fine, in which case the minimum still applies on every other pointer type, including a device that reports no pointer at all. This is the only route by which a control using the shared component may render below the minimum, and it SHALL be requested by name — the component remains the single place where tap-target dimensions are expressed, and a call site SHALL NOT restate its own dimensions to obtain the smaller rendering.

#### Scenario: New control using the shared component meets the minimum without manual sizing
- **WHEN** a button or icon-button is implemented using the shared tap-target component with no additional sizing classes and without the compact-on-fine-pointer opt-out
- **THEN** its rendered bounding box is at least 48×48 pixels

#### Scenario: A control using the opt-out keeps the minimum off a fine pointer
- **WHEN** a control implemented using the shared tap-target component declares the compact-on-fine-pointer opt-out
- **THEN** its rendered bounding box is at least 48×48 pixels on a coarse pointer
- **AND** it may render smaller only where the primary pointing device is fine

#### Scenario: Migrating an existing control preserves its identifying attributes
- **WHEN** an existing button with a `title`, `aria-label`, or `data-testid` attribute is migrated to the shared tap-target component
- **THEN** the rendered element still carries that same attribute unchanged

