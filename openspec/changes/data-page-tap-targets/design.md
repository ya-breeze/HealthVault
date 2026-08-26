## Context

`TapTarget` (`frontend/components/ui/TapTarget.tsx`) already exists and enforces the 48×48 minimum
by prepending `min-h-12 min-w-12` to whatever `className` a call site passes, spreading every other
prop through unchanged. The food flows, header/toast and `/settings` were migrated to it by the
archived `mobile-tap-targets` change.

`/data/[type]` was outside that change's scope and is only *partly* migrated: `DataTypeClient.tsx`
already imports `TapTarget` and uses it for the "Set goal" (line 583) and "Set height" (line 607)
controls, and the `AddRecordForm` rendered on the same route uses it for its own buttons. What was
never covered is the table's per-record controls, the zoom range tabs, and the nutrition macro tabs
— all still bare `<button>` elements.

Measured on `hcw-wip` at 375×667, the uncovered controls are: delete **14×20**, zoom tabs **28**
tall, macro tabs **27** tall.

Two of these sit inside layouts that constrain them, so "make them 48px" is not purely additive:

- The delete control lives in a table cell whose row height is currently driven by the timestamp
  text (65–85 px depending on data type).
- The zoom tabs are a compact segmented control shared by every data type, rendered identically on
  desktop where 28 px is a deliberate, unremarkable size.

## Goals / Non-Goals

**Goals:**
- Every control named in the spec delta reaches 48×48 at mobile widths.
- The enforcement comes from `TapTarget`, not from per-call-site sizing classes, so the fix cannot
  silently regress the way the original did.
- An e2e case exists that fails against today's code and is deterministic.

**Non-Goals:**
- The table's layout, column set, or nested horizontal scroll. The columns stay exactly as they are.
- The record-entry form's inputs on this route (`AddRecordForm`, ~34 px tall). Form-field sizing is
  a separate concern from button geometry and would pull in a wider pass; the spec delta names this
  exclusion explicitly rather than leaving it ambiguous.
- The mobile visual redesign (bottom tab bar, records-as-rows). That is a separate change with its
  own proposal; deliberately not pre-empted here.
- Any change to delete *semantics* — the two-step confirmation stays exactly as it is, only the
  geometry of its controls changes.

## Decisions

**Migrate to `TapTarget` rather than adding utility classes.** The original defect exists precisely
because sizing was left to each call site. Routing these controls through the shared component makes
the minimum structural. `TapTarget` spreads `aria-label`, `data-testid`, `disabled` and handlers
through unchanged, so e2e locators that address the delete control by `aria-label="Delete record"`
keep working — the archived change already relied on that guarantee.

*Alternative considered:* add `min-h-12 min-w-12` inline at each call site. Rejected — identical
rendered result, but it re-creates the per-call-site pattern that produced the gap.

**Grow the zoom tabs at every width, not only mobile.** A responsive-only bump (`min-h-12` under a
breakpoint) would leave two geometries to reason about and to test, for a control whose desktop size
carries no meaning worth preserving. A 48 px segmented control is unremarkable on desktop.

**Mock the data API in the new e2e case** rather than depending on seeded records. Every other case
in `mobile-tap-targets.spec.ts` mocks its API (`page.route('**/api/food/…')`), and the page defaults
to a `week` zoom that fetches only the last 7 days — so a case relying on real data would silently
start skipping the moment that type stopped receiving recent records, turning a regression guard into
a permanent green. Mocking removes both the flake and the need for a skip guard.

**Let the delete control's 48 px width sit inside the existing Actions cell.** Verified by
measurement rather than assumed: with the 48×48 minimum applied at 375 and 390 px on both
`/data/steps` and `/data/weight`, every column width is byte-identical before and after
(e.g. weight: 110 / 102 / 104 / 110 / Actions 86) and the table's `scrollWidth` does not move at all
(steps 590, weight 513). The reason is that the Actions column is already ~86 px wide — its width is
set by the "Actions" header label plus `px-4` cell padding, comfortably more than 48 — so the larger
control drops into space the cell already has. The data columns are unaffected because the table
already overflows its container, which means they are already at their min-content widths and cannot
be squeezed further.

*Alternative considered:* an overflow menu replacing the inline delete. Rejected — that is a
redesign of the interaction, and belongs to the upcoming visual change, not to a geometry fix.

## Risks / Trade-offs

- **Row height grows on types whose rows are currently under 48 px** → Measured: `/data/weight` rows
  go 65 → 73 px; `/data/steps` rows are unchanged at 85 px because their timestamps already wrap.
  This is the intended outcome, not a regression: a row shorter than 48 px cannot host a compliant
  control.
- **A 48 px-tall segmented control is visibly chunkier on desktop** → Accepted deliberately (see
  the decision above); it is the same trade the food flows already made. This is the one call in
  this change that is a matter of taste rather than measurement, and it is easy to reverse with a
  `sm:` variant if the wider look is unwelcome.
- **Seven macro tabs at 48 px tall will wrap onto more rows on `/data/nutrition`** → Their container
  is already `flex-wrap`, so this changes how many rows they occupy, not whether they fit. Verified
  in task 3.6 rather than assumed.
- **The table's horizontal scroll is unpleasant on mobile** → Real, but out of scope and untouched
  by this change; it belongs to the visual redesign. Noted here so it is not mistaken for something
  this change introduces.
