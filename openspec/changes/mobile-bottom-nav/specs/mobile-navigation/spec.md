## ADDED Requirements

### Requirement: Mobile bottom navigation bar

The frontend SHALL render a bottom navigation bar, fixed to the bottom of the viewport, on every
authenticated page when the viewport width is below the mobile navigation breakpoint. The bar SHALL
offer exactly five destinations in this order: Home (`/`), Photo (`/food/upload/`), Manual
(`/food/manual/`), History (`/food/history/`), and More (the sheet defined below).

The breakpoint SHALL be defined in exactly one place in the frontend and referenced from there; it
SHALL NOT be restated per component. At and above the breakpoint the bar SHALL NOT render.

This is a viewport-width condition, not a pointer-type one. The problem it solves is horizontal:
the header's control set wraps to multiple rows only when the viewport is too narrow to hold it on
one. The bar's own cost is vertical space, which is scarcest exactly when a handheld is held in
landscape and the viewport is already wide enough for the header to fit on one row.

#### Scenario: Bar renders on an authenticated page at a mobile width

- **WHEN** an authenticated user loads any authenticated page at a viewport width below the mobile
  navigation breakpoint
- **THEN** the bottom navigation bar SHALL be visible, fixed to the bottom of the viewport
- **AND** it SHALL offer the Home, Photo, Manual, History and More destinations

#### Scenario: Bar is absent at desktop widths

- **WHEN** an authenticated user loads any authenticated page at a viewport width at or above the
  mobile navigation breakpoint
- **THEN** the bottom navigation bar SHALL NOT be rendered

#### Scenario: Bar is absent when unauthenticated

- **WHEN** an unauthenticated visitor loads `/login` at a viewport width below the breakpoint
- **THEN** the bottom navigation bar SHALL NOT be rendered

#### Scenario: Each destination navigates

- **WHEN** a user activates one of the five destinations
- **THEN** the app SHALL navigate to that destination's route, or in the case of More SHALL open
  the More sheet without navigating

### Requirement: Logging actions are reachable without scrolling

Both food-logging entry points — photo capture and manual entry — SHALL be reachable from every
authenticated page, at a mobile viewport width, without scrolling. This is the measurable outcome
the bar exists for: before this change the photo action existed only in the dashboard body, below
the fold, and on no other screen.

#### Scenario: Photo capture reachable without scrolling from a data page

- **WHEN** an authenticated user loads `/data/steps` at a 390×844 viewport and does not scroll
- **THEN** a control that navigates to photo capture SHALL be within the visible viewport

#### Scenario: Manual entry reachable without scrolling from the dashboard

- **WHEN** an authenticated user loads the dashboard at a 390×844 viewport and does not scroll
- **THEN** a control that navigates to manual entry SHALL be within the visible viewport

### Requirement: Active destination is indicated

The bar SHALL indicate which destination corresponds to the current route, so the user's location
is legible without reading the page content. Exactly one destination SHALL be indicated as active
at a time, and no destination SHALL be indicated when the current route matches none of them.

The indication SHALL NOT be carried by color alone.

#### Scenario: Current route's destination is active

- **WHEN** an authenticated user is on `/food/history/` at a mobile viewport width
- **THEN** the History destination SHALL be indicated as active
- **AND** no other destination SHALL be indicated as active

#### Scenario: No destination active on an unlisted route

- **WHEN** an authenticated user is on `/data/steps`, which is not one of the five destinations
- **THEN** no destination SHALL be indicated as active

### Requirement: Bar does not occlude page content

The bar is fixed above the page, so the authenticated page shell SHALL reserve clearance equal to
the bar's height plus the device's bottom safe-area inset. No page's content, including the last
row of a scrolled list and any fixed-position element the page itself renders, SHALL be covered by
the bar at its scroll extremity.

#### Scenario: End of a long scrolled list is fully visible

- **WHEN** an authenticated user scrolls to the bottom of the meal history page at a mobile
  viewport width
- **THEN** the final row SHALL be fully visible above the bar, not covered by it

#### Scenario: Safe-area inset is respected

- **WHEN** the bar renders on a viewport reporting a non-zero bottom safe-area inset
- **THEN** the bar's interactive controls SHALL sit above that inset rather than beneath it

### Requirement: More sheet carries the header's remaining controls

Activating More SHALL open a sheet containing the controls the mobile header no longer carries:
the webhook URL (with its copy action), custom foods, import, settings, and logout. Every control
SHALL behave identically to its header counterpart.

The sheet SHALL be dismissible without selecting a control, and dismissing it SHALL NOT navigate.

#### Scenario: Sheet exposes every shed control

- **WHEN** a user opens the More sheet at a mobile viewport width
- **THEN** the sheet SHALL offer the webhook URL with its copy action, custom foods, import,
  settings, and logout

#### Scenario: Logout from the sheet

- **WHEN** a user activates logout in the More sheet
- **THEN** the session SHALL end and the app SHALL navigate to `/login`, identically to the
  header's logout control

#### Scenario: Sheet is dismissible

- **WHEN** a user dismisses the More sheet without activating any control
- **THEN** the sheet SHALL close
- **AND** the current route SHALL be unchanged

#### Scenario: No control is stranded on mobile

- **WHEN** the header's control set is reduced at a mobile viewport width
- **THEN** every control it sheds SHALL be reachable through the More sheet, so that no
  functionality available on desktop is unreachable on mobile
