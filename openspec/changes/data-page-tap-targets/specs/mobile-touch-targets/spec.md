## MODIFIED Requirements

### Requirement: Minimum Tap Target Size
Every interactive control (button, icon-button, or link acting as a button) in the food entry,
edit, review, history, and custom-foods flows, in the app header/toast dismiss control, on the
`/settings` route (including the Profile form's fields and the relocated Display Language switcher),
and — on the data detail route (`/data/[type]`) — each of the per-record delete control, that
control's inline confirmation step, the zoom range tabs, and the nutrition macro selector tabs,
SHALL have a rendered tap target of at least 48×48 CSS pixels, measured as the element's clickable
bounding box (including padding), not just its visible icon or text glyph.

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

#### Scenario: Data detail zoom range tabs meet the minimum
- **WHEN** the `/data/[type]` route is rendered at a mobile viewport width
- **THEN** each of the Day, Week, Month and Year zoom range tabs has a bounding box of at least
  48×48 pixels

#### Scenario: Data detail nutrition macro tabs meet the minimum
- **WHEN** the `/data/nutrition` route is rendered at a mobile viewport width
- **THEN** each macro selector tab (Calories, Protein, Carbs, Fat, Sugar, Sodium, Fiber) has a
  bounding box of at least 48×48 pixels
