# mobile-navigation Specification

## Purpose
TBD - created by archiving change mobile-bottom-nav. Update Purpose after archive.
## Requirements
### Requirement: Mobile bottom navigation bar

The frontend SHALL render a bottom navigation bar, fixed to the bottom of the viewport, on every
authenticated page when the viewport width is below the mobile navigation breakpoint. The bar SHALL
offer exactly five destinations in this order: Home (`/`), Photo (`/food/upload/`), Manual
(`/food/manual/`), History (`/food/history/`), and More (the sheet defined below).

The breakpoint SHALL be defined in exactly one place in the frontend and referenced from there; it
SHALL NOT be restated per component. At and above the breakpoint the bar SHALL NOT be visible to
the user, and none of its destinations SHALL be reachable by pointer or by keyboard focus. The bar
MAY remain present in the DOM in a hidden state — the boundary is resolved by CSS rather than by
conditional mounting, so that a statically exported page serves the correct navigation before
hydration.

This is a viewport-width condition, not a pointer-type one. The problem it solves is horizontal:
the header's control set wraps to multiple rows only when the viewport is too narrow to hold it on
one. The bar's own cost is vertical space, which is scarcest exactly when a handheld is held in
landscape and the viewport is already wide enough for the header to fit on one row.

#### Scenario: Bar renders on an authenticated page at a mobile width

- **WHEN** an authenticated user loads any authenticated page at a viewport width below the mobile
  navigation breakpoint
- **THEN** the bottom navigation bar SHALL be visible, fixed to the bottom of the viewport
- **AND** it SHALL offer the Home, Photo, Manual, History and More destinations

#### Scenario: Bar is hidden at desktop widths

- **WHEN** an authenticated user loads any authenticated page at a viewport width at or above the
  mobile navigation breakpoint
- **THEN** the bottom navigation bar SHALL NOT be visible
- **AND** none of its destinations SHALL be focusable by keyboard
- **AND** the visible page content SHALL be laid out as though the bar did not exist, with no
  reserved space at the bottom of the viewport

#### Scenario: Bar is absent from the login page

- **WHEN** an unauthenticated visitor loads `/login` at a viewport width below the breakpoint
- **THEN** no bottom navigation bar element SHALL be present in the document at all — `/login` does
  not receive the authenticated shell, so this is absence, not the hidden state above

#### Scenario: Bar does not flash on an unauthenticated deep link

- **WHEN** an unauthenticated visitor loads an authenticated route directly — `/` or
  `/food/history/` — at a viewport width below the breakpoint, and the client-side session check
  has not yet resolved
- **THEN** the bottom navigation bar SHALL NOT be visible at any point before the redirect to
  `/login`
- **AND** the same SHALL hold for the header, so no authenticated chrome is shown to a visitor who
  turns out to have no session

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

The vertical space the bar occupies — its height plus the device's bottom safe-area inset — SHALL be
defined in exactly one place and read from there by every element that must clear it. Its value
SHALL be zero at and above the breakpoint, so that elements reading it are unaffected on desktop.

The bottom safe-area inset SHALL be absorbed exactly once, by whichever element is bottom-most at
the current viewport width. Below the breakpoint that is the navigation bar; at and above it, where
the bar does not render, it is whatever the page itself anchors to the bottom edge. Neither
double-absorbing the inset nor dropping it above the breakpoint is acceptable — a handheld in
landscape can be both above the breakpoint and a device with a non-zero inset.

Two distinct cases follow, because one mechanism cannot serve both:

- **In-flow content.** The authenticated page shell SHALL reserve clearance equal to that space, so
  no page's content is covered by the bar at its scroll extremity.
- **Fixed-position elements the page itself renders.** Bottom-anchored `position: fixed` elements
  are unaffected by an ancestor's padding, so each SHALL be offset by that same space instead. This
  applies to the submit bars on the manual-entry and meal-review pages and to the app-wide toast
  stack. Raising the navigation bar's stacking order above them SHALL NOT be treated as satisfying
  this requirement: the two elements must not overlap, since a control underneath the bar is
  unusable regardless of which one paints on top.

Full-screen overlays that deliberately cover the whole viewport — the camera capture surface and the
modal dialogs — are outside this requirement. They are expected to cover the bar.

#### Scenario: End of a long scrolled list is fully visible

- **WHEN** an authenticated user scrolls to the bottom of the meal history page at a mobile
  viewport width
- **THEN** the final row SHALL be fully visible above the bar, not covered by it

#### Scenario: A page's own submit bar clears the navigation bar

- **WHEN** an authenticated user loads the manual food entry page at a mobile viewport width, where
  the page renders its own bottom-anchored submit bar
- **THEN** the submit bar's bounding box SHALL NOT intersect the navigation bar's bounding box
- **AND** the submit control SHALL be fully visible and activatable
- **AND** the same SHALL hold on the meal review page, which renders the same submit bar

#### Scenario: A toast clears the navigation bar

- **WHEN** a toast notification is shown at a mobile viewport width on any authenticated page
- **THEN** the toast's bounding box SHALL NOT intersect the navigation bar's bounding box
- **AND** the toast's dismiss control SHALL be fully visible and activatable

#### Scenario: Desktop layout is unaffected by the clearance mechanism

- **WHEN** any authenticated page is rendered at a viewport width at or above the breakpoint
- **THEN** the shell SHALL reserve no bottom clearance
- **AND** the manual-entry and meal-review submit bars SHALL sit flush to the bottom of the
  viewport, and the toast stack at its existing offset, positioned exactly as before this change
- **AND** those submit bars SHALL still absorb the bottom safe-area inset themselves, since no
  navigation bar is beneath them to absorb it

#### Scenario: The document opts into safe-area insets

- **WHEN** any page of the app is served
- **THEN** the document's viewport meta SHALL include `viewport-fit=cover`, without which
  `env(safe-area-inset-bottom)` resolves to zero on every device and the inset handling below is
  inert

#### Scenario: Safe-area inset is respected

- **WHEN** the bar renders on a viewport reporting a non-zero bottom safe-area inset
- **THEN** the bar's interactive controls SHALL sit above that inset rather than beneath it

> Verification note: headless Chromium reports a zero inset and offers no way to set a non-zero one,
> so the scenario above is covered by asserting the `viewport-fit=cover` meta tag is emitted and
> that the bar's computed bottom padding carries the `env()` term, plus a manual check on a notched
> device. This is disclosed in the change's task list rather than claimed as e2e-covered.

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

