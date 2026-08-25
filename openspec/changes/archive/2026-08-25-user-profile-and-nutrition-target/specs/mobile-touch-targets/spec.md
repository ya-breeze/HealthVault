## MODIFIED Requirements

### Requirement: Minimum Tap Target Size
Every interactive control (button, icon-button, or link acting as a button) in the food entry,
edit, review, history, and custom-foods flows, in the app header/toast dismiss control, and on the
`/settings` route (including the Profile form's fields and the relocated Display Language switcher),
SHALL have a rendered tap target of at least 48×48 CSS pixels, measured as the element's clickable
bounding box (including padding), not just its visible icon or text glyph.

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
