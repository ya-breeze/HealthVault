# dashboard-ui Specification

## Purpose
TBD - created by archiving change dashboard-instrument-panel. Update Purpose after archive.
## Requirements
### Requirement: Shared instrument-panel header
The system SHALL render a single shared header/navigation component on every authenticated page — the dashboard, every data page, the food logging flow, and the import page. Only one header implementation SHALL exist in the frontend; pages SHALL NOT each render their own copy.

The header's control set is conditional on viewport width, governed by the same single breakpoint definition as the mobile navigation bar:

- **At and above the breakpoint**, the header SHALL carry app name, active user, webhook link, custom foods link, import link, settings link, and logout — its full set, with its existing wrapping behaviour unchanged.
- **Below the breakpoint**, the header SHALL carry only the app name and the active user, and SHALL occupy a single row down to a 320px viewport width. The remaining controls SHALL be reachable through the mobile navigation shell instead, which is responsible for leaving none of them stranded.

The single-row guarantee is scoped to below the breakpoint deliberately. The desktop header's
wrapping is a pre-existing behaviour of the same shared container, and this change does not alter
it: between the breakpoint and roughly 768px the full seven-control set can still wrap to a second
row, more readily with Russian labels. That is out of scope here — narrowing it would be a desktop
change, and this change's premise is that desktop rendering is untouched. It is recorded rather
than silently left, so a later change can take it up.

Reducing the mobile control set is what stops the header consuming 21% of the fold on a 390px viewport; it is not a removal of functionality, because the navigation shell's own requirements mandate that every shed control remains reachable.

Note on the desktop enumeration: it now names the **settings link**, which the previous wording
omitted although `Header.tsx` has rendered it since the Display Language control was relocated to
`/settings`. That is a correction to a stale enumeration, not a control this change adds — the
"Desktop header is unchanged" scenario below still holds literally.

#### Scenario: Header present on the dashboard
- **WHEN** an authenticated user loads the dashboard
- **THEN** the shared header SHALL be visible with the active user, and — at a desktop viewport width — the webhook, custom foods, import, settings and logout controls

#### Scenario: Header present on a data page
- **WHEN** an authenticated user loads `/data/steps`
- **THEN** the same shared header component SHALL be visible, unchanged in structure from the dashboard's header at the same viewport width

#### Scenario: Header present on food logging and import pages
- **WHEN** an authenticated user loads the photo upload, manual entry, or import page
- **THEN** the shared header SHALL be visible, even though the page content below it is unchanged by this capability

#### Scenario: Mobile header is reduced to title and user
- **WHEN** an authenticated user loads any authenticated page at a viewport width below the breakpoint
- **THEN** the header SHALL show the app name and the active user
- **AND** it SHALL NOT render the webhook, custom foods, import, settings or logout controls
- **AND** it SHALL occupy a single row

#### Scenario: Desktop header is unchanged
- **WHEN** an authenticated user loads any authenticated page at a viewport width at or above the breakpoint
- **THEN** the header SHALL carry its full control set exactly as before this change

### Requirement: Dashboard vitals grid

The dashboard SHALL replace the 3-card summary and separate "Browse Data" grid with a single vitals grid covering the 8 primary metrics: `steps`, `heart_rate`, `sleep`, `heart_rate_variability`, `distance`, `weight`, `blood_pressure`, `oxygen_saturation`. Each card SHALL show the metric's current value, a 7-day sparkline, and a trend indicator, and SHALL link to that metric's data page. Which of those 8 cards actually render is governed by the "Customizable vitals grid order and visibility" requirement and the "Presence-based visibility of vitals grid cards" requirement.

#### Scenario: Vitals grid renders all 8 primary metrics

- **WHEN** an authenticated user with data in all 8 primary metrics, who has hidden none of them, loads the dashboard
- **THEN** the system SHALL render 8 vital cards, one per metric, each showing a current value and a sparkline

#### Scenario: Vital card links to its data page

- **WHEN** a user clicks a vital card
- **THEN** the system SHALL navigate to that metric's `/data/{type}` page

#### Scenario: Missing data for a metric

- **WHEN** the resolved user has at least one record ever for one of the 8 primary metrics, but none in the last 7 days
- **THEN** that metric's card SHALL still render and SHALL indicate no data rather than rendering an empty or broken sparkline — presence-based hiding (see "Presence-based visibility of vitals grid cards") applies only to metrics with zero records ever, not to a metric whose only gap is the last 7 days

### Requirement: Secondary data types remain reachable

Registered types outside the 8-metric vitals grid SHALL remain reachable from the dashboard as a compact link list, for every such type the resolved user has ever recorded at least one row of, without giving every type a full vital card. Which secondary types actually appear in that list is governed by the "Presence-based filtering of the More Data section" requirement — this requirement no longer guarantees full-catalog access to types the user has never recorded.

#### Scenario: Secondary type link navigates correctly

- **WHEN** a user clicks a secondary type's link on the dashboard
- **THEN** the system SHALL navigate to that type's `/data/{type}` page, identically to a vitals-grid card

### Requirement: Consistent per-metric color
Each registered data type SHALL be assigned exactly one accent color, used consistently for that type's dashboard card, sparkline, and data-page chart. The mapping SHALL be defined once and imported wherever a metric is rendered, not duplicated per page.

#### Scenario: Same color on dashboard and data page
- **WHEN** a user views a metric's color on its dashboard vital card and then opens that metric's data page
- **THEN** the accent color SHALL be identical in both places

### Requirement: Icon set replaces emoji
User-facing action controls (photo capture, manual entry, and other iconized actions previously rendered as emoji) SHALL use the app's inline-SVG icon set instead of emoji characters.

#### Scenario: Food logging actions use SVG icons
- **WHEN** a user views the "Log food" actions on the dashboard
- **THEN** the photo and manual-entry actions SHALL render inline SVG icons, not emoji glyphs

### Requirement: Needs-attention indicator
The dashboard SHALL show an indicator when the authenticated user has one or more logged meals needing attention (as counted by `GET /api/food/meals/needs-attention-count`), stating the count and linking to the meal history page (`/food/history/`). The indicator SHALL NOT be rendered when the count is zero.

#### Scenario: Indicator shown when meals need attention
- **GIVEN** the resolved user has 2 meals needing attention
- **WHEN** they load the dashboard
- **THEN** an indicator is visible stating that 2 meals need attention

#### Scenario: Indicator hidden when nothing needs attention
- **GIVEN** the resolved user has no meals needing attention (none logged, or all confirmed)
- **WHEN** they load the dashboard
- **THEN** no needs-attention indicator is rendered

#### Scenario: Indicator links to meal history
- **WHEN** a user clicks the needs-attention indicator
- **THEN** the system navigates to `/food/history/`

### Requirement: Customizable vitals grid order and visibility

The dashboard vitals grid SHALL support a per-user, persisted custom display order AND per-card visibility for its cards, scoped to primary metrics that have presence (per "Presence-based visibility of vitals grid cards" — a metric with zero records ever is excluded from this customizable set entirely, not defaulted into it as hidden). Users SHALL enter an explicit Edit/Customize mode to change order or visibility; outside that mode, the grid SHALL render only the visible, presence-eligible cards, in the user's saved order (or the default order, all visible, if none is saved), with no visible reorder or visibility controls.

#### Scenario: Default order for a user who has not customized

- **WHEN** a user who has never saved a custom order/visibility loads the dashboard
- **THEN** the vitals grid SHALL render all presence-eligible metrics in the application's default order, none hidden

#### Scenario: Entering edit mode reveals reorder controls

- **WHEN** a user clicks the dashboard's Edit/Customize control
- **THEN** each vitals-grid card for a presence-eligible metric SHALL show move-up, move-down, and a show/hide control, the move-up control SHALL be disabled on the first card, the move-down control SHALL be disabled on the last card, and cards currently hidden SHALL still be shown (visually distinguished as hidden) so they can be found and re-shown; metrics with zero presence SHALL NOT appear in this list at all

#### Scenario: Hiding a card

- **WHEN** a user in edit mode toggles a visible card's show/hide control to hidden, and clicks Done
- **THEN** the system SHALL persist that card as hidden while keeping its position in the saved order, and the read-only grid SHALL no longer render that card

#### Scenario: Re-showing a hidden card restores its position

- **WHEN** a user in edit mode toggles a previously-hidden card back to visible, and clicks Done
- **THEN** the read-only grid SHALL render that card again in the same position it held before it was hidden, not appended at the end

#### Scenario: Hiding every card shows a placeholder

- **WHEN** a user hides every presence-eligible card in the vitals grid and clicks Done
- **THEN** the system SHALL persist the all-hidden state without error, and the read-only grid SHALL render the `vitals-grid-empty` placeholder message instead of an empty grid — this scenario presumes at least one metric has presence; a user with zero presence-eligible metrics instead sees the distinct placeholder described in "Distinct empty state for a vitals grid with no recorded data"

#### Scenario: Reordering and saving

- **WHEN** a user in edit mode moves a card and then clicks Done
- **THEN** the system SHALL persist the new order (and each card's visibility) for that user and exit edit mode, and the grid SHALL immediately reflect the new order and visibility

#### Scenario: Saved order persists across sessions

- **WHEN** a user who previously saved a custom order/visibility loads the dashboard again (including from a different browser/device)
- **THEN** the vitals grid SHALL render in their saved order, showing only their visible, presence-eligible cards

#### Scenario: Saved order tolerates the default metric set changing

- **WHEN** a user's saved order references a metric no longer in the default set, or the default set has gained a metric absent from their saved order
- **THEN** the system SHALL render only currently-valid, presence-eligible metrics in the user's saved relative order and visibility, and SHALL append any presence-eligible metric missing from their saved order at the end as visible, without erroring; a metric referenced in the saved order but with zero presence SHALL NOT be rendered or appended regardless of its saved visibility

#### Scenario: Hidden cards are never revealed before the saved settings load

- **WHEN** a user's saved order and visibility have not yet loaded, or failed to load
- **THEN** the dashboard SHALL render a loading or error placeholder in place of the vitals grid, and SHALL NOT render any card until the saved visibility is known, so that cards the user hid are never briefly (or, on failure, indefinitely) shown

#### Scenario: A failed settings load is recoverable without a page reload

- **WHEN** the saved order and visibility failed to load and the user activates the placeholder's retry control
- **THEN** the system SHALL re-attempt the load, and on success SHALL render the vitals grid in the user's saved order and visibility with the customize control re-enabled

#### Scenario: Saved order in the pre-visibility shape still loads

- **WHEN** a user's saved settings still hold the earlier plain list-of-types shape (no per-entry visibility) from before this capability existed
- **THEN** the system SHALL treat every entry in that saved order as visible and render it exactly as it did before, without erroring

### Requirement: Presence-based visibility of vitals grid cards

The vitals grid SHALL only ever render or offer, in both the read-only view and edit/Customize mode, a primary metric for which the resolved user has at least one recorded data point of any age, as reported by `GET /api/data-types/presence`. A metric with zero presence SHALL NOT render as a card, SHALL NOT appear in edit mode's reorder/show-hide list, and SHALL NOT count toward the "all cards hidden" placeholder — even if the user's saved order references it.

If the presence fetch fails, the system SHALL fail open: treat every primary metric as present (subject only to the user's existing hide/show preferences) rather than hiding any of them because of the failure. A metric absent from an otherwise-successful presence response SHALL be treated the same as a fetch failure for that metric — shown, not hidden.

#### Scenario: Metric with no data ever is hidden

- **WHEN** the resolved user has zero records ever for one of the 8 primary metrics
- **THEN** that metric's card SHALL NOT render in the read-only vitals grid

#### Scenario: Metric with data outside the 7-day window is still shown

- **WHEN** the resolved user has at least one record ever for a primary metric, but none within the last 7 days
- **THEN** that metric's card SHALL still render, showing the existing "no data" sparkline placeholder rather than being hidden

#### Scenario: Zero-presence metric excluded from edit mode

- **WHEN** a user with zero records for a primary metric enters edit/Customize mode
- **THEN** that metric SHALL NOT appear in the reorder/show-hide list

#### Scenario: Presence fetch failure shows all metrics

- **WHEN** the presence fetch fails
- **THEN** the system SHALL render every primary metric (subject to the user's existing hide/show preferences) as if every metric had presence

#### Scenario: Metric missing from a successful presence response is shown

- **WHEN** the presence fetch succeeds but its response omits an entry for one primary metric
- **THEN** the system SHALL treat that metric as present and render it, not treat the omission as `false`

#### Scenario: No card is shown before presence is known

- **WHEN** the presence fetch has not yet resolved (and has not failed)
- **THEN** the dashboard SHALL NOT render any vitals-grid card as filtered-in by presence until the fetch resolves or fails, consistent with the existing settings-load gating in "Customizable vitals grid order and visibility" — cards SHALL wait for both the saved settings load and the presence fetch before rendering

### Requirement: Presence-based filtering of the More Data section

The More Data section SHALL list only secondary (non-primary) registered types for which the resolved user has at least one recorded data point of any age, per `GET /api/data-types/presence` and the same fail-open rules as the vitals grid (a fetch failure, or a type missing from a successful response, means that type is shown). When zero secondary types have presence, the More Data section SHALL NOT render at all — no heading, no empty list. The section's container SHALL carry `data-testid="more-data"`. This presence-based filtering is independent of, and evaluated before, the user's own hide/show preference for the section as a whole (see "User-hidden More Data section") — a user-hidden section SHALL NOT render regardless of what presence filtering alone would have produced, including during a presence-fetch failure. In the read-only view, the section additionally SHALL NOT render until the user's saved hide/show preference has loaded, so that a user-hidden section is never briefly shown before that preference is known.

#### Scenario: Secondary type with data appears in More Data

- **WHEN** the resolved user has at least one record ever for a secondary type, and has not hidden the More Data section
- **THEN** that type's link SHALL appear in the More Data section

#### Scenario: Secondary type with no data ever is omitted

- **WHEN** the resolved user has zero records ever for a secondary type
- **THEN** that type's link SHALL NOT appear in the More Data section

#### Scenario: More Data section absent when nothing qualifies

- **WHEN** the resolved user has zero records ever for every secondary type
- **THEN** the dashboard SHALL NOT render the More Data section (no `more-data` testid element present)

#### Scenario: Presence fetch failure shows all secondary types

- **WHEN** the presence fetch fails, and the user has not hidden the More Data section
- **THEN** the system SHALL render every secondary type in the More Data section, as if every type had presence

#### Scenario: More Data section waits for presence before rendering

- **WHEN** the presence fetch has not yet resolved (and has not failed)
- **THEN** the dashboard SHALL NOT render the More Data section as filtered-in by presence until the fetch resolves or fails

#### Scenario: More Data section waits for the saved hide/show preference before rendering

- **WHEN** the user's saved hide/show preference has not yet loaded (or failed to load)
- **THEN** the dashboard SHALL NOT render the More Data section as filtered-in until the saved preference has resolved

### Requirement: Distinct empty state for a vitals grid with no recorded data

When presence filtering (not user hide/show) leaves zero eligible primary metrics — the resolved user has never recorded any of the 8 primary metrics — the read-only vitals grid SHALL render a placeholder with `data-testid="vitals-grid-empty-no-data"`, with copy that guides the user toward recording data rather than toward Customize. This placeholder is distinct from the existing `vitals-grid-empty` placeholder (which covers a user who has hidden every presence-eligible card via Customize): the two SHALL use different testids and different copy, and SHALL NOT both render at once.

#### Scenario: No data ever recorded shows the no-data placeholder

- **WHEN** the resolved user has zero records ever for all 8 primary metrics
- **THEN** the vitals grid SHALL render the `vitals-grid-empty-no-data` placeholder, not the `vitals-grid-empty` placeholder

#### Scenario: Data exists but every eligible card is user-hidden

- **WHEN** the resolved user has recorded at least one primary metric, but has hidden every presence-eligible card via Customize
- **THEN** the vitals grid SHALL render the existing `vitals-grid-empty` placeholder, not `vitals-grid-empty-no-data`

### Requirement: User-hidden More Data section

The dashboard SHALL support a per-user, persisted preference to hide the entire More Data section, independent of presence filtering. Users SHALL toggle this preference from a show/hide control on the More Data section's heading, visible only in the dashboard's existing Edit/Customize mode (the same mode used for vitals-grid card order and visibility), and SHALL persist it via the existing Done action. The control SHALL only be offered when the section would otherwise render (i.e. at least one secondary type currently has presence); when nothing has presence, there is nothing to hide and no control SHALL be shown. Once hidden, the section SHALL NOT render in the read-only view under any presence outcome, including a presence-fetch failure that would otherwise show every secondary type.

#### Scenario: Entering edit mode reveals the More Data hide toggle

- **GIVEN** the resolved user has at least one secondary type with presence
- **WHEN** a user clicks the dashboard's Edit/Customize control
- **THEN** the More Data section heading SHALL show a show/hide control

#### Scenario: No hide toggle when nothing has presence

- **GIVEN** the resolved user has zero secondary types with presence
- **WHEN** a user clicks the dashboard's Edit/Customize control
- **THEN** the More Data section, and its show/hide control, SHALL NOT render

#### Scenario: Hiding the More Data section

- **WHEN** a user in edit mode toggles the More Data section's show/hide control to hidden, and clicks Done
- **THEN** the system SHALL persist the section as hidden, and the read-only view SHALL no longer render the More Data section

#### Scenario: Re-showing the More Data section

- **WHEN** a user in edit mode toggles a previously-hidden More Data section back to visible, and clicks Done
- **THEN** the read-only view SHALL render the More Data section again, subject to presence filtering as normal

#### Scenario: Hidden section stays hidden in edit mode, but discoverable

- **GIVEN** the resolved user has hidden the More Data section and at least one secondary type has presence
- **WHEN** the user enters edit mode
- **THEN** the More Data section (heading, toggle, and link list) SHALL still render, visually distinguished as hidden, so it can be found and re-shown

#### Scenario: User-hidden section overrides presence fail-open

- **GIVEN** the resolved user has hidden the More Data section
- **WHEN** the presence fetch fails
- **THEN** the More Data section SHALL still NOT render in the read-only view, even though a presence-fetch failure would otherwise show every secondary type

#### Scenario: Hidden preference persists across sessions

- **WHEN** a user who previously hid the More Data section loads the dashboard again (including from a different browser/device)
- **THEN** the More Data section SHALL remain hidden in the read-only view, subject to presence filtering as normal once re-shown

#### Scenario: Hidden preference survives presence regressing to zero

- **GIVEN** the resolved user has hidden the More Data section
- **WHEN** every secondary type's presence subsequently regresses to zero (e.g. via record deletion), causing the section and its toggle to no longer render in edit mode
- **THEN** the stored hide/show preference SHALL remain unchanged, and the More Data section SHALL reappear as hidden once presence for any secondary type is regained

