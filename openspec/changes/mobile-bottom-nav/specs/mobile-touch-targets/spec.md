## MODIFIED Requirements

### Requirement: Minimum Tap Target Size
Every interactive control (button, icon-button, or link acting as a button) in the food entry,
edit, review, history, and custom-foods flows, in the app header/toast dismiss control, in the
mobile bottom navigation bar and its More sheet, on the
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

The bottom navigation bar's five destinations take no such exemption: the bar renders only below
the mobile navigation breakpoint, where the minimum applies unconditionally. Its narrowest case is
the binding one — at a 320px viewport, five destinations divide the width into 64px columns, which
clears the minimum's width but leaves the labels, not the tap targets, as the constraint.

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

#### Scenario: Bottom navigation destinations meet the minimum
- **WHEN** the bottom navigation bar is rendered at any viewport width it renders at, down to 320px
- **THEN** each of the five destinations has a bounding box of at least 48×48 pixels

#### Scenario: More sheet controls meet the minimum
- **WHEN** the More sheet is open at a mobile viewport width
- **THEN** each of its controls — webhook copy, custom foods, import, settings, logout — has a
  bounding box of at least 48×48 pixels

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
